// Package list implements the "krci deployment list" command.
package list

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
)

// maxInlineApps is the maximum number of applications shown inline before truncating.
const maxInlineApps = 3

// ListOptions holds all inputs for the deployment list command.
type ListOptions struct {
	IO           *iostreams.IOStreams
	Services     func() (*portal.Services, error)
	OutputFormat string
}

// NewCmdList returns the "deployment list" cobra.Command.
// runF is the business logic function; pass nil to use the default listRun.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:       f.IOStreams,
		Services: f.Services,
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
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}

			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "", render.OutputFlagUsage)

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	services, err := opts.Services()
	if err != nil {
		return err
	}

	svc := services.Deployment()

	deployments, err := svc.List(ctx)
	if err != nil {
		return render.HandleAuthError(err)
	}

	return render.RenderBare(opts.IO, opts.OutputFormat, deployments, func(w io.Writer, isTTY bool) error {
		headers := []string{"NAME", "APPLICATIONS", "ENVS", "STATUS"}
		rows := make([][]output.Cell, 0, len(deployments))

		for _, d := range deployments {
			rows = append(rows, []output.Cell{
				output.Text(d.Name),
				output.Text(formatApplications(d.Applications)),
				output.Text(strings.Join(d.StageNames, " \u2192 ")),
				output.StatusColor(d.Status),
			})
		}

		return output.PrintTable(w, isTTY, headers, rows)
	})
}

// formatApplications joins application names, truncating if more than maxInlineApps.
func formatApplications(apps []string) string {
	if len(apps) <= maxInlineApps {
		return strings.Join(apps, ", ")
	}

	return strings.Join(apps[:maxInlineApps], ", ") + fmt.Sprintf(" +%d more", len(apps)-maxInlineApps)
}
