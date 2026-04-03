// Package cmdutil provides shared CLI utilities, including the Factory dependency container.
package cmdutil

import (
	"fmt"
	"sync"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/token"
)

// Factory holds lazy-func dependencies shared across all CLI commands.
// Each func is memoized: the first call resolves the dependency; subsequent calls
// return the cached result instantly.
type Factory struct {
	IOStreams     *iostreams.IOStreams
	Config        func() (*config.Config, error)
	TokenProvider func() (auth.TokenProvider, error)
	PortalClient  func() (*portal.Client, error)
}

// New creates a Factory wired to real system resources.
// Config, TokenProvider, and PortalClient are lazily resolved after Cobra
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
		oncePortal      sync.Once
		cachedPortal    *portal.Client
		cachedPortalErr error
	)

	f.PortalClient = func() (*portal.Client, error) {
		oncePortal.Do(func() {
			cfg, err := f.Config()
			if err != nil {
				cachedPortalErr = err
				return
			}

			if cfg.PortalURL == "" {
				cachedPortalErr = ConfigNotSetError("portal URL", "", PortalURLOption, LoginHint)
				return
			}

			if cfg.ClusterName == "" {
				cachedPortalErr = ConfigNotSetError("cluster name", "", ClusterNameOption, LoginHint)
				return
			}

			if cfg.Namespace == "" {
				cachedPortalErr = ConfigNotSetError("namespace", "", NamespaceOption, LoginHint)
				return
			}

			tp, err := f.TokenProvider()
			if err != nil {
				cachedPortalErr = err
				return
			}

			cachedPortal, cachedPortalErr = portal.NewClient(cfg.PortalURL, tp.GetToken)
		})

		return cachedPortal, cachedPortalErr
	}

	return f
}
