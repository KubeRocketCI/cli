// Package cli provides Cobra command definitions for the krci CLI.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/k8s"
	"github.com/KubeRocketCI/cli/internal/token"
)

// App holds wired dependencies for all command handlers.
// Config and providers are resolved lazily after Cobra parses flags.
type App struct {
	cfg           *config.Config
	tokenProvider auth.TokenProvider
	k8sDynClient  dynamic.Interface
	k8sProject    *k8s.ProjectService
	k8sDeployment *k8s.DeploymentService
}

// initConfig resolves config after flags are parsed, then wires dependencies.
func (a *App) initConfig(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Resolve()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	a.cfg = cfg
	enc := token.NewAESEncryptor(cfg.KeyringService, cfg.ConfigDir)
	store := token.NewEncryptedStore(cfg.TokenPath, enc)
	a.tokenProvider = auth.NewTokenProvider(store, cfg)

	return nil
}

// requireK8s creates the K8s client on first call and caches it.
// Both ProjectService and DeploymentService share the same dynamic client.
func (a *App) requireK8s() error {
	if a.k8sDynClient != nil {
		return nil
	}

	if a.cfg.APIServer == "" {
		return fmt.Errorf(
			"kubernetes API server not configured\n\nSet it via:\n" +
				"  --api-server flag\n" +
				"  KRCI_API_SERVER env var\n" +
				"  api-server in ~/.config/krci/config.yaml",
		)
	}

	if a.cfg.Namespace == "" {
		return fmt.Errorf(
			"kubernetes namespace not configured\n\nSet it via:\n" +
				"  -n/--namespace flag\n" +
				"  KRCI_NAMESPACE env var\n" +
				"  namespace in ~/.config/krci/config.yaml",
		)
	}

	dynClient, err := k8s.NewDynamicClient(k8s.ClientConfig{
		APIServer: a.cfg.APIServer,
		CAData:    a.cfg.CAData,
		TokenFunc: a.tokenProvider.GetToken,
	})
	if err != nil {
		return fmt.Errorf("kubernetes client initialization failed: %w", err)
	}

	a.k8sDynClient = dynClient
	a.k8sProject = k8s.NewProjectService(dynClient, a.cfg.Namespace)
	a.k8sDeployment = k8s.NewDeploymentService(dynClient, a.cfg.Namespace)

	return nil
}

// NewRootCmd builds the root cobra.Command with all subcommands.
func NewRootCmd() *cobra.Command {
	app := &App{}

	cmd := &cobra.Command{
		Use:               "krci",
		Short:             "KubeRocketCI CLI",
		Long:              "Command-line interface for the KubeRocketCI platform.",
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: app.initConfig,
	}

	config.BindFlags(cmd)

	cmd.AddCommand(
		app.newAuthCmd(),
		app.newProjectCmd(),
		app.newDeploymentCmd(),
		app.newVersionCmd(),
	)

	return cmd
}
