package portal

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestBuildMessages_DocumentedInProjectDocs pins the error table in
// docs/project.md to the messages checkBuildResponse produces for the
// sample request the table uses (project my-app, branch feat/x).
func TestBuildMessages_DocumentedInProjectDocs(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../docs/project.md")
	if err != nil {
		t.Fatalf("read docs: %v", err)
	}

	docs := string(raw)
	in := BuildInput{Codebase: buildCodebase, Branch: buildBranch, Params: map[string]string{"git-source-url": "x"}}
	args := buildReasonArgs(in)

	for reason, r := range buildReasons {
		if msg := r.err(args...).Error(); !strings.Contains(docs, "`"+msg+"`") {
			t.Errorf("reason %s: message not in docs/project.md:\n%s", reason, msg)
		}
	}

	routeMissing := checkBuildResponse(http.StatusNotFound, []byte(fastifyRouteMissing), in)
	if !strings.Contains(docs, "`"+routeMissing.Error()+"`") {
		t.Errorf("route-missing message not in docs/project.md:\n%s", routeMissing.Error())
	}
}

// TestBuildManagedParams_DocumentedInProjectDocs pins the "Managed params"
// section of docs/project.md to BuildManagedParams.
func TestBuildManagedParams_DocumentedInProjectDocs(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../docs/project.md")
	if err != nil {
		t.Fatalf("read docs: %v", err)
	}

	_, section, ok := strings.Cut(string(raw), "### Managed params")
	if !ok {
		t.Fatal("docs/project.md has no 'Managed params' section")
	}

	section, _, _ = strings.Cut(section, "\n### ")

	for _, p := range BuildManagedParams {
		if !strings.Contains(section, "`"+p+"`") {
			t.Errorf("managed param %s not listed under 'Managed params' in docs/project.md", p)
		}
	}
}
