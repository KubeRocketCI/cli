package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/k8s"
)

func (a *App) newDeploymentGetCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get deployment details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.requireK8s(); err != nil {
				return err
			}

			detail, err := a.k8sDeployment.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return renderDetail(cmd.OutOrStdout(), output, detail, detailRenderer[*k8s.DeploymentDetail]{
				styled: printStyledDeploymentDetail,
				plain:  printPlainDeploymentDetail,
			})
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: table, json (default: auto-detect)")

	return cmd
}

// deploymentDetailLines builds the ordered field list for a deployment detail.
func deploymentDetailLines(d *k8s.DeploymentDetail, styled bool) []detailLine {
	lines := []detailLine{
		{label: "Name", value: d.Name},
		{label: "Namespace", value: d.Namespace},
		{label: "Applications", value: strings.Join(d.Applications, ", ")},
	}

	if d.Description != "" {
		lines = append(lines, detailLine{label: "Description", value: d.Description})
	}

	statusLine := detailLine{label: "Status", value: d.Status}
	availableLine := detailLine{label: "Available", value: strconv.FormatBool(d.Available)}

	if styled {
		statusLine.styled = statusColor(d.Status)
		availableLine.styled = availableText(d.Available)
	}

	lines = append(lines, statusLine, availableLine)

	return lines
}

// printStyledDeploymentDetail renders deployment details with lipgloss styling.
func printStyledDeploymentDetail(w io.Writer, d *k8s.DeploymentDetail) error {
	if err := printStyledDetailLines(w, deploymentDetailLines(d, true)); err != nil {
		return err
	}

	return printStageSection(w, d.Stages, true)
}

// printPlainDeploymentDetail renders deployment details as plain text for piped output.
func printPlainDeploymentDetail(w io.Writer, d *k8s.DeploymentDetail) error {
	if err := printPlainDetailLines(w, deploymentDetailLines(d, false)); err != nil {
		return err
	}

	return printStageSection(w, d.Stages, false)
}

// printStageSection renders the stages table below the detail lines.
func printStageSection(w io.Writer, stages []k8s.Stage, styled bool) error {
	if len(stages) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if styled {
		if _, err := fmt.Fprintln(w, labelStyle.Render("Environments:")); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "Environments:"); err != nil {
			return err
		}
	}

	headers := []string{"ORDER", "ENV", "DEPLOY MODE", "PROMOTE GATES", "NAMESPACE", "STATUS"}
	rows := stageRows(stages, styled)

	if styled {
		return printStyledTable(w, headers, rows)
	}

	return printTable(w, headers, rows)
}

// stageRows builds table rows from stages. When styled is true, status is colorized.
func stageRows(stages []k8s.Stage, styled bool) [][]string {
	rows := make([][]string, 0, len(stages))

	for _, s := range stages {
		status := s.Status
		if styled {
			status = statusColor(s.Status)
		}

		rows = append(rows, []string{
			strconv.FormatInt(s.Order, 10),
			s.Name,
			s.TriggerType,
			summarizeGates(s.QualityGates),
			s.Namespace,
			status,
		})
	}

	return rows
}

// summarizeGates returns a human-readable summary of quality gates by type.
// e.g., "1 autotest, 1 manual" or "—" if none.
func summarizeGates(gates []k8s.QualityGate) string {
	if len(gates) == 0 {
		return "—"
	}

	var autotests, manual int

	for _, g := range gates {
		switch g.Type {
		case k8s.QualityGateTypeAutotests:
			autotests++
		case k8s.QualityGateTypeManual:
			manual++
		}
	}

	var parts []string

	if autotests > 0 {
		label := "autotest"
		if autotests > 1 {
			label = "autotests"
		}

		parts = append(parts, fmt.Sprintf("%d %s", autotests, label))
	}

	if manual > 0 {
		parts = append(parts, fmt.Sprintf("%d manual", manual))
	}

	if len(parts) == 0 {
		return fmt.Sprintf("%d", len(gates))
	}

	return strings.Join(parts, ", ")
}
