package cli

import (
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/k8s"
)

func (a *App) newProjectGetCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get project details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.requireK8s(); err != nil {
				return err
			}

			project, err := a.k8sProject.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return renderDetail(cmd.OutOrStdout(), output, project, detailRenderer[*k8s.Project]{
				styled: printStyledProjectDetail,
				plain:  printPlainProjectDetail,
			})
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json (default: auto-detect)")

	return cmd
}

// detailLine holds one label-value pair for resource detail rendering.
// When styled is populated, the styled renderer uses it instead of value.
type detailLine struct {
	label  string
	value  string
	styled string
}

// projectDetailLines builds the ordered field list for a project.
// When styled is true, Status and Available get colorized representations.
func projectDetailLines(p *k8s.Project, styled bool) []detailLine {
	lines := []detailLine{
		{label: "Name", value: p.Name},
		{label: "Namespace", value: p.Namespace},
		{label: "Type", value: p.Type},
		{label: "Language", value: p.Language},
		{label: "Build Tool", value: p.BuildTool},
	}

	if p.Framework != "" {
		lines = append(lines, detailLine{label: "Framework", value: p.Framework})
	}

	lines = append(lines, detailLine{label: "Git Server", value: p.GitServer})

	if p.GitURL != "" {
		lines = append(lines, detailLine{label: "Git URL", value: p.GitURL})
	}

	statusLine := detailLine{label: "Status", value: p.Status}
	availableLine := detailLine{label: "Available", value: strconv.FormatBool(p.Available)}

	if styled {
		statusLine.styled = statusColor(p.Status)
		availableLine.styled = availableText(p.Available)
	}

	lines = append(lines, statusLine, availableLine)

	return lines
}

// printStyledProjectDetail renders project details with lipgloss styling.
func printStyledProjectDetail(w io.Writer, p *k8s.Project) error {
	return printStyledDetailLines(w, projectDetailLines(p, true))
}

// printPlainProjectDetail renders project details as plain text for piped output.
func printPlainProjectDetail(w io.Writer, p *k8s.Project) error {
	return printPlainDetailLines(w, projectDetailLines(p, false))
}
