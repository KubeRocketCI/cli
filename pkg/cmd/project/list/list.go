// Package list implements the "krci project list" command.
package list

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
)

// ListOptions holds all inputs for the project list command.
type ListOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	OutputFormat string
}

// NewCmdList returns the "project list" cobra.Command.
// runF is the business logic function; pass nil to use the default listRun.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List projects",
		Aliases: []string{"ls"},
		Example: `  # List all projects
  krci project list

  # Output as JSON
  krci project list -o json

  # Use the ls alias
  krci project ls`,
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
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	client, err := opts.RestClient()
	if err != nil {
		return err
	}

	svc := portal.NewProjectService(client, cfg.ClusterName, cfg.Namespace)

	projects, err := svc.List(ctx)
	if err != nil {
		return render.HandleAuthError(err)
	}

	return render.RenderBare(opts.IO, opts.OutputFormat, projects, func(w io.Writer, isTTY bool) error {
		headers := []string{"NAME", "TYPE", "LANGUAGE", "BUILD TOOL", "STATUS"}
		rows := make([][]output.Cell, 0, len(projects))

		for _, p := range projects {
			rows = append(rows, append(output.Texts(p.Name, p.Type, p.Language, p.BuildTool), output.StatusColor(p.Status)))
		}

		return output.PrintTable(w, isTTY, headers, rows)
	})
}
