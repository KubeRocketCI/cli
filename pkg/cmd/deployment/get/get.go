// Package get implements the "krci deployment get" command.
package get

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// GetOptions holds all inputs for the deployment get command.
type GetOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Name         string
	OutputFormat string
}

// NewCmdGet returns the "deployment get" cobra.Command.
// runF is the business logic function; pass nil to use the default getRun.
func NewCmdGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get deployment details",
		Args:  cmdutil.ExactArgs(1, "a deployment name", "to see available deployments: krci deployment list"),
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

	client, err := opts.RestClient()
	if err != nil {
		return err
	}

	svc := portal.NewDeploymentService(client, cfg.ClusterName, cfg.Namespace)

	detail, err := svc.Get(ctx, opts.Name)
	if err != nil {
		if errors.Is(err, portal.ErrNotFound) {
			return fmt.Errorf("deployment %q not found", opts.Name)
		}

		if errors.Is(err, portal.ErrUnauthorized) {
			return cmdutil.ErrAuthRequired(err)
		}

		return err
	}

	return output.RenderDetail(opts.IO, opts.OutputFormat, detail, output.DetailRenderer[*portal.DeploymentDetail]{
		Styled: printStyledDeploymentDetail,
		Plain:  printPlainDeploymentDetail,
	})
}

// deploymentDetailLines builds the ordered field list for a deployment detail.
func deploymentDetailLines(d *portal.DeploymentDetail, styled bool) []output.DetailLine {
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

	if d.DetailedMessage != "" {
		lines = append(lines, output.DetailLine{Label: "Message", Value: output.SingleLine(d.DetailedMessage)})
	}

	return lines
}

// printStyledDeploymentDetail renders deployment details with lipgloss styling.
func printStyledDeploymentDetail(w io.Writer, d *portal.DeploymentDetail) error {
	if err := output.PrintStyledDetailLines(w, deploymentDetailLines(d, true)); err != nil {
		return err
	}

	return printStageSection(w, d.Stages, true)
}

// printPlainDeploymentDetail renders deployment details as plain text for piped output.
func printPlainDeploymentDetail(w io.Writer, d *portal.DeploymentDetail) error {
	if err := output.PrintPlainDetailLines(w, deploymentDetailLines(d, false)); err != nil {
		return err
	}

	return printStageSection(w, d.Stages, false)
}

// printStageSection renders the stages table below the detail lines.
func printStageSection(w io.Writer, stages []portal.Stage, styled bool) error {
	if len(stages) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := printSectionHeading(w, styled, "Environments"); err != nil {
		return err
	}

	headers := []string{"ORDER", "ENV", "DEPLOY MODE", "PROMOTE GATES", "NAMESPACE", "STATUS"}
	rows := stageRows(stages, styled)

	var err error
	if styled {
		err = output.PrintStyledTable(w, headers, rows)
	} else {
		err = output.PrintTable(w, headers, rows)
	}

	if err != nil {
		return err
	}

	return printStageMessages(w, stages, styled)
}

// printStageMessages lists the status message of every stage that carries
// one, so a failed environment explains itself below the table.
func printStageMessages(w io.Writer, stages []portal.Stage, styled bool) error {
	lines := make([]string, 0, len(stages))

	for _, s := range stages {
		if s.DetailedMessage != "" {
			lines = append(lines, fmt.Sprintf("  %s: %s", s.Name, output.SingleLine(s.DetailedMessage)))
		}
	}

	if len(lines) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := printSectionHeading(w, styled, "Messages"); err != nil {
		return err
	}

	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}

	return nil
}

// printSectionHeading writes "<text>:", in LabelStyle when styled.
func printSectionHeading(w io.Writer, styled bool, text string) error {
	heading := text + ":"
	if styled {
		heading = output.LabelStyle.Render(heading)
	}

	_, err := fmt.Fprintln(w, heading)

	return err
}

// stageRows builds table rows from stages. When styled is true, status is colorized.
func stageRows(stages []portal.Stage, styled bool) [][]string {
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
func summarizeGates(gates []portal.QualityGate) string {
	if len(gates) == 0 {
		return "—"
	}

	var autotests, manual int

	for _, g := range gates {
		switch g.Type {
		case portal.QualityGateTypeAutotests:
			autotests++
		case portal.QualityGateTypeManual:
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
