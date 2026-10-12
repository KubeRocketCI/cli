// Package get implements the "krci project get" command.
package get

import (
	"context"
	"errors"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
)

// GetOptions holds all inputs for the project get command.
type GetOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Name         string
	OutputFormat string
}

// NewCmdGet returns the "project get" cobra.Command.
// runF is the business logic function; pass nil to use the default getRun.
func NewCmdGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get project details",
		Args:  cmdutil.ExactArgs(1, "a project name", "to see available projects: krci project list"),
		Example: `  # Get details for a project
  krci project get my-app

  # Output as JSON
  krci project get my-app -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]

			if runF != nil {
				return runF(opts)
			}

			return getRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "", render.OutputFlagUsage)

	return cmd
}

func getRun(ctx context.Context, opts *GetOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	client, err := opts.RestClient()
	if err != nil {
		return err
	}

	svc := portal.NewProjectService(client, cfg.ClusterName, cfg.Namespace)

	project, err := svc.Get(ctx, opts.Name)
	if err != nil {
		if errors.Is(err, portal.ErrNotFound) {
			return cmdutil.Rewritef(err, "project %q not found", opts.Name)
		}

		return render.HandleAuthError(err)
	}

	return render.RenderBare(opts.IO, opts.OutputFormat, project, func(w io.Writer, isTTY bool) error {
		return output.PrintDetailLines(w, isTTY, projectDetailLines(project))
	})
}

// projectDetailLines builds the ordered field list for a project. Status and
// Available carry their terminal colour in Styled.
func projectDetailLines(p *portal.Project) []output.DetailLine {
	lines := []output.DetailLine{
		{Label: "Name", Value: p.Name},
		{Label: "Namespace", Value: p.Namespace},
		{Label: "Type", Value: p.Type},
		{Label: "Language", Value: p.Language},
		{Label: "Build Tool", Value: p.BuildTool},
	}

	if p.Framework != "" {
		lines = append(lines, output.DetailLine{Label: "Framework", Value: p.Framework})
	}

	lines = append(lines, output.DetailLine{Label: "Git Server", Value: p.GitServer})

	if p.GitURL != "" {
		lines = append(lines, output.DetailLine{Label: "Git URL", Value: p.GitURL})
	}

	lines = append(lines,
		output.DetailLine{Label: "Status", Value: p.Status, Styled: output.StatusColor(p.Status)},
		output.DetailLine{
			Label: "Available", Value: strconv.FormatBool(p.Available), Styled: output.AvailableText(p.Available),
		},
	)

	return lines
}
