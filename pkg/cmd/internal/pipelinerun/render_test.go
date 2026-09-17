package pipelinerun

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// portalManifest is what the Portal procedure returns for `dryRun=true`:
// the rendered PipelineRun draft as a JSON object (parsed at the transport
// boundary). The CLI re-encodes per requested output format.
func portalManifest() map[string]any {
	return map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata": map[string]any{
			"generateName": "foo-build-run-",
			"labels":       map[string]any{"app.edp.epam.com/codebase": "my-app"},
		},
		"spec": map[string]any{
			"params": []any{map[string]any{"name": "git-revision", "value": "main"}},
		},
	}
}

func TestRenderDryRun_OutputYAMLEmitsYAML(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}

	if err := renderDryRun(out, "yaml", portalManifest()); err != nil {
		t.Fatalf("RenderDryRun: %v", err)
	}

	if !strings.Contains(out.String(), "apiVersion: tekton.dev/v1") {
		t.Errorf("-o yaml output missing YAML markers:\n%s", out.String())
	}
}

func TestRenderDryRun_RejectsEmptyManifest(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}

	err := renderDryRun(out, "", nil)
	if err == nil {
		t.Fatal("expected error on empty manifest")
	}
}

// goldenStartResult is the fixed StartResult used by the golden-output tests
// below.
func goldenStartResult() *portal.StartResult {
	return &portal.StartResult{
		Name:     "foo-build-run-abcde",
		Status:   "Succeeded",
		Project:  "my-app",
		PR:       "42",
		Author:   "jdoe",
		Type:     "build",
		Started:  "2024-01-01T00:00:00Z",
		Duration: "1m2s",
	}
}

// TestRenderRow_GoldenTableOutput pins the non-TTY plain-table rendering of
// RenderRow against goldenStartResult; a byte-for-byte regression guard.
func TestRenderRow_GoldenTableOutput(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}
	io := &iostreams.IOStreams{Out: out, ErrOut: &bytes.Buffer{}}

	if err := renderRow(io, "", goldenStartResult()); err != nil {
		t.Fatalf("RenderRow: %v", err)
	}

	const want = "NAME                  STATUS      PROJECT   PR   AUTHOR   TYPE    STARTED                DURATION\n" +
		"foo-build-run-abcde   Succeeded   my-app    42   jdoe     build   2024-01-01T00:00:00Z   1m2s\n"

	if out.String() != want {
		t.Errorf("table output mismatch:\ngot:  %q\nwant: %q", out.String(), want)
	}
}

// TestRenderDryRun_GoldenYAMLOutput pins the default (YAML) RenderDryRun
// output; a byte-for-byte regression guard.
func TestRenderDryRun_GoldenYAMLOutput(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}

	if err := renderDryRun(out, "", portalManifest()); err != nil {
		t.Fatalf("RenderDryRun: %v", err)
	}

	const want = "apiVersion: tekton.dev/v1\n" +
		"kind: PipelineRun\n" +
		"metadata:\n" +
		"    generateName: foo-build-run-\n" +
		"    labels:\n" +
		"        app.edp.epam.com/codebase: my-app\n" +
		"spec:\n" +
		"    params:\n" +
		"        - name: git-revision\n" +
		"          value: main\n"

	if out.String() != want {
		t.Errorf("YAML output mismatch:\ngot:  %q\nwant: %q", out.String(), want)
	}
}

// TestRenderDryRun_GoldenJSONEnvelope pins the -o json RenderDryRun output;
// a byte-for-byte regression guard.
func TestRenderDryRun_GoldenJSONEnvelope(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}

	if err := renderDryRun(out, "json", portalManifest()); err != nil {
		t.Fatalf("RenderDryRun: %v", err)
	}

	const want = `{
  "schemaVersion": "1",
  "data": {
    "apiVersion": "tekton.dev/v1",
    "kind": "PipelineRun",
    "metadata": {
      "generateName": "foo-build-run-",
      "labels": {
        "app.edp.epam.com/codebase": "my-app"
      }
    },
    "spec": {
      "params": [
        {
          "name": "git-revision",
          "value": "main"
        }
      ]
    }
  }
}
`

	if out.String() != want {
		t.Errorf("JSON envelope mismatch:\ngot:  %q\nwant: %q", out.String(), want)
	}
}
