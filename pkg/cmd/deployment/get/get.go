// Package get implements the "krci deployment get" command.
package get

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/k8s"
	"github.com/KubeRocketCI/cli/internal/output"
)

// GetOptions holds all inputs for the deployment get command.
type GetOptions struct {
	IO           *iostreams.IOStreams
	K8sClient    func() (dynamic.Interface, error)
	Config       func() (*config.Config, error)
	Name         string
	OutputFormat string
}

// NewCmdGet returns the "deployment get" cobra.Command.
// runF is the business logic function; pass nil to use the default getRun.
func NewCmdGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO:        f.IOStreams,
		K8sClient: f.K8sClient,
		Config:    f.Config,
	}

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get deployment details",
		Args:  cobra.ExactArgs(1),
		Example: `  # Get details for a deployment
  krci deployment get my-pipeline

  # Output as JSON
  krci deployment get my-pipeline -o json`,
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

	svc := k8s.NewDeploymentService(dynClient, cfg.Namespace)

	detail, err := svc.Get(ctx, opts.Name)
	if err != nil {
		return err
	}

	return output.RenderDetail(opts.IO, opts.OutputFormat, detail, output.DetailRenderer[*k8s.DeploymentDetail]{
		Styled: printStyledDeploymentDetail,
		Plain:  printPlainDeploymentDetail,
	})
}

// deploymentDetailLines builds the ordered field list for a deployment detail.
func deploymentDetailLines(d *k8s.DeploymentDetail, styled bool) []output.DetailLine {
	lines := []output.DetailLine{
		{Label: "Name", Value: d.Name},
		{Label: "Namespace", Value: d.Namespace},
		{Label: "Applications", Value: strings.Join(d.Applications, ", ")},
	}

	if d.Description != "" {
		lines = append(lines, output.DetailLine{Label: "Description", Value: d.Description})
	}

	statusLine := output.DetailLine{Label: "Status", Value: d.Status}
	availableLine := output.DetailLine{Label: "Available", Value: strconv.FormatBool(d.Available)}

	if styled {
		statusLine.Styled = output.StatusColor(d.Status)
		availableLine.Styled = output.AvailableText(d.Available)
	}

	lines = append(lines, statusLine, availableLine)

	return lines
}

// printStyledDeploymentDetail renders deployment details with lipgloss styling.
func printStyledDeploymentDetail(w io.Writer, d *k8s.DeploymentDetail) error {
	if err := output.PrintStyledDetailLines(w, deploymentDetailLines(d, true)); err != nil {
		return err
	}

	return printStageSection(w, d.Stages, true)
}

// printPlainDeploymentDetail renders deployment details as plain text for piped output.
func printPlainDeploymentDetail(w io.Writer, d *k8s.DeploymentDetail) error {
	if err := output.PrintPlainDetailLines(w, deploymentDetailLines(d, false)); err != nil {
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
		if _, err := fmt.Fprintln(w, output.LabelStyle.Render("Environments:")); err != nil {
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
		return output.PrintStyledTable(w, headers, rows)
	}

	return output.PrintTable(w, headers, rows)
}

// stageRows builds table rows from stages. When styled is true, status is colorized.
func stageRows(stages []k8s.Stage, styled bool) [][]string {
	rows := make([][]string, 0, len(stages))

	for _, s := range stages {
		status := s.Status
		if styled {
			status = output.StatusColor(s.Status)
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
