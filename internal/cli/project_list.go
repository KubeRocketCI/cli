package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) newProjectListCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List projects",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.requireK8s(); err != nil {
				return err
			}

			projects, err := a.k8sProject.List(cmd.Context())
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			format := resolveFormat(output, w)

			switch format {
			case outputFormatJSON:
				return printJSON(w, projects)
			case outputFormatTable:
				headers := []string{"NAME", "TYPE", "LANGUAGE", "BUILD TOOL", "STATUS"}
				rows := make([][]string, 0, len(projects))

				isTTY := isTerminal(w)

				for _, p := range projects {
					status := p.Status
					if isTTY {
						status = statusColor(p.Status)
					}

					rows = append(rows, []string{p.Name, p.Type, p.Language, p.BuildTool, status})
				}

				if isTTY {
					return printStyledTable(w, headers, rows)
				}

				return printTable(w, headers, rows)
			default:
				return fmt.Errorf("unknown output format: %s (use 'json' or 'table')", format)
			}
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json (default: auto-detect)")

	return cmd
}
