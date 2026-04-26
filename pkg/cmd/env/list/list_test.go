package list

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

var newFactory = cmdtest.NewFactory

func TestList_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(newFactory(), nil)
	cmd.SetArgs([]string{"-o", "yaml"})
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

func TestList_RejectsInvalidDeployment(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(newFactory(), nil)
	cmd.SetArgs([]string{"--deployment", "Invalid_Name"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid deployment")
	}

	if !strings.Contains(err.Error(), "DNS-1123") {
		t.Errorf("expected DNS-1123 message, got: %v", err)
	}
}

func TestList_RejectsInvalidCluster(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(newFactory(), nil)
	cmd.SetArgs([]string{"--cluster", "BAD"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid cluster")
	}

	if !strings.Contains(err.Error(), "DNS-1123") {
		t.Errorf("expected DNS-1123 message, got: %v", err)
	}
}

func TestList_AcceptsValidFlags(t *testing.T) {
	t.Parallel()

	called := false

	cmd := NewCmdList(newFactory(), func(opts *ListOptions) error {
		called = true

		if opts.Deployment != "my-pipeline" {
			t.Errorf("Deployment = %q, want my-pipeline", opts.Deployment)
		}

		if opts.Cluster != "in-cluster" {
			t.Errorf("Cluster = %q, want in-cluster", opts.Cluster)
		}

		if opts.OutputFormat != "json" {
			t.Errorf("OutputFormat = %q, want json", opts.OutputFormat)
		}

		return nil
	})

	cmd.SetArgs([]string{"--deployment", "my-pipeline", "--cluster", "in-cluster", "-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not invoked")
	}
}

func TestList_RejectsPositionalArgs(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(newFactory(), nil)
	cmd.SetArgs([]string{"frobnicate"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for stray positional arg")
	}
}
