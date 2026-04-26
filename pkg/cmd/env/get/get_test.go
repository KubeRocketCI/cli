package get

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

var newFactory = cmdtest.NewFactory

func TestGet_RequiresExactlyTwoPositionals(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{},
		{"only-one"},
		{"a", "b", "c"},
	}

	for _, args := range cases {
		cmd := NewCmdGet(newFactory(), nil)
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.Execute(); err == nil {
			t.Errorf("expected error for args=%v", args)
		}
	}
}

func TestGet_RejectsInvalidDeployment(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{"BAD_NAME", "dev"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "DNS-1123") {
		t.Errorf("expected DNS-1123 message, got: %v", err)
	}
}

func TestGet_RejectsInvalidEnv(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{"my-pipeline", "Bad_Env"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "DNS-1123") {
		t.Errorf("expected DNS-1123 message, got: %v", err)
	}
}

func TestGet_AcceptsValidArgs(t *testing.T) {
	t.Parallel()

	called := false

	cmd := NewCmdGet(newFactory(), func(opts *GetOptions) error {
		called = true

		if opts.Deployment != "my-pipeline" {
			t.Errorf("Deployment = %q", opts.Deployment)
		}

		if opts.Env != "prod" {
			t.Errorf("Env = %q", opts.Env)
		}

		return nil
	})

	cmd.SetArgs([]string{"my-pipeline", "prod", "-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not invoked")
	}
}
