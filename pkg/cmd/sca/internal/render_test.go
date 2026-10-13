package scainternal

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

func newStreams() *iostreams.IOStreams {
	return &iostreams.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
}

func TestHandleError_UpstreamUnavailableAnnotates(t *testing.T) {
	t.Parallel()
	ios := newStreams()
	got := HandleError(ios, "", portal.ErrUpstreamUnavailable)
	if got == nil || !strings.Contains(got.Error(), "dependency-track") {
		t.Errorf("expected dep-track hint, got %v", got)
	}
	if !errors.Is(got, portal.ErrUpstreamUnavailable) {
		t.Error("wrap must preserve ErrUpstreamUnavailable")
	}
}

func TestBoolYesNo(t *testing.T) {
	t.Parallel()
	if BoolYesNo(true) != "yes" || BoolYesNo(false) != "no" {
		t.Errorf("BoolYesNo: true=%q false=%q", BoolYesNo(true), BoolYesNo(false))
	}
}

func TestFormatRisk(t *testing.T) {
	t.Parallel()
	if FormatRisk(0) != EmptyPlaceholder {
		t.Errorf("zero must render as %q, got %q", EmptyPlaceholder, FormatRisk(0))
	}
	if got := FormatRisk(12.345); !strings.Contains(got, "12.3") {
		t.Errorf("FormatRisk(12.345) = %q; want substring 12.3", got)
	}
}

func TestSeverityColor(t *testing.T) {
	t.Parallel()

	cases := map[string]output.Cell{
		"CRITICAL":     output.Red("CRITICAL"),
		"critical":     output.Red("critical"),
		"HIGH":         output.Yellow("HIGH"),
		"MEDIUM":       output.Blue("MEDIUM"),
		"LOW":          output.Gray("LOW"),
		"INFO":         output.Gray("INFO"),
		"UNASSIGNED":   output.Gray("UNASSIGNED"),
		"BOGUS\x1b[2J": output.Text("BOGUS\x1b[2J"),
		"":             {},
	}

	for severity, want := range cases {
		if got := SeverityColor(severity); got != want {
			t.Errorf("SeverityColor(%q) = %#v, want %#v", severity, got, want)
		}
	}
}

func TestFormatVulnCounts(t *testing.T) {
	t.Parallel()
	if got := FormatVulnCounts(nil); got != output.Text("0/0/0/0") {
		t.Errorf("nil metrics = %#v; want 0/0/0/0", got)
	}
	m := &portal.SCAMetrics{Critical: 1, High: 2, Medium: 3, Low: 4}
	if got := FormatVulnCounts(m); got != output.Yellow("1/2/3/4") {
		t.Errorf("critical+high > 0 = %#v; want yellow 1/2/3/4", got)
	}
	if got := FormatVulnCounts(&portal.SCAMetrics{Medium: 3}); got != output.Text("0/0/3/0") {
		t.Errorf("no critical or high = %#v; want plain 0/0/3/0", got)
	}
}

func TestPageFooter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                       string
		noun                       string
		total, pageIndex, pageSize int
		want                       string
	}{
		{"none", "project", 0, 1, 25, "no projects (page-size 25)"},
		{"singular", "project", 1, 1, 25, "1 project, page 1 of 1 (page-size 25)"},
		{"plural", "project", 42, 2, 25, "42 projects, page 2 of 2 (page-size 25)"},
		{"beyond last page", "project", 42, 99, 25, "42 projects — page 99 is beyond last page 2 (page-size 25)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := PageFooter(tc.noun, tc.total, tc.pageIndex, tc.pageSize); got != tc.want {
				t.Errorf("PageFooter(%q, %d, %d, %d) = %q, want %q",
					tc.noun, tc.total, tc.pageIndex, tc.pageSize, got, tc.want)
			}
		})
	}
}
