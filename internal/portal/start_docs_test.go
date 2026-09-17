package portal

import (
	"os"
	"strings"
	"testing"
)

// TestStartMessages_DocumentedInJSONSchemas pins the `pipelinerun start` error
// table in docs/json-schemas.md to the messages startReasons renders. The
// table names the pipeline as <name>.
func TestStartMessages_DocumentedInJSONSchemas(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../docs/json-schemas.md")
	if err != nil {
		t.Fatalf("read docs: %v", err)
	}

	docs := string(raw)

	for reason, r := range startReasons {
		if msg := r.err("<name>").Error(); !strings.Contains(docs, "`"+msg+"`") {
			t.Errorf("reason %s: message not in docs/json-schemas.md:\n%s", reason, msg)
		}
	}
}
