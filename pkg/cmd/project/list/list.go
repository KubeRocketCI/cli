// Package list implements the "krci project list" command.
package list

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/k8s"
	"github.com/KubeRocketCI/cli/internal/output"
)

// ListOptions holds all inputs for the project list command.
type ListOptions struct {
	IO           *iostreams.IOStreams
	K8sClient    func() (dynamic.Interface, error)
	Config       func() (*config.Config, error)
	OutputFormat string
}

// NewCmdList returns the "project list" cobra.Command.
// runF is the business logic function; pass nil to use the default listRun.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:        f.IOStreams,
		K8sClient: f.K8sClient,
		Config:    f.Config,
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

	dynClient, err := opts.K8sClient()
	if err != nil {
		return err
	}

	svc := k8s.NewProjectService(dynClient, cfg.Namespace)

	projects, err := svc.List(ctx)
	if err != nil {
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
