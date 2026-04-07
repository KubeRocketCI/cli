// Package list implements the "krci project list" command.
package list

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
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

	svc := portal.NewProjectService(client, cfg.ClusterName, cfg.Namespace)

	projects, err := svc.List(ctx)
	if err != nil {
		if errors.Is(err, portal.ErrUnauthorized) {
			return cmdutil.ErrAuthRequired(err)
		}

		return err
	}

	return output.RenderList(opts.IO, opts.OutputFormat, projects, func(isTTY bool) ([]string, [][]string) {
		headers := []string{"NAME", "TYPE", "LANGUAGE", "BUILD TOOL", "STATUS"}
		rows := make([][]string, 0, len(projects))

		for _, p := range projects {
			status := p.Status
			if isTTY {
				status = output.StatusColor(p.Status)
			}

			rows = append(rows, []string{p.Name, p.Type, p.Language, p.BuildTool, status})
		}

		return headers, rows
	})
}
