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
		Use:   "get <codebase>",
		Short: "Show a Dep-Track project's overview for a codebase",
		Args: cmdutil.ExactArgs(1, "a KubeRocketCI codebase name",
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
		"Output format: table, json (default: table)")

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

	return scainternal.Render(opts.IO, opts.OutputFormat, detail, func(w io.Writer, isTTY bool) error {
		return printDetail(w, opts.Codebase, opts.Branch, detail, isTTY)
	})
}

// kvPair is one aligned "label: value" row in the `get` output. display holds
// the optional styled rendering of the value (used for coloured severity
// badges); when display is empty, value is rendered verbatim.
type kvPair struct {
	label   string
	value   string
	display string
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
	if _, err := fmt.Fprintf(w, "%s @ %s\n\n", project.Name, project.Version); err != nil {
		return err
	}

	pairs := []kvPair{
		{label: "Codebase", value: codebase},
		{label: "Branch", value: project.Version},
		{label: "Classifier", value: scainternal.OrDash(project.Classifier)},
		{label: "Active", value: formatActive(project.Active, styled)},
		{label: "Is latest", value: scainternal.BoolYesNo(project.IsLatest)},
		{label: "Risk score", value: scainternal.FormatRisk(project.RiskScore)},
		{label: "Last BOM", value: formatTimestamp(project.LastBomImport, project.LastBomImportFormat)},
		{label: "Last vuln scan", value: formatRelativeTimestamp(project.LastVulnerabilityAnalysis)},
	}
	if err := printAligned(w, pairs, "", styled); err != nil {
		return err
	}

	if d.Metrics == nil {
		return nil
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := printHeading(w, "Vulnerabilities", styled); err != nil {
		return err
	}

	vulnPairs := []kvPair{
		severityRow("Critical", "CRITICAL", d.Metrics.Critical, styled),
		severityRow("High", "HIGH", d.Metrics.High, styled),
		severityRow("Medium", "MEDIUM", d.Metrics.Medium, styled),
		severityRow("Low", "LOW", d.Metrics.Low, styled),
		severityRow("Info/Unassigned", "INFO", d.Metrics.Unassigned, styled),
		{label: "Total", value: fmt.Sprintf("%d", d.Metrics.Vulnerabilities)},
	}
	if err := printAligned(w, vulnPairs, "  ", styled); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := printHeading(w, "Components", styled); err != nil {
		return err
	}

	compPairs := []kvPair{
		{label: "Total", value: fmt.Sprintf("%d", d.Metrics.Components)},
		{label: "Vulnerable", value: fmt.Sprintf("%d", d.Metrics.VulnerableComponents)},
	}
	return printAligned(w, compPairs, "  ", styled)
}

// severityRow builds a "label: count" kvPair. When styled && count>0, the
// value is augmented with the coloured canonical severity badge so the user
// sees e.g. "3 CRITICAL" in red.
func severityRow(label, canonical string, count int, styled bool) kvPair {
	p := kvPair{label: label, value: fmt.Sprintf("%d", count)}
	if styled && count > 0 && canonical != "" {
		p.display = fmt.Sprintf("%s %s", p.value, output.SCASeverityColor(canonical))
	}
	return p
}

func formatActive(active, styled bool) string {
	if active {
		if styled {
			return output.GreenText("yes")
		}
		return "yes"
	}
	return "no"
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

// printAligned / printHeading mirror the sonar helpers verbatim so the two
// groups render with the same visual rhythm.
var scaLabelStyle = output.LabelStyle.UnsetWidth()

func printAligned(w io.Writer, pairs []kvPair, indent string, styled bool) error {
	maxLabel := 0
	for _, p := range pairs {
		if len(p.label) > maxLabel {
			maxLabel = len(p.label)
		}
	}

	for _, p := range pairs {
		pad := maxLabel - len(p.label)
		value := p.display
		if value == "" {
			value = p.value
		}

		var err error
		if styled {
			_, err = fmt.Fprintf(w, "%s%s%*s   %s\n", indent, scaLabelStyle.Render(p.label), pad, "", value)
		} else {
			_, err = fmt.Fprintf(w, "%s%s%*s   %s\n", indent, p.label, pad, "", value)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func printHeading(w io.Writer, title string, styled bool) error {
	if styled {
		_, err := fmt.Fprintln(w, output.HeaderStyle.Render(title))
		return err
	}

	_, err := fmt.Fprintln(w, title+":")
	return err
}
