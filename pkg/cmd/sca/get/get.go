// Package get implements the "krci sca get" command.
package get

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
	scainternal "github.com/KubeRocketCI/cli/pkg/cmd/sca/internal"
)

// GetOptions holds all inputs for `krci sca get <codebase>`.
type GetOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Codebase     string
	Branch       string
	OutputFormat string
}

// NewCmdGet returns the "sca get" cobra.Command.
func NewCmdGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
	}

	cmd := &cobra.Command{
		Use:   "get <project>",
		Short: "Show SCA scan overview for a project",
		Args: cmdutil.ExactArgs(1, "a KubeRocketCI project name",
			"to see available projects: krci sca list"),
		Example: `  # Uses Codebase.spec.defaultBranch when --branch is omitted
  krci sca get payments-api

  # Explicit branch / release tag
  krci sca get payments-api --branch=release/1.0

  # Scripting — risk score for the default branch
  krci sca get payments-api -o json | jq -r '.data.project.riskScore'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Codebase = args[0]

			if err := scainternal.ValidateCodebaseCommand(
				cmd, opts.OutputFormat, opts.Codebase, opts.Branch,
			); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return getRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Branch, "branch", "", scainternal.BranchFlagUsage)
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		render.OutputFlagUsage)

	return cmd
}

func getRun(ctx context.Context, opts *GetOptions) error {
	client, err := opts.RestClient()
	if err != nil {
		return scainternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	detail, err := portal.NewSCAService(client).Get(ctx, opts.Codebase, opts.Branch)
	if err != nil {
		return scainternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	return render.Render(opts.IO, opts.OutputFormat, detail, func(w io.Writer, isTTY bool) error {
		return printDetail(w, opts.Codebase, opts.Branch, detail, isTTY)
	})
}

func printDetail(w io.Writer, codebase, requestedBranch string, d *portal.SCAProjectDetail, styled bool) error {
	if d.Status == portal.SCAStatusNone {
		branchLabel := cmp.Or(requestedBranch, "default branch")
		if _, err := fmt.Fprintf(w, "status: NONE — no SCA scanner bound for %s @ %s\n", codebase, branchLabel); err != nil {
			return err
		}
		return nil
	}

	if d.Project == nil {
		return fmt.Errorf("portal returned status=OK without project")
	}

	project := d.Project

	// Header line shows which branch was queried; when --branch was omitted,
	// the Portal fills in the resolved default and returns it verbatim.
	name, version := output.SingleLine(project.Name), output.SingleLine(project.Version)
	if _, err := fmt.Fprintf(w, "%s @ %s\n\n", name, version); err != nil {
		return err
	}

	pairs := []output.DetailLine{
		{Label: "Project", Value: codebase},
		{Label: "Branch", Value: project.Version},
		{Label: "Classifier", Value: scainternal.OrDash(project.Classifier)},
		activeLine(project.Active),
		{Label: "Is latest", Value: scainternal.BoolYesNo(project.IsLatest)},
		{Label: "Risk score", Value: scainternal.FormatRisk(project.RiskScore)},
		{Label: "Last BOM", Value: formatTimestamp(project.LastBomImport, project.LastBomImportFormat)},
		{Label: "Last vuln scan", Value: formatRelativeTimestamp(project.LastVulnerabilityAnalysis)},
	}
	if err := output.PrintAligned(w, styled, pairs, "", 0); err != nil {
		return err
	}

	if d.Metrics == nil {
		return nil
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := output.PrintHeading(w, styled, "Vulnerabilities"); err != nil {
		return err
	}

	vulnPairs := []output.DetailLine{
		severityRow("Critical", "CRITICAL", d.Metrics.Critical),
		severityRow("High", "HIGH", d.Metrics.High),
		severityRow("Medium", "MEDIUM", d.Metrics.Medium),
		severityRow("Low", "LOW", d.Metrics.Low),
		severityRow("Info/Unassigned", "INFO", d.Metrics.Unassigned),
		{Label: "Total", Value: fmt.Sprintf("%d", d.Metrics.Vulnerabilities)},
	}
	if err := output.PrintAligned(w, styled, vulnPairs, "  ", 0); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := output.PrintHeading(w, styled, "Components"); err != nil {
		return err
	}

	compPairs := []output.DetailLine{
		{Label: "Total", Value: fmt.Sprintf("%d", d.Metrics.Components)},
		{Label: "Vulnerable", Value: fmt.Sprintf("%d", d.Metrics.VulnerableComponents)},
	}
	return output.PrintAligned(w, styled, compPairs, "  ", 0)
}

// severityRow builds a "label: count" detail line. For count>0 and a known
// severity, Styled appends the coloured severity badge, e.g. "3 CRITICAL" in
// red.
func severityRow(label, canonical string, count int) output.DetailLine {
	p := output.DetailLine{Label: label, Value: fmt.Sprintf("%d", count)}
	if count > 0 && canonical != "" {
		p.Styled = output.Concat(output.Text(p.Value+" "), output.SCASeverityColor(canonical))
	}
	return p
}

// activeLine renders the Active flag as "yes" or "no"; Styled shows "yes" in
// green.
func activeLine(active bool) output.DetailLine {
	l := output.DetailLine{Label: "Active", Value: scainternal.BoolYesNo(active)}
	if active {
		l.Styled = output.Green(l.Value)
	}
	return l
}

func formatTimestamp(ms int64, format string) string {
	if ms <= 0 {
		return scainternal.EmptyPlaceholder
	}
	t := time.UnixMilli(ms).UTC()
	rel := output.RelativeTime(t)
	base := t.Format("2006-01-02 15:04 UTC")
	if format != "" {
		base = fmt.Sprintf("%s (%s)", base, format)
	}
	if rel == "" {
		return base
	}
	return fmt.Sprintf("%s, %s", base, rel)
}

func formatRelativeTimestamp(ms int64) string {
	if ms <= 0 {
		return scainternal.EmptyPlaceholder
	}
	return output.RelativeTime(time.UnixMilli(ms))
}
