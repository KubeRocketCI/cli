// Package get implements the "krci sonar get" command.
package get

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	sonarinternal "github.com/KubeRocketCI/cli/pkg/cmd/sonar/internal"
)

// GetOptions holds all inputs for `krci sonar get <project>`.
type GetOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Project      string
	PullRequest  string
	Branch       string
	OutputFormat string
}

// NewCmdGet returns the "sonar get" cobra.Command.
func NewCmdGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
	}

	cmd := &cobra.Command{
		Use:   "get <project>",
		Short: "Show a SonarQube project's metadata, measures, and gate",
		Args: cmdutil.ExactArgs(1, "a SonarQube project key",
			"to see available projects: krci sonar list"),
		Example: `  # Default branch
  krci sonar get payments-api

  # Scope to a pull request
  krci sonar get payments-api --pr 123

  # Scope to a named branch (mirrors Sonar's ?branch= URL param)
  krci sonar get payments-api --branch main

  # Scripting — pull a single metric
  krci sonar get payments-api -o json | jq -r '.data.measures.coverage'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Project = args[0]

			if err := sonarinternal.ValidateProjectCommand(
				cmd, opts.OutputFormat, opts.Project, opts.PullRequest, opts.Branch,
			); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return getRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.PullRequest, "pr", "",
		"SonarQube pull-request id (mutually exclusive with --branch)")
	cmd.Flags().StringVar(&opts.Branch, "branch", "",
		"SonarQube branch name (mutually exclusive with --pr)")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func getRun(ctx context.Context, opts *GetOptions) error {
	client, err := opts.RestClient()
	if err != nil {
		return sonarinternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewSonarService(client)

	detail, err := svc.Get(ctx, opts.Project, portal.SonarScope{
		PullRequest: opts.PullRequest,
		Branch:      opts.Branch,
	})
	if err != nil {
		return sonarinternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	return sonarinternal.Render(opts.IO, opts.OutputFormat, detail, func(w io.Writer, isTTY bool) error {
		return printDetail(w, detail, isTTY)
	})
}

// kvPair is one aligned "label: value" row in the `get` output. display holds
// the optional styled rendering of the value (used for the Quality Gate color);
// when display is empty, value is rendered verbatim.
type kvPair struct {
	label   string
	value   string
	display string
}

func printDetail(w io.Writer, d *portal.SonarProjectDetail, styled bool) error {
	gate := cmp.Or(d.QualityGateStatus, portal.QualityGateNone)
	gateDisplay := ""
	if styled {
		gateDisplay = output.SonarGateStatusColor(gate)
	}

	header := []kvPair{
		{label: "Project", value: d.Key},
		{label: "Name", value: d.Name},
		{label: "Visibility", value: cmp.Or(d.Visibility, "—")},
		{label: "Last run", value: formatLastRun(d.LastAnalysisDate)},
		{label: "Revision", value: cmp.Or(d.Revision, "—")},
		{label: "Quality Gate", value: string(gate), display: gateDisplay},
	}

	if err := printAligned(w, header, "", styled); err != nil {
		return err
	}

	for _, section := range measureSections {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}

		if err := printHeading(w, section.title, styled); err != nil {
			return err
		}

		pairs := make([]kvPair, 0, len(section.lines))
		for _, ml := range section.lines {
			raw, ok := d.Measures[ml.key]
			pairs = append(pairs, kvPair{label: ml.label, value: formatMeasure(raw, ok, ml)})
		}

		if err := printAligned(w, pairs, "  ", styled); err != nil {
			return err
		}
	}

	return nil
}

// measureSections is the static, read-only grouped metric layout used by the
// `get` TTY renderer.
var measureSections = []measureSection{
	{
		title: "Reliability",
		lines: []measureLine{
			{label: "Bugs", key: "bugs"},
			{label: "Reliability rating", key: "reliability_rating", rating: true},
		},
	},
	{
		title: "Security",
		lines: []measureLine{
			{label: "Vulnerabilities", key: "vulnerabilities"},
			{label: "Security rating", key: "security_rating", rating: true},
			{label: "Security hotspots", key: "security_hotspots"},
			{label: "Hotspots reviewed", key: "security_hotspots_reviewed", percent: true},
			{label: "Security review rating", key: "security_review_rating", rating: true},
		},
	},
	{
		title: "Maintainability",
		lines: []measureLine{
			{label: "Code smells", key: "code_smells"},
			{label: "Maintainability rating", key: "sqale_rating", rating: true},
		},
	},
	{
		title: "Coverage & Size",
		lines: []measureLine{
			{label: "Line coverage", key: "coverage", percent: true},
			{label: "Duplicated lines", key: "duplicated_lines_density", percent: true},
			{label: "Lines of code", key: "ncloc"},
		},
	},
}

// printAligned prints "<indent><label padded to column-width>   <value>" for
// each pair. The column width is the max label length in the block so every
// value aligns regardless of label length. Reuses `output.LabelStyle` via
// `UnsetWidth()` to match the sibling `project` / `deployment` get views
// while letting this renderer own the column-width decision.
// sonarLabelStyle reuses the shared LabelStyle but lets printAligned control
// the column width dynamically via its own padding logic.
var sonarLabelStyle = output.LabelStyle.UnsetWidth()

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
			_, err = fmt.Fprintf(w, "%s%s%*s   %s\n", indent, sonarLabelStyle.Render(p.label), pad, "", value)
		} else {
			_, err = fmt.Fprintf(w, "%s%s%*s   %s\n", indent, p.label, pad, "", value)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// printHeading renders a section heading. Styled mode uses the shared
// `output.HeaderStyle` (accent color + bold) so the look matches the top-of-
// table styling elsewhere in the CLI. Plain mode appends ":" for readability
// in piped output.
func printHeading(w io.Writer, title string, styled bool) error {
	if styled {
		_, err := fmt.Fprintln(w, output.HeaderStyle.Render(title))
		return err
	}

	_, err := fmt.Fprintln(w, title+":")

	return err
}

type measureLine struct {
	label   string
	key     string
	rating  bool
	percent bool
}

type measureSection struct {
	title string
	lines []measureLine
}

// formatMeasure returns the display string for a measure with optional
// rating translation (1..5 → A..E) or percent formatting.
func formatMeasure(raw string, present bool, ml measureLine) string {
	if !present || raw == "" {
		return "—"
	}

	switch {
	case ml.rating:
		return ratingLetter(raw)
	case ml.percent:
		return formatPercent(raw)
	default:
		return raw
	}
}

// ratingLetter converts Sonar's numeric rating (e.g. "1.0") to a letter A..E.
func ratingLetter(raw string) string {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw
	}

	n := int(v + 0.0001)

	switch n {
	case 1:
		return "A"
	case 2:
		return "B"
	case 3:
		return "C"
	case 4:
		return "D"
	case 5:
		return "E"
	default:
		return raw
	}
}

// formatPercent turns "84.2" into "84.2%".
func formatPercent(raw string) string {
	if _, err := strconv.ParseFloat(raw, 64); err != nil {
		return raw
	}

	return raw + "%"
}

// formatLastRun renders a Sonar analysisDate (`2006-01-02T15:04:05-0700` or
// RFC3339) as `2006-01-02 15:04 UTC (Xh ago)`. Returns "—" for empty input
// and the raw string for unparseable input.
func formatLastRun(raw string) string {
	if raw == "" {
		return "—"
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05",
	}

	var (
		t   time.Time
		err error
	)

	for _, layout := range layouts {
		t, err = time.Parse(layout, raw)
		if err == nil {
			break
		}
	}

	if err != nil {
		return raw
	}

	abs := t.UTC().Format("2006-01-02 15:04 UTC")
	rel := output.RelativeTime(t)

	if rel == "" {
		return abs
	}

	return fmt.Sprintf("%s (%s)", abs, rel)
}
