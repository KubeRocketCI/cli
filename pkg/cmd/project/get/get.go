// Package get implements the "krci project get" command.
package get

import (
	"context"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/k8s"
	"github.com/KubeRocketCI/cli/internal/output"
)

// GetOptions holds all inputs for the project get command.
type GetOptions struct {
	IO           *iostreams.IOStreams
	K8sClient    func() (dynamic.Interface, error)
	Config       func() (*config.Config, error)
	Name         string
	OutputFormat string
}

// NewCmdGet returns the "project get" cobra.Command.
// runF is the business logic function; pass nil to use the default getRun.
func NewCmdGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO:        f.IOStreams,
		K8sClient: f.K8sClient,
		Config:    f.Config,
	}

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get project details",
		Args:  cobra.ExactArgs(1),
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

	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "", "Output format: table, json (default: auto-detect)")

	return cmd
}

func getRun(ctx context.Context, opts *GetOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	dynClient, err := opts.K8sClient()
	if err != nil {
		return err
	}

	svc := k8s.NewProjectService(dynClient, cfg.Namespace)

	project, err := svc.Get(ctx, opts.Name)
	if err != nil {
		return err
	}

	return output.RenderDetail(opts.IO, opts.OutputFormat, project, output.DetailRenderer[*k8s.Project]{
		Styled: printStyledProjectDetail,
		Plain:  printPlainProjectDetail,
	})
}

// projectDetailLines builds the ordered field list for a project.
// When styled is true, Status and Available get colorized representations.
func projectDetailLines(p *k8s.Project, styled bool) []output.DetailLine {
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

	statusLine := output.DetailLine{Label: "Status", Value: p.Status}
	availableLine := output.DetailLine{Label: "Available", Value: strconv.FormatBool(p.Available)}

	if styled {
		statusLine.Styled = output.StatusColor(p.Status)
		availableLine.Styled = output.AvailableText(p.Available)
	}

	lines = append(lines, statusLine, availableLine)

	return lines
}

// printStyledProjectDetail renders project details with lipgloss styling.
func printStyledProjectDetail(w io.Writer, p *k8s.Project) error {
	return output.PrintStyledDetailLines(w, projectDetailLines(p, true))
}

// printPlainProjectDetail renders project details as plain text for piped output.
func printPlainProjectDetail(w io.Writer, p *k8s.Project) error {
	return output.PrintPlainDetailLines(w, projectDetailLines(p, false))
}
