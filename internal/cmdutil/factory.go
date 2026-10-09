// Package cmdutil provides shared CLI utilities, including the Factory dependency container.
package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/token"
)

// DefaultHTTPTimeout caps every portal REST request. Must exceed the portal's
// own 30s request timeout so its 408 or truncated:true response arrives before
// the client aborts as a transport error.
const DefaultHTTPTimeout = 45 * time.Second

// Factory holds lazy-func dependencies shared across all CLI commands.
// Config memoizes the first successful result and retries after a failure;
// TokenProvider and RestClient memoize their first result, including an error.
// Commands copy the func values when NewCmdRoot builds the tree; reassigning a
// field afterwards does not reach them. Set the config source through
// SetConfigResolver.
type Factory struct {
	IOStreams     *iostreams.IOStreams
	Config        func() (*config.Config, error)
	TokenProvider func() (auth.TokenProvider, error)
	RestClient    func() (*restapi.ClientWithResponses, error)

	// Now is the clock for running pipeline-run durations. Default: time.Now.
	// Set it before the command is built.
	Now func() time.Time

	muCfg   sync.Mutex // guards resolve and cfg
	resolve func() (*config.Config, error)
	cfg     *config.Config // first successful resolve result
}

// New creates a Factory over ios, which must be non-nil.
// Config, TokenProvider, and RestClient resolve on first call, from a command's
// run function after Cobra has parsed flags. Config reads from the resolver set
// by SetConfigResolver and memoizes the first successful result.
func New(ios *iostreams.IOStreams) *Factory {
	f := &Factory{
		IOStreams: ios,
		Now:       time.Now,
		resolve: func() (*config.Config, error) {
			return nil, errors.New("config resolver not set")
		},
	}

	f.Config = func() (*config.Config, error) {
		f.muCfg.Lock()
		defer f.muCfg.Unlock()

		if f.cfg != nil {
			return f.cfg, nil
		}

		cfg, err := f.resolve()
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}

		f.cfg = cfg
		return f.cfg, nil
	}

	var (
		onceTP      sync.Once
		cachedTP    auth.TokenProvider
		cachedTPErr error
	)

	f.TokenProvider = func() (auth.TokenProvider, error) {
		onceTP.Do(func() {
			cfg, err := f.Config()
			if err != nil {
				cachedTPErr = err
				return
			}

			enc := token.NewAESEncryptor(cfg.KeyringService, cfg.ConfigDir)
			store := token.NewEncryptedStore(cfg.TokenPath, enc)
			cachedTP = auth.NewTokenProvider(store, cfg)
		})

		return cachedTP, cachedTPErr
	}

	var (
		onceRest      sync.Once
		cachedRest    *restapi.ClientWithResponses
		cachedRestErr error
	)

	f.RestClient = func() (*restapi.ClientWithResponses, error) {
		onceRest.Do(func() {
			cfg, err := f.Config()
			if err != nil {
				cachedRestErr = err
				return
			}

			if cfg.PortalURL == "" {
				cachedRestErr = ConfigNotSetError("portal URL", "", PortalURLOption, LoginHint)
				return
			}

			if cfg.ClusterName == "" {
				cachedRestErr = ConfigNotSetError("cluster name", "", ClusterNameOption, LoginHint)
				return
			}

			if cfg.Namespace == "" {
				cachedRestErr = ConfigNotSetError("namespace", "", NamespaceOption, LoginHint)
				return
			}

			tp, err := f.TokenProvider()
			if err != nil {
				cachedRestErr = err
				return
			}

			httpClient := &http.Client{Timeout: DefaultHTTPTimeout}

			bearerAuth := func(ctx context.Context, req *http.Request) error {
				tok, err := tp.GetToken(ctx)
				if err != nil {
					return fmt.Errorf("obtaining auth token: %w", err)
				}
				req.Header.Set("Authorization", "Bearer "+tok)
				return nil
			}

			cachedRest, cachedRestErr = restapi.NewClientWithResponses(
				cfg.PortalURL+"/rest",
				restapi.WithHTTPClient(httpClient),
				restapi.WithRequestEditorFn(bearerAuth),
			)
		})

		return cachedRest, cachedRestErr
	}

	return f
}

// SetConfigResolver sets the source Config reads from. Call before the first
// Config call; a value memoized earlier is kept.
func (f *Factory) SetConfigResolver(r func() (*config.Config, error)) {
	f.muCfg.Lock()
	defer f.muCfg.Unlock()

	f.resolve = r
}
