// Package list implements the "krci deployment list" command.
package list

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// maxInlineApps is the maximum number of applications shown inline before truncating.
const maxInlineApps = 3

// ListOptions holds all inputs for the deployment list command.
type ListOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	OutputFormat string
}

// NewCmdList returns the "deployment list" cobra.Command.
// runF is the business logic function; pass nil to use the default listRun.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List deployments",
		Aliases: []string{"ls"},
		Example: `  # List all deployments
  krci deployment list

  # Output as JSON
  krci deployment list -o json

  # Use the ls alias
  krci deployment ls`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}

			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "", "Output format: table, json (default: auto-detect)")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	client, err := opts.RestClient()
	if err != nil {
		return err
	}

	svc := portal.NewDeploymentService(client, cfg.ClusterName, cfg.Namespace)

	deployments, err := svc.List(ctx)
	if err != nil {
		if errors.Is(err, portal.ErrUnauthorized) {
			return cmdutil.ErrAuthRequired(err)
		}

		return err
	}

	return output.RenderList(opts.IO, opts.OutputFormat, deployments, func(isTTY bool) ([]string, [][]string) {
		headers := []string{"NAME", "APPLICATIONS", "ENVS", "STATUS"}
		rows := make([][]string, 0, len(deployments))

		for _, d := range deployments {
			status := d.Status
			if isTTY {
				status = output.StatusColor(d.Status)
			}

			rows = append(rows, []string{
				d.Name,
				formatApplications(d.Applications),
				strings.Join(d.StageNames, " \u2192 "),
				status,
			})
		}

		return headers, rows
	})
}

// formatApplications joins application names, truncating if more than maxInlineApps.
func formatApplications(apps []string) string {
	if len(apps) <= maxInlineApps {
		return strings.Join(apps, ", ")
	}

	return strings.Join(apps[:maxInlineApps], ", ") + fmt.Sprintf(" +%d more", len(apps)-maxInlineApps)
}
