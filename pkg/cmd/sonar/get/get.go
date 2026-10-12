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
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
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
		render.OutputFlagUsage)

	return cmd
}

func getRun(ctx context.Context, opts *GetOptions) error {
	client, err := opts.RestClient()
	if err != nil {
		return render.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewSonarService(client)

	detail, err := svc.Get(ctx, opts.Project, portal.SonarScope{
		PullRequest: opts.PullRequest,
		Branch:      opts.Branch,
	})
	if err != nil {
		return render.HandleError(opts.IO, opts.OutputFormat, err)
	}

	return render.Render(opts.IO, opts.OutputFormat, detail, func(w io.Writer, isTTY bool) error {
		return printDetail(w, detail, isTTY)
	})
}

func printDetail(w io.Writer, d *portal.SonarProjectDetail, styled bool) error {
	gate := cmp.Or(d.QualityGateStatus, portal.QualityGateNone)

	header := []output.DetailLine{
		{Label: "Project", Value: d.Key},
		{Label: "Name", Value: d.Name},
		{Label: "Visibility", Value: cmp.Or(d.Visibility, "—")},
		{Label: "Last run", Value: formatLastRun(d.LastAnalysisDate)},
		{Label: "Revision", Value: cmp.Or(d.Revision, "—")},
		{Label: "Quality Gate", Value: string(gate), Styled: output.SonarGateStatusColor(gate)},
	}

	if err := output.PrintAligned(w, styled, header, "", 0); err != nil {
		return err
	}

	for _, section := range measureSections {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}

		if err := output.PrintHeading(w, styled, section.title); err != nil {
			return err
		}

		pairs := make([]output.DetailLine, 0, len(section.lines))
		for _, ml := range section.lines {
			raw, ok := d.Measures[ml.key]
			pairs = append(pairs, output.DetailLine{Label: ml.label, Value: formatMeasure(raw, ok, ml)})
		}

		if err := output.PrintAligned(w, styled, pairs, "  ", 0); err != nil {
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
