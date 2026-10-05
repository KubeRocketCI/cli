package portal

import (
	"os"
	"strings"
	"testing"
)

// TestPipelineRunFilters_DocumentedInPipelinerunDocs pins docs/pipelinerun.md
// to the status keywords and to the tasksUnavailable reasons.
func TestPipelineRunFilters_DocumentedInPipelinerunDocs(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../docs/pipelinerun.md")
	if err != nil {
		t.Fatalf("read docs: %v", err)
	}

	docs := string(raw)

	if row := "`" + strings.Join(PipelineRunStatusKeywords(), "`, `") + "`"; !strings.Contains(docs, row) {
		t.Errorf("status keywords not in docs/pipelinerun.md: %s", row)
	}

	for _, reason := range []string{TasksRunNotFinished, TasksNotIndexed} {
		if !strings.Contains(docs, "| `"+reason+"`") {
			t.Errorf("tasksUnavailable reason %q not in docs/pipelinerun.md", reason)
		}
	}
}
