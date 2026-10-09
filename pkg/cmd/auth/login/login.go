// Package login implements the "krci auth login" command.
package login

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// clusterFetchFunc fetches cluster configuration from a portal URL using a bearer token.
type clusterFetchFunc func(ctx context.Context, portalURL, token string) (*portal.ClusterConfig, error)

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
		Args: cobra.NoArgs,
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

	// Auto-discover OIDC issuer URL from portal (public endpoint, no auth needed).
	// Mutate the factory-cached pointer so TokenProvider() (which holds the
	// same *config.Config) sees the discovered issuer URL during construction.
	if cfg.IssuerURL == "" {
		issuerURL, err := portal.FetchOIDCConfig(cmd.Context(), cfg.PortalURL)
		if err != nil {
			if errors.Is(err, portal.ErrHTTPSRequired) {
				return fmt.Errorf("invalid portal URL: %w", err)
			}

			return cmdutil.ConfigNotSetError("OIDC issuer URL",
				fmt.Sprintf("could not discover OIDC issuer from portal: %v", err),
				cmdutil.IssuerURLOption, "")
		}

		cfg.IssuerURL = issuerURL

		if err := auth.ValidateIssuerURL(cfg.IssuerURL); err != nil {
			return fmt.Errorf("portal returned invalid OIDC issuer URL: %w", err)
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
	cfgCopy := *cfg

	populateClusterConfig(cmd.Context(), tp, &cfgCopy, opts.IO.ErrOut, portal.FetchClusterConfig)

	if err := config.Save(&cfgCopy); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "Warning: could not save config: %v\n", err)
	}

	// Checked after Save: an interrupted login keeps the token and portal URL,
	// and the process still dies by the signal.
	return cmd.Context().Err()
}

// populateClusterConfig fetches cluster name and namespace from the portal
// when they are not already provided via env vars or config file.
func populateClusterConfig(
	ctx context.Context, tp auth.TokenProvider, cfg *config.Config,
	errOut io.Writer, fetch clusterFetchFunc,
) {
	if cfg.ClusterName != "" && cfg.Namespace != "" {
		return
	}

	err := fetchClusterMetadata(ctx, tp, cfg, errOut, fetch)

	// Cancelled: no warnings; loginRun returns ctx.Err().
	if ctx.Err() != nil {
		return
	}

	if err != nil {
		_, _ = fmt.Fprintf(errOut, "Warning: %v\n", err)
	}

	if cfg.Namespace == "" {
		_, _ = fmt.Fprintf(errOut,
			"Warning: namespace not configured; set KRCI_NAMESPACE or re-run login\n")
	}
}

func fetchClusterMetadata(
	ctx context.Context, tp auth.TokenProvider, cfg *config.Config,
	errOut io.Writer, fetch clusterFetchFunc,
) error {
	tok, err := tp.GetToken(ctx)
	if err != nil {
		return fmt.Errorf("could not get token for cluster config: %w", err)
	}

	clusterCfg, err := fetch(ctx, cfg.PortalURL, tok)
	if err != nil {
		return fmt.Errorf("could not fetch cluster config: %w", err)
	}

	if cfg.ClusterName == "" && clusterCfg.ClusterName != "" {
		cfg.ClusterName = clusterCfg.ClusterName
	}

	if cfg.Namespace == "" && clusterCfg.DefaultNamespace != "" {
		if cmdutil.IsValidDNS1123Label(clusterCfg.DefaultNamespace) {
			cfg.Namespace = clusterCfg.DefaultNamespace
			_, _ = fmt.Fprintf(errOut, "Namespace: %s (from portal)\n", cfg.Namespace)
		} else {
			_, _ = fmt.Fprintf(errOut,
				"Warning: portal returned invalid namespace %q, ignoring\n", clusterCfg.DefaultNamespace)
		}
	}

	return nil
}
