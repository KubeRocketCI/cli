package portal

import (
	"os"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// TestEnvNamespaceReads_DocumentedInDocs pins docs/env.md and
// docs/json-schemas.md to the vocabulary and refusal messages `env pods` and
// `env events` emit, for the sample names each document uses.
func TestEnvNamespaceReads_DocumentedInDocs(t *testing.T) {
	t.Parallel()

	quoted := func(err error) string {
		return "`" + err.Error() + "`"
	}
	denied := func(rc restapi.K8sListJSONBody) string {
		return quoted(namespaceReadError(rc, "<namespace>", ErrPermissionDenied))
	}

	cases := map[string][]string{
		"../../docs/env.md": {
			remoteClusterError("my-pipeline", "prod", "prod-cluster").Error(),
			denied(podResourceConfig),
		},
		"../../docs/json-schemas.md": {
			quoted(remoteClusterError("<deployment>", "<env>", "<cluster>")),
			quoted(noNamespaceError("<deployment>", "<env>")),
			denied(podResourceConfig),
			denied(eventResourceConfig),
		},
	}

	for path, messages := range cases {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read docs: %v", err)
		}

		docs := string(raw)

		wants := append([]string{
			"`" + strings.Join([]string{ContainerWaiting, ContainerRunning, ContainerTerminated}, "`, `") + "`",
			"`" + PodProjectLabel + "`",
			"`" + EventTypeWarning + "`",
		}, messages...)

		for _, want := range wants {
			if !strings.Contains(docs, want) {
				t.Errorf("%s misses %s", path, want)
			}
		}
	}
}
