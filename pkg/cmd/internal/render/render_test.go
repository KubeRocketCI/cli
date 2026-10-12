package render_test

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
)

func newBufferedIO() (*iostreams.IOStreams, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	return &iostreams.IOStreams{Out: stdout, ErrOut: stderr}, stdout
}

func TestRender_JSONEnvelope(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()

	err := render.Render(ios, "json", map[string]int{"bugs": 3}, func(_ io.Writer, _ bool) error {
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

	if got.SchemaVersion != render.SchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", got.SchemaVersion, render.SchemaVersion)
	}

	if got.Data["bugs"] != 3 {
		t.Errorf("data.bugs = %d, want 3", got.Data["bugs"])
	}
}

func TestRender_TableDefault(t *testing.T) {
	t.Parallel()

	ios, _ := newBufferedIO()
	called := false

	err := render.Render(ios, "", "unused", func(_ io.Writer, _ bool) error {
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

	err := render.Render(ios, "yaml", "unused", func(_ io.Writer, _ bool) error {
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

	err := render.Render(ios, "table", "payload", func(_ io.Writer, isTTY bool) error {
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

func TestRenderBare_JSONHasNoEnvelope(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()

	err := render.RenderBare(ios, "json", map[string]int{"bugs": 3}, func(_ io.Writer, _ bool) error {
		t.Fatal("table renderer must not run for -o json")
		return nil
	})
	if err != nil {
		t.Fatalf("RenderBare returned error: %v", err)
	}

	if want := "{\n  \"bugs\": 3\n}\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRenderBare_TableWritesToStdout(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"", "table"} {
		ios, stdout := newBufferedIO()

		err := render.RenderBare(ios, format, "unused", func(w io.Writer, isTTY bool) error {
			if isTTY {
				t.Error("isTTY should be false for a bytes.Buffer writer")
			}

			_, err := io.WriteString(w, "row\n")

			return err
		})
		if err != nil {
			t.Fatalf("RenderBare(-o %q) returned error: %v", format, err)
		}

		if stdout.String() != "row\n" {
			t.Errorf("RenderBare(-o %q) stdout = %q, want %q", format, stdout.String(), "row\n")
		}
	}
}

func TestRenderBare_UnknownFormat(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()

	err := render.RenderBare(ios, "xml", "unused", func(_ io.Writer, _ bool) error {
		t.Fatal("table renderer must not run for an unknown format")
		return nil
	})

	if want := "unknown output format: xml (use 'json' or 'table')"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}
