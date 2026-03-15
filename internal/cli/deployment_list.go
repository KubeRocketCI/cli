package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// maxInlineApps is the maximum number of applications shown inline before truncating.
const maxInlineApps = 3

func (a *App) newDeploymentListCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List deployments",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.requireK8s(); err != nil {
				return err
			}

			deployments, err := a.k8sDeployment.List(cmd.Context())
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			format := resolveFormat(output, w)

			switch format {
			case outputFormatJSON:
				return printJSON(w, deployments)
			case outputFormatTable:
				headers := []string{"NAME", "APPLICATIONS", "ENVS", "STATUS"}
				rows := make([][]string, 0, len(deployments))

				isTTY := isTerminal(w)

				for _, d := range deployments {
					status := d.Status
					if isTTY {
						status = statusColor(d.Status)
					}

					rows = append(rows, []string{
						d.Name,
						formatApplications(d.Applications),
						strings.Join(d.StageNames, " \u2192 "),
						status,
					})
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

// formatApplications joins application names, truncating if more than maxInlineApps.
func formatApplications(apps []string) string {
	if len(apps) <= maxInlineApps {
		return strings.Join(apps, ", ")
	}

	return strings.Join(apps[:maxInlineApps], ", ") + fmt.Sprintf(" +%d more", len(apps)-maxInlineApps)
}
