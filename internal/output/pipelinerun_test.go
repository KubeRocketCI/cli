package output

import (
	"bytes"
	"regexp"
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

func TestRenderRunInfo_DeploymentAndEnv(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := RenderRunInfo(&buf, &portal.PipelineRunInfo{
		Name: "deploy-demo-dev-ab12", Status: portal.StatusFailed, Deployment: "demo", Env: "dev",
	}); err != nil {
		t.Fatalf("RenderRunInfo: %v", err)
	}

	out := buf.String()
	for _, want := range []string{`(?m)^Deployment:\s+demo$`, `(?m)^Env:\s+dev$`} {
		if !regexp.MustCompile(want).MatchString(out) {
			t.Errorf("no line matches %q in:\n%s", want, out)
		}
	}
}

func TestRenderRunInfo_AllFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := RenderRunInfo(&buf, &portal.PipelineRunInfo{
		Name:       "deploy-demo-dev-ab12",
		Status:     portal.StatusSucceeded,
		Duration:   "1m 2s",
		Pipeline:   "deploy",
		Project:    "demo-app",
		Deployment: "demo",
		Env:        "dev",
		Results:    map[string]any{"VCS_TAG": "1.0.0"},
	}); err != nil {
		t.Fatalf("RenderRunInfo: %v", err)
	}

	want := `Pipeline: deploy-demo-dev-ab12
Status:      Succeeded
Duration:    1m 2s
Pipeline:    deploy
Project:     demo-app
Deployment:  demo
Env:         dev
Results:     VCS_TAG=1.0.0
`
	if got := buf.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderNoTaskData(t *testing.T) {
	t.Parallel()

	notes := map[string]string{
		portal.TasksRunNotFinished: "Pipeline run has not finished yet. Task data is available after it finishes.\n",
		portal.TasksNotIndexed:     "Task data is not available. The run may not yet be indexed in Tekton Results.\n",
	}

	for reason, want := range notes {
		var buf bytes.Buffer
		if err := RenderNoTaskData(&buf, reason); err != nil {
			t.Fatalf("RenderNoTaskData(%q): %v", reason, err)
		}

		if buf.String() != want {
			t.Errorf("RenderNoTaskData(%q) = %q, want %q", reason, buf.String(), want)
		}
	}
}
