package versions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

var newFactory = cmdtest.NewFactory

func TestVersions_RequiresExactlyOnePositional(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{}, {"a", "b"}} {
		cmd := NewCmdVersions(newFactory(), nil)
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.Execute(); err == nil {
			t.Errorf("expected error for args=%v", args)
		}
	}
}

func TestVersions_RejectsInvalidProject(t *testing.T) {
	t.Parallel()

	for _, p := range []string{"Bad_Name", "UPPER", strings.Repeat("a", 256)} {
		cmd := NewCmdVersions(newFactory(), nil)
		cmd.SetArgs([]string{p})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.Execute()
		if err == nil {
			t.Errorf("expected error for project=%q", p)
			continue
		}

		if !strings.Contains(err.Error(), "DNS-1123") {
			t.Errorf("project=%q: expected DNS-1123 message, got %v", p, err)
		}
	}
}

func TestVersions_AcceptsBranchWithSlash(t *testing.T) {
	t.Parallel()

	called := false

	cmd := NewCmdVersions(newFactory(), func(opts *ListOptions) error {
		called = true

		if opts.Project != "my-app" {
			t.Errorf("Project = %q, want my-app", opts.Project)
		}

		if opts.Branch != "release/2.27" {
			t.Errorf("Branch = %q, want release/2.27", opts.Branch)
		}

		return nil
	})

	cmd.SetArgs([]string{"my-app", "--branch", "release/2.27", "-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not invoked")
	}
}

func TestVersions_RejectsTooLongBranch(t *testing.T) {
	t.Parallel()

	cmd := NewCmdVersions(newFactory(), nil)
	cmd.SetArgs([]string{"my-app", "--branch", strings.Repeat("b", portal.MaxBranchLength+1)})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for an over-long branch")
	}

	if !strings.Contains(err.Error(), "--branch must be at most") {
		t.Errorf("error = %v, want the --branch bound", err)
	}
}

func TestVersions_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	cmd := NewCmdVersions(newFactory(), nil)
	cmd.SetArgs([]string{"my-app", "-o", "yaml"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for -o yaml")
	}

	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("error = %v, want 'unknown output format'", err)
	}
}

func sampleStreams() []portal.ProjectVersionStream {
	return []portal.ProjectVersionStream{
		{Branch: "feature-x", Image: "registry.example.com/ns/my-app", Versions: []portal.ImageVersion{}},
		{
			Branch: "main", Image: "registry.example.com/ns/my-app",
			Versions: []portal.ImageVersion{
				{Name: "0.1.0-SNAPSHOT.2", Created: "2026-09-02T09:00:00Z", Digest: "sha256:cccc3333dddd4444"},
				{Name: "0.1.0-SNAPSHOT.1", Created: "2026-08-30T09:00:00Z"},
			},
		},
	}
}

func TestRenderStreamsTable(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := renderStreamsTable(&buf, false, sampleStreams()); err != nil {
		t.Fatalf("renderStreamsTable error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"BRANCH", "VERSIONS", "LATEST", "CREATED", "IMAGE", "main", "0.1.0-SNAPSHOT.2", "2026-09-02T09:00:00Z", "feature-x"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("want header + 2 rows, got %d lines:\n%s", len(lines), out)
	}

	if !strings.Contains(lines[1], "feature-x") || !strings.Contains(lines[1], "0") || !strings.Contains(lines[1], "-") {
		t.Errorf("a stream without versions shows 0 and dashes: %q", lines[1])
	}

	if strings.Contains(out, "0.1.0-SNAPSHOT.1") {
		t.Errorf("the per-branch view lists only the newest version:\n%s", out)
	}
}

func TestEmptyNote(t *testing.T) {
	t.Parallel()

	streams := sampleStreams()

	cases := []struct {
		name    string
		branch  string
		streams []portal.ProjectVersionStream
		want    string
	}{
		{name: "streams with versions", branch: "", streams: streams, want: ""},
		{name: "no streams", branch: "", streams: nil, want: "No versions found for project my-app."},
		{name: "branch with versions", branch: "main", streams: streams[1:], want: ""},
		{name: "branch never built", branch: "feature-x", streams: streams[:1], want: "No versions found for branch feature-x of project my-app."},
		{name: "unknown branch", branch: "ghost", streams: nil, want: "No versions found for branch ghost of project my-app."},
	}

	for _, tc := range cases {
		if got := emptyNote("my-app", tc.branch, tc.streams); got != tc.want {
			t.Errorf("%s: emptyNote = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRenderVersionsTable(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := renderVersionsTable(&buf, false, sampleStreams()[1:]); err != nil {
		t.Fatalf("renderVersionsTable error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"VERSION", "CREATED", "DIGEST", "IMAGE", "0.1.0-SNAPSHOT.2", "0.1.0-SNAPSHOT.1", "sha256:cccc333"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	if strings.Contains(out, "sha256:cccc3333dddd4444") {
		t.Errorf("the table shortens digests:\n%s", out)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "0.1.0-SNAPSHOT.2") {
		t.Errorf("want header + newest-first rows, got:\n%s", out)
	}
}
