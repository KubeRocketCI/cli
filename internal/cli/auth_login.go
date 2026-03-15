package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/portal"
)

func (a *App) newAuthLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with OIDC provider (Keycloak)",
		Long: `Authenticate by opening a browser to the OIDC provider.
After successful login, credentials are stored encrypted locally
and the OIDC configuration is saved to ~/.config/krci/config.yaml.

The OIDC issuer URL must be configured via one of:
  --issuer-url flag
  KRCI_ISSUER_URL environment variable
  issuer-url in ~/.config/krci/config.yaml`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.cfg.IssuerURL == "" {
				return fmt.Errorf(
					"OIDC issuer URL required.\n\nSet it via:\n" +
						"  --issuer-url flag\n" +
						"  KRCI_ISSUER_URL env var\n" +
						"  issuer-url in ~/.config/krci/config.yaml",
				)
			}

			if err := a.tokenProvider.Login(cmd.Context()); err != nil {
				return err
			}

			// Fetch portal config to auto-populate namespace.
			if a.cfg.PortalURL != "" && a.cfg.Namespace == "" {
				portalCfg, err := portal.FetchConfig(a.cfg.PortalURL)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not fetch portal config: %v\n", err)
				} else if portalCfg.DefaultNamespace != "" {
					if errs := validation.IsDNS1123Label(portalCfg.DefaultNamespace); len(errs) > 0 {
						fmt.Fprintf(os.Stderr, "Warning: portal returned invalid namespace %q, ignoring\n", portalCfg.DefaultNamespace)
					} else {
						a.cfg.Namespace = portalCfg.DefaultNamespace
						fmt.Fprintf(os.Stderr, "Namespace: %s (from portal)\n", a.cfg.Namespace)
					}
				}
			}

			// Save config so subsequent commands don't need --issuer-url.
			if err := config.Save(a.cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not save config: %v\n", err)
			}

			return nil
		},
	}
}
