package scainternal

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
)

type testPayload struct {
	Message string `json:"message"`
}

func newStreams() (*iostreams.IOStreams, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	return &iostreams.IOStreams{Out: out, ErrOut: errOut}, out
}

func TestRender_JSONEnvelope(t *testing.T) {
	t.Parallel()

	ios, out := newStreams()
	payload := testPayload{Message: "hello"}
	if err := Render(ios, "json", payload, func(io.Writer, bool) error {
		t.Fatal("table renderer must not run for -o json")
		return nil
	}); err != nil {
		t.Fatalf("Render error: %v", err)
	}

	var env struct {
		SchemaVersion string      `json:"schemaVersion"`
		Data          testPayload `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion = %q; want %q", env.SchemaVersion, SchemaVersion)
	}
	if env.Data.Message != "hello" {
		t.Errorf("data.message = %q; want hello", env.Data.Message)
	}
}

func TestRender_TableInvokesCallback(t *testing.T) {
	t.Parallel()

	ios, out := newStreams()
	called := false
	err := Render(ios, "", testPayload{Message: "x"}, func(w io.Writer, isTTY bool) error {
		called = true
		_, _ = w.Write([]byte("ok"))
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !called {
		t.Fatal("table callback must run for default format")
	}
	if out.String() != "ok" {
		t.Errorf("out = %q", out.String())
	}
}

func TestRender_UnknownFormatErrors(t *testing.T) {
	t.Parallel()
	ios, _ := newStreams()
	err := Render(ios, "yaml", testPayload{}, func(io.Writer, bool) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("expected unknown format error, got %v", err)
	}
}

func TestHandleError_UnauthorizedPromotes(t *testing.T) {
	t.Parallel()
	ios, _ := newStreams()
	got := HandleError(ios, "", portal.ErrUnauthorized)
	if got == nil || !strings.Contains(got.Error(), "krci auth login") {
		t.Errorf("expected auth-login message, got %v", got)
	}
}

func TestHandleError_UpstreamUnavailableAnnotates(t *testing.T) {
	t.Parallel()
	ios, _ := newStreams()
	got := HandleError(ios, "", portal.ErrUpstreamUnavailable)
	if got == nil || !strings.Contains(got.Error(), "dependency-track") {
		t.Errorf("expected dep-track hint, got %v", got)
	}
	if !errors.Is(got, portal.ErrUpstreamUnavailable) {
		t.Error("wrap must preserve ErrUpstreamUnavailable")
	}
}

func TestHandleError_JSONEnvelopePrinted(t *testing.T) {
	t.Parallel()
	ios, out := newStreams()
	_ = HandleError(ios, "json", errors.New("kaboom"))

	var env struct {
		SchemaVersion string `json:"schemaVersion"`
		Error         struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion = %q", env.SchemaVersion)
	}
	if env.Error.Message != "kaboom" {
		t.Errorf("error.message = %q", env.Error.Message)
	}
}

func TestHandleError_PlainTableFormatDoesNotEmitJSONEnvelope(t *testing.T) {
	t.Parallel()
	ios, out := newStreams()
	_ = HandleError(ios, "table", errors.New("kaboom"))
	if out.Len() != 0 {
		t.Errorf("table format must not write JSON envelope to stdout, got %q", out.String())
	}
}

func TestPageCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		total, pageSize, want int
	}{
		{0, 25, 1},
		{1, 25, 1},
		{25, 25, 1},
		{26, 25, 2},
		{50, 25, 2},
		{51, 25, 3},
		{100, 0, 1}, // defensive: zero page size → 1
	}
	for _, c := range cases {
		if got := PageCount(c.total, c.pageSize); got != c.want {
			t.Errorf("PageCount(%d,%d) = %d want %d", c.total, c.pageSize, got, c.want)
		}
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
