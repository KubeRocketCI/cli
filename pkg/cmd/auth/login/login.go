// Package login implements the "krci auth login" command.
package login

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// dns1123LabelRegexp validates a Kubernetes namespace name.
var dns1123LabelRegexp = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

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
		Short: "Authenticate with OIDC provider",
		Long: `Authenticate by opening a browser to the OIDC provider.
After successful login, credentials are stored encrypted locally
and the configuration is saved to ~/.config/krci/config.yaml.

The portal URL must be configured via one of:
  --portal-url flag
  KRCI_PORTAL_URL environment variable
  portal-url in ~/.config/krci/config.yaml`,
		Example: `  # Log in using portal URL
  krci auth login --portal-url https://portal.example.com

  # Log in using an environment variable
  export KRCI_PORTAL_URL=https://portal.example.com
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

	if cfg.PortalURL == "" {
		return cmdutil.ConfigNotSetError("portal URL", "", cmdutil.PortalURLOption, "")
	}

	// Fetch portal config to auto-discover namespace and cluster name.
	portalCfg, err := portal.FetchConfig(cfg.PortalURL)
	if err != nil {
		if errors.Is(err, portal.ErrHTTPSRequired) {
			return fmt.Errorf("invalid portal URL: %w", err)
		}

		_, _ = fmt.Fprintf(opts.IO.ErrOut, "Warning: could not fetch portal config: %v\n", err)
	}

	// Auto-discover issuer URL from portal config if not explicitly set.
	// Mutate the factory-cached pointer so TokenProvider() (which holds the
	// same *config.Config) sees the discovered issuer URL during construction.
	if cfg.IssuerURL == "" {
		switch {
		case portalCfg != nil && portalCfg.OIDCIssuerURL != "":
			cfg.IssuerURL = portalCfg.OIDCIssuerURL

			if err := auth.ValidateIssuerURL(cfg.IssuerURL); err != nil {
				return fmt.Errorf("portal returned invalid OIDC issuer URL: %w", err)
			}
		case portalCfg != nil:
			return cmdutil.ConfigNotSetError("OIDC issuer URL",
				"portal did not return an OIDC issuer URL (portal may be outdated)",
				cmdutil.IssuerURLOption, "")
		default:
			return cmdutil.ConfigNotSetError("OIDC issuer URL",
				"could not fetch portal config; unable to discover OIDC issuer URL",
				cmdutil.IssuerURLOption, "")
		}
	}

	tp, err := opts.TokenProvider()
	if err != nil {
		return err
	}

	if err := tp.Login(cmd.Context()); err != nil {
		return err
	}

	// Clone cfg so Save() writes only the fields we intend to persist.
	// The in-memory mutation of cfg.IssuerURL above is intentional (for
	// TokenProvider) and is also captured in cfgCopy for disk persistence.
	cfgCopy := *cfg

	// Auto-populate cluster name and namespace from portal.
	if portalCfg != nil {
		if cfgCopy.ClusterName == "" && portalCfg.ClusterName != "" {
			cfgCopy.ClusterName = portalCfg.ClusterName
		}

		if cfgCopy.Namespace == "" && portalCfg.DefaultNamespace != "" {
			if dns1123LabelRegexp.MatchString(portalCfg.DefaultNamespace) {
				cfgCopy.Namespace = portalCfg.DefaultNamespace
				_, _ = fmt.Fprintf(opts.IO.ErrOut, "Namespace: %s (from portal)\n", cfgCopy.Namespace)
			} else {
				_, _ = fmt.Fprintf(opts.IO.ErrOut,
					"Warning: portal returned invalid namespace %q, ignoring\n", portalCfg.DefaultNamespace)
			}
		}
	}

	if cfgCopy.Namespace == "" {
		_, _ = fmt.Fprintf(opts.IO.ErrOut,
			"Warning: namespace not configured; set KRCI_NAMESPACE or re-run login\n")
	}

	if err := config.Save(&cfgCopy); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "Warning: could not save config: %v\n", err)
	}

	return nil
}
