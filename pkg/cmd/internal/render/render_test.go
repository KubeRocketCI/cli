package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
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

	ios, stdout := newBufferedIO()

	err := Render(ios, "yaml", "unused", func(_ io.Writer, _ bool) error {
		t.Fatal("table renderer must not run for an unknown format")
		return nil
	})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}

	if want := "unknown output format: yaml (use 'json' or 'table')"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRender_TableCallbackReceivesNonTTY(t *testing.T) {
	t.Parallel()

	// Out is a *bytes.Buffer, not an *os.File, so IsStdoutTTY() is false.
	ios, _ := newBufferedIO()

	var (
		capturedTTY    bool
		rendererCalled bool
	)

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

func TestHandleError_JSONEnvelopePrinted(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()
	boom := errors.New("boom")

	if got := HandleError(ios, "json", boom); got != boom {
		t.Errorf("HandleError must return the original error; got %v", got)
	}

	want := output.JSONErrorEnvelope{SchemaVersion: SchemaVersion, Error: output.JSONErrorBody{Message: "boom"}}
	if got := cmdtest.DecodeErrorEnvelope(t, stdout); got != want {
		t.Errorf("envelope = %+v, want %+v", got, want)
	}
}

func TestHandleError_TableWritesNothing(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()
	boom := errors.New("boom")

	if got := HandleError(ios, "table", boom); got != boom {
		t.Errorf("HandleError must return the original error; got %v", got)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
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
		{101, 25, 5},
		{0, 0, 1},
		{100, 0, 1},
		{100, -5, 1},
	}

	for _, tc := range cases {
		if got := PageCount(tc.total, tc.pageSize); got != tc.want {
			t.Errorf("PageCount(%d,%d) = %d, want %d", tc.total, tc.pageSize, got, tc.want)
		}
	}
}

func TestOrDash(t *testing.T) {
	t.Parallel()

	if got := OrDash(""); got != "-" {
		t.Errorf("OrDash(\"\") = %q, want %q", got, "-")
	}

	if got := OrDash("x"); got != "x" {
		t.Errorf("OrDash(\"x\") = %q, want %q", got, "x")
	}
}
