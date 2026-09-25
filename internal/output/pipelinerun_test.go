package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
)

func TestRenderRunInfo_Results(t *testing.T) {
	t.Parallel()

	run := &portal.PipelineRunInfo{
		Name:   "build-my-app-main-abc12",
		Status: portal.StatusSucceeded,
		Results: map[string]any{
			"VCS_TAG": "build/1.0.0-SNAPSHOT.3",
			"IMAGES":  []any{"app:1.0.0"},
		},
	}

	var buf bytes.Buffer
	if err := RenderRunInfo(&buf, run); err != nil {
		t.Fatalf("RenderRunInfo: %v", err)
	}

	out := buf.String()

	images := strings.Index(out, `IMAGES=["app:1.0.0"]`)
	tag := strings.Index(out, "VCS_TAG=build/1.0.0-SNAPSHOT.3")

	if !strings.Contains(out, "Results:") || images < 0 || tag < 0 {
		t.Fatalf("expected a Results block with every result, got:\n%s", out)
	}

	if images > tag {
		t.Errorf("results must be sorted by name, got:\n%s", out)
	}
}

func TestRenderRunInfo_NoResults(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := RenderRunInfo(&buf, &portal.PipelineRunInfo{Name: "review-x", Status: portal.StatusFailed}); err != nil {
		t.Fatalf("RenderRunInfo: %v", err)
	}

	if out := buf.String(); strings.Contains(out, "Results:") {
		t.Errorf("a run without results must not print the block, got:\n%s", out)
	}
}
