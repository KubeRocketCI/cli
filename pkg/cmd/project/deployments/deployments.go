// Package deployments implements the "krci project deployments <project>" command.
package deployments

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
)

// ListOptions holds all inputs for `krci project deployments <project>`.
type ListOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Project      string
	OutputFormat string
}

// NewCmdDeployments returns the "project deployments <project>" cobra.Command.
// runF is the business-logic function; pass nil to use the default run.
func NewCmdDeployments(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "deployments <project>",
		Short: "Show every (deployment, env) row where this project is registered",
		Long: `Show every (deployment, env) pair in the configured namespace where
the given project is registered, with current health, sync, version, image
digest, cluster, namespace, and ingress URLs. Rows for stages where the
project is registered (CDPipeline.spec.applications) but no Application
exists yet are emitted with "-" placeholders (table) or null values (JSON).

Rows are sorted by deployment ascending, then by Stage.spec.order ascending.`,
		Args: cmdutil.ExactArgs(1, "a project name",
			"to see available projects: krci project list"),
		Example: `  # Default
  krci project deployments my-app

  # JSON envelope (full image digest, full ingress URLs)
  krci project deployments my-app -o json

  # Scripting — list every env where my-app is degraded
  krci project deployments my-app -o json |
    jq -r '.data.rows[] | select(.status=="degraded") | "\(.deployment)/\(.env)"'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Project = args[0]

			if err := discovery.ValidateOutputFormat(opts.OutputFormat); err != nil {
				return err
			}

			if err := discovery.ValidateProject(opts.Project); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	client, err := opts.RestClient()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewDeploymentByProjectService(client, cfg.ClusterName, cfg.Namespace)

	rows, err := svc.List(ctx, opts.Project)
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	payload := portal.ProjectDeploymentsPayload{
		Project: opts.Project,
		Rows:    rows,
	}

	if err := discovery.Render(opts.IO, opts.OutputFormat, payload, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, rows)
	}); err != nil {
		return err
	}

	if len(rows) == 0 && opts.OutputFormat != output.FormatJSON {
		if _, err := fmt.Fprintf(opts.IO.ErrOut, "No deployments found for project %s.\n", opts.Project); err != nil {
			return err
		}
	}

	return nil
}

func renderTable(w io.Writer, isTTY bool, rows []portal.ProjectDeploymentRow) error {
	headers := []string{"DEPLOYMENT", "ENV", "STATUS", "SYNC", "VERSION", "IMAGE_SHA", "CLUSTER", "NAMESPACE", "INGRESS"}
	tableRows := make([][]string, 0, len(rows))

	for _, r := range rows {
		cells := []string{
			r.Deployment,
			r.Env,
			discovery.OptStatusCell(r.Status, isTTY),
			discovery.OptCell(r.Sync),
			discovery.OptCell(r.Version),
			discovery.ShortDigestCell(r.ImageDigest),
			r.Cluster,
			r.Namespace,
		}
		tableRows = append(tableRows, discovery.ExpandIngressRows(cells, discovery.IngressLines(r.IngressURLs, isTTY))...)
	}

	return discovery.PrintTable(w, isTTY, headers, tableRows)
}
