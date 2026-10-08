package envinternal

import (
	"os"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
)

// TestMapNotFound_DocumentedInJSONSchemas pins the error table of
// `krci env pods` in docs/json-schemas.md to the not-found messages.
func TestMapNotFound_DocumentedInJSONSchemas(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../docs/json-schemas.md")
	if err != nil {
		t.Fatalf("read docs: %v", err)
	}

	for _, sentinel := range []error{portal.ErrDeploymentNotFound, portal.ErrEnvNotFound} {
		if msg := MapNotFound(sentinel, "<deployment>", "<env>").Error(); !strings.Contains(string(raw), "`"+msg+"`") {
			t.Errorf("message not in docs/json-schemas.md:\n%s", msg)
		}
	}
}
