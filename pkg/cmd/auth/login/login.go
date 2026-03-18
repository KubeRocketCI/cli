// Package login implements the "krci auth login" command.
package login

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// LoginOptions holds all inputs for the login command.
type LoginOptions struct {
	IO            *iostreams.IOStreams
	Config        func() (*config.Config, error)
	TokenProvider func() (auth.TokenProvider, error)
}

// NewCmdLogin returns the "auth login" cobra.Command.
// runF is the business logic function; pass nil to use the default loginRun.
func NewCmdLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{
		IO:            f.IOStreams,
		Config:        f.Config,
		TokenProvider: f.TokenProvider,
	}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with OIDC provider (Keycloak)",
		Long: `Authenticate by opening a browser to the OIDC provider.
After successful login, credentials are stored encrypted locally
and the OIDC configuration is saved to ~/.config/krci/config.yaml.

The OIDC issuer URL must be configured via one of:
  --issuer-url flag
  KRCI_ISSUER_URL environment variable
  issuer-url in ~/.config/krci/config.yaml`,
		Example: `  # Log in using an issuer URL passed as a flag
  krci auth login --issuer-url https://keycloak.example.com/realms/myrealm

  # Log in using an issuer URL from the environment
  export KRCI_ISSUER_URL=https://keycloak.example.com/realms/myrealm
  krci auth login`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}

			return loginRun(cmd, opts)
		},
	}

	return cmd
}

func loginRun(cmd *cobra.Command, opts *LoginOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	if cfg.IssuerURL == "" {
		return fmt.Errorf(
			"OIDC issuer URL required.\n\nSet it via:\n" +
				"  --issuer-url flag\n" +
				"  KRCI_ISSUER_URL env var\n" +
				"  issuer-url in ~/.config/krci/config.yaml",
		)
	}

	tp, err := opts.TokenProvider()
	if err != nil {
		return err
	}

	if err := tp.Login(cmd.Context()); err != nil {
		return err
	}

	// Clone cfg to avoid mutating the factory-cached pointer.
	cfgCopy := *cfg

	// Fetch portal config to auto-populate namespace.
	if cfgCopy.PortalURL != "" && cfgCopy.Namespace == "" {
		portalCfg, err := portal.FetchConfig(cfgCopy.PortalURL)
		if err != nil {
			_, _ = fmt.Fprintf(opts.IO.ErrOut, "Warning: could not fetch portal config: %v\n", err)
		} else if portalCfg.DefaultNamespace != "" {
			if errs := validation.IsDNS1123Label(portalCfg.DefaultNamespace); len(errs) > 0 {
				_, _ = fmt.Fprintf(opts.IO.ErrOut,
					"Warning: portal returned invalid namespace %q, ignoring\n", portalCfg.DefaultNamespace)
			} else {
				cfgCopy.Namespace = portalCfg.DefaultNamespace
				_, _ = fmt.Fprintf(opts.IO.ErrOut, "Namespace: %s (from portal)\n", cfgCopy.Namespace)
			}
		}
	}

	// Save config so subsequent commands don't need --issuer-url.
	if err := config.Save(&cfgCopy); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "Warning: could not save config: %v\n", err)
	}

	return nil
}
