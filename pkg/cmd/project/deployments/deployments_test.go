package deployments

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

var newFactory = cmdtest.NewFactory

func TestDeployments_RequiresExactlyOnePositional(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{}, {"a", "b"}} {
		cmd := NewCmdDeployments(newFactory(), nil)
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.Execute(); err == nil {
			t.Errorf("expected error for args=%v", args)
		}
	}
}

func TestDeployments_RejectsInvalidProject(t *testing.T) {
	t.Parallel()

	cases := []string{"Bad_Name", "UPPER", strings.Repeat("a", 256)}

	for _, p := range cases {
		cmd := NewCmdDeployments(newFactory(), nil)
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

func TestDeployments_AcceptsValidArgs(t *testing.T) {
	t.Parallel()

	called := false

	cmd := NewCmdDeployments(newFactory(), func(opts *ListOptions) error {
		called = true

		if opts.Project != "my-app" {
			t.Errorf("Project = %q, want my-app", opts.Project)
		}

		return nil
	})

	cmd.SetArgs([]string{"my-app", "-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not invoked")
	}
}

func TestDeployments_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	cmd := NewCmdDeployments(newFactory(), nil)
	cmd.SetArgs([]string{"my-app", "-o", "yaml"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for -o yaml")
	}

	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("unexpected error: %v", err)
	}
}
