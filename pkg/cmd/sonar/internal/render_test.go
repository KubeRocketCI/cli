package sonarinternal

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
)

func newBufferedIO() (*iostreams.IOStreams, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return &iostreams.IOStreams{Out: stdout, ErrOut: stderr}, stdout
}

func TestRender_JSONEnvelope(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()

	err := Render(ios, "json", map[string]int{"bugs": 3}, func(_ io.Writer, _ bool) error {
		t.Fatal("table renderer must not run for -o json")
		return nil
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	var got struct {
		SchemaVersion string         `json:"schemaVersion"`
		Data          map[string]int `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", got.SchemaVersion, SchemaVersion)
	}
	if got.Data["bugs"] != 3 {
		t.Errorf("data.bugs = %d, want 3", got.Data["bugs"])
	}
}

func TestRender_TableDefault(t *testing.T) {
	t.Parallel()

	ios, _ := newBufferedIO()
	called := false

	err := Render(ios, "", "unused", func(_ io.Writer, _ bool) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !called {
		t.Error("table renderer was not invoked for empty (default) format")
	}
}

func TestRender_UnknownFormat(t *testing.T) {
	t.Parallel()

	ios, _ := newBufferedIO()

	err := Render(ios, "yaml", "unused", func(_ io.Writer, _ bool) error {
		t.Fatal("table renderer must not run for an unknown format")
		return nil
	})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "unknown output format: yaml") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHandleError_EmitsJSONEnvelope(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()
	boom := errors.New("boom")

	got := HandleError(ios, "json", boom)
	if got != boom {
		t.Errorf("HandleError must return the original error; got %v", got)
	}

	var env struct {
		SchemaVersion string `json:"schemaVersion"`
		Error         struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if env.Error.Message != "boom" {
		t.Errorf("error.message = %q, want boom", env.Error.Message)
	}
}

func TestHandleError_TableModeLeavesStdoutUntouched(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()

	_ = HandleError(ios, "table", errors.New("boom"))
	if stdout.Len() != 0 {
		t.Errorf("table mode must not touch stdout; got %q", stdout.String())
	}
}

func TestRender_TableCallbackReceivesNonTTY(t *testing.T) {
	t.Parallel()

	// Out is a *bytes.Buffer — not an *os.File — so IsStdoutTTY() returns false.
	ios, _ := newBufferedIO()
	var capturedTTY bool
	var rendererCalled bool

	err := Render(ios, "table", "payload", func(_ io.Writer, isTTY bool) error {
		rendererCalled = true
		capturedTTY = isTTY
		return nil
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !rendererCalled {
		t.Error("table renderer was not invoked for -o table")
	}
	if capturedTTY {
		t.Error("isTTY should be false for a bytes.Buffer writer")
	}
}

func TestHandleError_PromotesErrUnauthorized(t *testing.T) {
	t.Parallel()

	ios, _ := newBufferedIO()

	// Pass portal.ErrUnauthorized directly — HandleError must promote it.
	got := HandleError(ios, "table", portal.ErrUnauthorized)
	if got == nil {
		t.Fatal("HandleError must return an error")
	}

	// ErrAuthRequired wraps its cause, so errors.Is must still find portal.ErrUnauthorized.
	if !errors.Is(got, portal.ErrUnauthorized) {
		t.Errorf("returned error should still wrap portal.ErrUnauthorized; got %v", got)
	}

	// Confirm the promotion adds the expected "authentication required" prefix
	// (same phrasing produced by cmdutil.ErrAuthRequired).
	want := cmdutil.ErrAuthRequired(portal.ErrUnauthorized).Error()
	if got.Error() != want {
		t.Errorf("promoted error = %q, want %q", got.Error(), want)
	}
}

func TestHandleError_JSONWritesErrorEnvelopeAndReturnsSameError(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()
	boom := errors.New("something bad")

	got := HandleError(ios, "json", boom)
	if got != boom {
		t.Errorf("HandleError must return the original error; got %v", got)
	}

	var env struct {
		SchemaVersion string `json:"schemaVersion"`
		Error         struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout JSON is invalid: %v (raw: %q)", err, stdout.String())
	}
	if env.Error.Message != "something bad" {
		t.Errorf("error.message = %q, want %q", env.Error.Message, "something bad")
	}
}

func TestPrintTable_NonTTY(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	headers := []string{"NAME", "STATUS"}
	rows := [][]string{{"foo", "OK"}, {"bar", "ERROR"}}

	if err := PrintTable(&buf, false, headers, rows); err != nil {
		t.Fatalf("PrintTable(isTTY=false) returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "foo") {
		t.Errorf("plain table output missing expected content: %q", out)
	}
}

func TestPrintTable_TTY(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	headers := []string{"NAME", "STATUS"}
	rows := [][]string{{"foo", "OK"}}

	// Styled path: just verify no error; we do not assert ANSI codes.
	if err := PrintTable(&buf, true, headers, rows); err != nil {
		t.Fatalf("PrintTable(isTTY=true) returned error: %v", err)
	}
}

func TestPageCount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		total, pageSize, want int
	}{
		{0, 50, 1},
		{1, 50, 1},
		{50, 50, 1},
		{51, 50, 2},
		{100, 25, 4},
		{0, 0, 1},
	}

	for _, tc := range cases {
		if got := PageCount(tc.total, tc.pageSize); got != tc.want {
			t.Errorf("PageCount(%d,%d) = %d, want %d", tc.total, tc.pageSize, got, tc.want)
		}
	}
}
