package get

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

const emDash = "—"

func newFactory() *cmdutil.Factory {
	return &cmdutil.Factory{
		IOStreams: &iostreams.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
		RestClient: func() (*restapi.ClientWithResponses, error) {
			return nil, nil
		},
	}
}

func TestGet_RequiresProject(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when no project arg is given")
	}
}

func TestGet_RejectsInvalidProject(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{"UPPER_CASE_BAD"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "DNS-1123") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGet_RatingLetter(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"1.0":   "A",
		"2.0":   "B",
		"3.0":   "C",
		"4.0":   "D",
		"5.0":   "E",
		"bogus": "bogus",
		"7.0":   "7.0",
		"":      "",
	}

	for in, want := range cases {
		if got := ratingLetter(in); got != want {
			t.Errorf("ratingLetter(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGet_FormatPercent(t *testing.T) {
	t.Parallel()

	if got := formatPercent("84.2"); got != "84.2%" {
		t.Errorf("formatPercent(84.2) = %q", got)
	}

	if got := formatPercent("bogus"); got != "bogus" {
		t.Errorf("formatPercent(bogus) = %q", got)
	}
}

func TestGet_FormatLastRun(t *testing.T) {
	t.Parallel()

	if got := formatLastRun(""); got != emDash {
		t.Errorf("empty input: got %q, want —", got)
	}

	// SonarQube format (no colon in timezone)
	got := formatLastRun("2026-04-18T12:17:59+0000")
	if !strings.Contains(got, "2026-04-18 12:17 UTC") {
		t.Errorf("sonar format: got %q, want to contain 2026-04-18 12:17 UTC", got)
	}

	// RFC3339 format
	got = formatLastRun("2026-04-18T12:17:59Z")
	if !strings.Contains(got, "2026-04-18 12:17 UTC") {
		t.Errorf("rfc3339: got %q, want to contain 2026-04-18 12:17 UTC", got)
	}

	// Unparseable input — returned verbatim
	if got := formatLastRun("garbage"); got != "garbage" {
		t.Errorf("unparseable: got %q, want garbage", got)
	}
}

func TestGet_FormatMeasure(t *testing.T) {
	t.Parallel()

	if got := formatMeasure("", false, measureLine{}); got != emDash {
		t.Errorf("missing measure should show em-dash, got %q", got)
	}
	if got := formatMeasure("2", true, measureLine{}); got != "2" {
		t.Errorf("plain measure = %q", got)
	}
	if got := formatMeasure("1.0", true, measureLine{rating: true}); got != "A" {
		t.Errorf("rating = %q", got)
	}
	if got := formatMeasure("84.2", true, measureLine{percent: true}); got != "84.2%" {
		t.Errorf("percent = %q", got)
	}
}

func TestGet_RatingLetterAllValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"1.0", "A"},
		{"1", "A"},
		{"2.0", "B"},
		{"3.0", "C"},
		{"4.0", "D"},
		{"5.0", "E"},
		{"6.0", "6.0"},
		{"0.0", "0.0"},
		{"bogus", "bogus"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			if got := ratingLetter(tc.input); got != tc.want {
				t.Errorf("ratingLetter(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGet_FormatPercentInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"0", "0%"},
		{"100", "100%"},
		{"84.2", "84.2%"},
		{"not-a-number", "not-a-number"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			if got := formatPercent(tc.input); got != tc.want {
				t.Errorf("formatPercent(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGet_FormatLastRun_PlainDateLayout(t *testing.T) {
	t.Parallel()

	// SonarQube layout without seconds
	got := formatLastRun("2026-04-18T12:17:59")
	if !strings.Contains(got, "2026-04-18 12:17 UTC") {
		t.Errorf("plain datetime layout: got %q, want to contain '2026-04-18 12:17 UTC'", got)
	}
}

func TestGet_ValidInvocationReachesRunF(t *testing.T) {
	t.Parallel()

	called := false
	cmd := NewCmdGet(newFactory(), func(opts *GetOptions) error {
		called = true

		if opts.Project != "my-proj" {
			t.Errorf("Project = %q, want %q", opts.Project, "my-proj")
		}

		if opts.PullRequest != "99" {
			t.Errorf("PullRequest = %q, want %q", opts.PullRequest, "99")
		}

		if opts.OutputFormat != "json" {
			t.Errorf("OutputFormat = %q, want %q", opts.OutputFormat, "json")
		}

		return nil
	})

	cmd.SetArgs([]string{"my-proj", "--pr", "99", "-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not called")
	}
}

func TestGet_RejectsEmptyPullRequest(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--pr", ""})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty --pr")
	}

	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGet_BranchReachesRunF(t *testing.T) {
	t.Parallel()

	called := false
	cmd := NewCmdGet(newFactory(), func(opts *GetOptions) error {
		called = true

		if opts.Branch != "main" {
			t.Errorf("Branch = %q, want %q", opts.Branch, "main")
		}
		if opts.PullRequest != "" {
			t.Errorf("PullRequest must stay empty when only --branch is set, got %q", opts.PullRequest)
		}

		return nil
	})

	cmd.SetArgs([]string{"my-proj", "--branch", "main"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not called")
	}
}

func TestGet_RejectsEmptyBranch(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--branch", ""})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty --branch")
	}

	if !strings.Contains(err.Error(), "--branch requires a non-empty value") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGet_RejectsBothScopes(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--pr", "42", "--branch", "main"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when both scope flags are set")
	}

	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGet_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "-o", "yaml"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for -o yaml")
	}

	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGet_PrintDetail_PlainOutput(t *testing.T) {
	t.Parallel()

	d := &portal.SonarProjectDetail{
		Key:               "payments-api",
		Name:              "Payments API",
		Visibility:        "public",
		LastAnalysisDate:  "2026-04-18T12:17:59+0000",
		Revision:          "abc123",
		QualityGateStatus: portal.QualityGateOK,
		Measures: map[string]string{
			"bugs":                     "3",
			"reliability_rating":       "2.0",
			"vulnerabilities":          "0",
			"security_rating":          "1.0",
			"coverage":                 "84.2",
			"duplicated_lines_density": "1.5",
			"ncloc":                    "1234",
		},
	}

	var buf bytes.Buffer
	if err := printDetail(&buf, d, false); err != nil {
		t.Fatalf("printDetail error: %v", err)
	}

	out := buf.String()

	wantSubstrings := []string{
		"payments-api",
		"Payments API",
		"public",
		"abc123",
		"OK",
		"Reliability",
		"Security",
		"Maintainability",
		"Coverage",
	}

	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, not found in:\n%s", want, out)
		}
	}
}

func TestGet_PrintDetail_EmptyMeasures(t *testing.T) {
	t.Parallel()

	d := &portal.SonarProjectDetail{
		Key:               "my-proj",
		Name:              "My Project",
		QualityGateStatus: portal.QualityGateNone,
		Measures:          map[string]string{},
	}

	var buf bytes.Buffer
	if err := printDetail(&buf, d, false); err != nil {
		t.Fatalf("printDetail error: %v", err)
	}

	out := buf.String()

	// All measures absent → em-dash displayed
	if !strings.Contains(out, emDash) {
		t.Errorf("expected em-dash for missing measures, got:\n%s", out)
	}
}

func TestGet_PrintAligned_NonEmpty(t *testing.T) {
	t.Parallel()

	pairs := []kvPair{
		{label: "Short", value: "val1"},
		{label: "A much longer label", value: "val2"},
	}

	var buf bytes.Buffer
	if err := printAligned(&buf, pairs, "  ", false); err != nil {
		t.Fatalf("printAligned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Short") || !strings.Contains(out, "val1") {
		t.Errorf("printAligned output missing expected content: %s", out)
	}

	if !strings.Contains(out, "A much longer label") || !strings.Contains(out, "val2") {
		t.Errorf("printAligned output missing expected content: %s", out)
	}
}

func TestGet_PrintAligned_WithDisplay(t *testing.T) {
	t.Parallel()

	pairs := []kvPair{
		{label: "Gate", value: "OK", display: "[OK-styled]"},
	}

	var buf bytes.Buffer
	if err := printAligned(&buf, pairs, "", false); err != nil {
		t.Fatalf("printAligned error: %v", err)
	}

	// In non-styled mode, display field is ignored; value is used
	out := buf.String()
	if !strings.Contains(out, "OK") {
		t.Errorf("expected value in output: %s", out)
	}
}

func TestGet_PrintHeading(t *testing.T) {
	t.Parallel()

	t.Run("plain mode appends colon", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		if err := printHeading(&buf, "Reliability", false); err != nil {
			t.Fatalf("printHeading error: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Reliability:") {
			t.Errorf("expected 'Reliability:' in plain output, got: %s", out)
		}
	})

	t.Run("styled mode does not append colon", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		if err := printHeading(&buf, "Reliability", true); err != nil {
			t.Fatalf("printHeading error: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Reliability") {
			t.Errorf("expected 'Reliability' in styled output, got: %s", out)
		}
	})
}
