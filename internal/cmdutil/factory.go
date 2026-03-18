// Package cmdutil provides shared CLI utilities, including the Factory dependency container.
package cmdutil

import (
	"fmt"
	"sync"

	"k8s.io/client-go/dynamic"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/k8s"
	"github.com/KubeRocketCI/cli/internal/token"
)

// Factory holds lazy-func dependencies shared across all CLI commands.
// Each func is memoized: the first call resolves the dependency; subsequent calls
// return the cached result instantly.
type Factory struct {
	IOStreams     *iostreams.IOStreams
	Config        func() (*config.Config, error)
	TokenProvider func() (auth.TokenProvider, error)
	K8sClient     func() (dynamic.Interface, error)
}

// New creates a Factory wired to real system resources.
// Config, TokenProvider, and K8sClient are lazily resolved after Cobra
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
		onceK8s      sync.Once
		cachedK8s    dynamic.Interface
		cachedK8sErr error
	)

	f.K8sClient = func() (dynamic.Interface, error) {
		onceK8s.Do(func() {
			cfg, err := f.Config()
			if err != nil {
				cachedK8sErr = err
				return
			}

			if cfg.APIServer == "" {
				cachedK8sErr = fmt.Errorf(
					"kubernetes API server not configured\n\nSet it via:\n" +
						"  --api-server flag\n" +
						"  KRCI_API_SERVER env var\n" +
						"  api-server in ~/.config/krci/config.yaml",
				)
				return
			}

			if cfg.Namespace == "" {
				cachedK8sErr = fmt.Errorf(
					"kubernetes namespace not configured\n\nSet it via:\n" +
						"  -n/--namespace flag\n" +
						"  KRCI_NAMESPACE env var\n" +
						"  namespace in ~/.config/krci/config.yaml",
				)
				return
			}

			tp, err := f.TokenProvider()
			if err != nil {
				cachedK8sErr = err
				return
			}

			dynClient, err := k8s.NewDynamicClient(k8s.ClientConfig{
				APIServer: cfg.APIServer,
				CAData:    cfg.CAData,
				TokenFunc: tp.GetToken,
			})
			if err != nil {
				cachedK8sErr = fmt.Errorf("kubernetes client initialization failed: %w", err)
				return
			}

			cachedK8s = dynClient
		})

		return cachedK8s, cachedK8sErr
	}

	return f
}
