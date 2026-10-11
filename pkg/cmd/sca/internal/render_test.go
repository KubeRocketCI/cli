package scainternal

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/iostreams"
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

func TestFormatVulnCounts(t *testing.T) {
	t.Parallel()
	if got := FormatVulnCounts(nil, false); got != "0/0/0/0" {
		t.Errorf("nil metrics = %q; want 0/0/0/0", got)
	}
	m := &portal.SCAMetrics{Critical: 1, High: 2, Medium: 3, Low: 4}
	if got := FormatVulnCounts(m, false); got != "1/2/3/4" {
		t.Errorf("non-TTY = %q; want 1/2/3/4", got)
	}
	// TTY mode wraps in YellowText when critical+high > 0; just assert the
	// digits remain present (we don't depend on the exact ANSI sequence).
	if got := FormatVulnCounts(m, true); !strings.Contains(got, "1/2/3/4") {
		t.Errorf("TTY render must still contain digits, got %q", got)
	}
}

func TestPageFooter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		noun                         string
		total, pageIndex, pageSize   int
		wantSubstr                   string
		wantSingular                 bool
		wantBeyondLastPage, wantNone bool
	}{
		{"project", 0, 1, 25, "no projects", false, false, true},
		{"project", 1, 1, 25, "1 project,", true, false, false},
		{"project", 42, 2, 25, "42 projects, page 2 of 2", false, false, false},
		{"project", 42, 99, 25, "beyond last page", false, true, false},
	}
	for _, c := range cases {
		got := PageFooter(c.noun, c.total, c.pageIndex, c.pageSize)
		if !strings.Contains(got, c.wantSubstr) {
			t.Errorf("PageFooter(%s,%d,%d,%d) = %q; want substr %q",
				c.noun, c.total, c.pageIndex, c.pageSize, got, c.wantSubstr)
		}
	}
}
