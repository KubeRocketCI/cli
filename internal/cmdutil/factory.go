// Package cmdutil provides shared CLI utilities, including the Factory dependency container.
package cmdutil

import (
	"context"
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
// Each func is memoized: the first call resolves the dependency; subsequent calls
// return the cached result instantly.
type Factory struct {
	IOStreams     *iostreams.IOStreams
	Config        func() (*config.Config, error)
	TokenProvider func() (auth.TokenProvider, error)
	RestClient    func() (*restapi.ClientWithResponses, error)
}

// New creates a Factory wired to real system resources.
// Config, TokenProvider, and RestClient are lazily resolved after Cobra
// parses command-line flags (triggered by PersistentPreRunE on the root command).
func New() *Factory {
	f := &Factory{
		IOStreams: iostreams.System(),
	}

	var (
		muCfg        sync.Mutex
		cachedConfig *config.Config
	)

	f.Config = func() (*config.Config, error) {
		muCfg.Lock()
		defer muCfg.Unlock()

		if cachedConfig != nil {
			return cachedConfig, nil
		}

		cfg, err := config.Resolve()
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}

		cachedConfig = cfg
		return cachedConfig, nil
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
