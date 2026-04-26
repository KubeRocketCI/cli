// Package list implements the "krci env list" command.
package list

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

// ListOptions holds all inputs for `krci env list`.
type ListOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Deployment   string
	Cluster      string
	OutputFormat string
}

// NewCmdList returns the "env list" cobra.Command.
// runF is the business-logic function; pass nil to use the default listRun.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List environments (Stages) in the configured namespace",
		Long: `List every KRCI Stage in the configured namespace, optionally
filtered by parent deployment and/or cluster. Rows are sorted by deployment
ascending, then by Stage.spec.order ascending.`,
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		Example: `  # All environments across deployments
  krci env list

  # Filter to one deployment
  krci env list --deployment my-pipeline

  # Filter by cluster
  krci env list --cluster in-cluster

  # JSON envelope for scripting
  krci env list -o json | jq -r '.data.stages[] | "\(.deployment)/\(.env): \(.status)"'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := discovery.ValidateOutputFormat(opts.OutputFormat); err != nil {
				return err
			}

			if cmd.Flags().Changed("deployment") {
				if err := discovery.ValidateDeployment(opts.Deployment); err != nil {
					return err
				}
			}

			if err := discovery.ValidateCluster(opts.Cluster); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Deployment, "deployment", "",
		"Filter by parent deployment (CDPipeline) name")
	cmd.Flags().StringVar(&opts.Cluster, "cluster", "",
		"Filter by cluster name")
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

	svc := portal.NewEnvService(client, cfg.ClusterName, cfg.Namespace)

	rows, err := svc.List(ctx, portal.EnvListFilters{
		Deployment: opts.Deployment,
		Cluster:    opts.Cluster,
	})
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	payload := portal.EnvListPayload{Stages: rows}

	if err := discovery.Render(opts.IO, opts.OutputFormat, payload, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, rows)
	}); err != nil {
		return err
	}

	if len(rows) == 0 && opts.OutputFormat != output.FormatJSON {
		if _, err := fmt.Fprintln(opts.IO.ErrOut, "No environments found."); err != nil {
			return err
		}
	}

	return nil
}

func renderTable(w io.Writer, isTTY bool, rows []portal.EnvSummary) error {
	headers := []string{"DEPLOYMENT", "ENV", "CLUSTER", "NAMESPACE", "TRIGGER", "STATUS"}
	tableRows := make([][]string, 0, len(rows))

	for _, r := range rows {
		status := r.Status
		if isTTY {
			status = output.StatusColor(r.Status)
		}

		tableRows = append(tableRows, []string{
			r.Deployment,
			r.Env,
			r.Cluster,
			r.Namespace,
			r.TriggerType,
			status,
		})
	}

	return discovery.PrintTable(w, isTTY, headers, tableRows)
}
