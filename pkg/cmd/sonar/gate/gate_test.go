package gate

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

func newFactory() *cmdutil.Factory {
	return &cmdutil.Factory{
		IOStreams: &iostreams.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
		RestClient: func() (*restapi.ClientWithResponses, error) {
			return nil, nil
		},
	}
}

func TestGate_RequiresProject(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGate(newFactory(), nil)
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when no project arg is given")
	}
}

func TestGate_RejectsInvalidProject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		project string
	}{
		{name: "uppercase", project: "UPPER_CASE_BAD"},
		{name: "underscore", project: "bad_project"},
		{name: "space", project: "has space"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := NewCmdGate(newFactory(), nil)
			cmd.SetArgs([]string{tc.project})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error for invalid project key")
			}

			if !strings.Contains(err.Error(), "DNS-1123") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGate_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGate(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "-o", "yaml"})
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

func TestGate_RejectsEmptyPullRequest(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGate(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--pr", ""})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty --pr")
	}

	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGate_BranchReachesRunF(t *testing.T) {
	t.Parallel()

	called := false
	cmd := NewCmdGate(newFactory(), func(opts *GateOptions) error {
		called = true
		if opts.Branch != "main" {
			t.Errorf("Branch = %q, want %q", opts.Branch, "main")
		}
		if opts.PullRequest != "" {
			t.Errorf("PullRequest must stay empty when only --branch is set, got %q", opts.PullRequest)
		}
		return nil
	})

	cmd.SetArgs([]string{"my-proj", "--branch", "main"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !called {
		t.Error("runF was not called")
	}
}

func TestGate_RejectsEmptyBranch(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGate(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--branch", ""})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty --branch")
	}

	if !strings.Contains(err.Error(), "--branch requires a non-empty value") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGate_RejectsBothScopes(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGate(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--pr", "42", "--branch", "main"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when both scope flags are set")
	}

	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGate_ValidInvocationReachesRunF(t *testing.T) {
	t.Parallel()

	called := false
	cmd := NewCmdGate(newFactory(), func(opts *GateOptions) error {
		called = true

		if opts.Project != "my-proj" {
			t.Errorf("Project = %q, want %q", opts.Project, "my-proj")
		}

		if opts.PullRequest != "42" {
			t.Errorf("PullRequest = %q, want %q", opts.PullRequest, "42")
		}

		if opts.OutputFormat != "json" {
			t.Errorf("OutputFormat = %q, want %q", opts.OutputFormat, "json")
		}

		return nil
	})

	cmd.SetArgs([]string{"my-proj", "--pr", "42", "-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not called")
	}
}

func TestGate_ValidInvocationNoFlags(t *testing.T) {
	t.Parallel()

	called := false
	cmd := NewCmdGate(newFactory(), func(opts *GateOptions) error {
		called = true

		if opts.Project != "payments-api" {
			t.Errorf("Project = %q, want %q", opts.Project, "payments-api")
		}

		if opts.PullRequest != "" {
			t.Errorf("PullRequest = %q, want empty", opts.PullRequest)
		}

		if opts.OutputFormat != "" {
			t.Errorf("OutputFormat = %q, want empty", opts.OutputFormat)
		}

		return nil
	})

	cmd.SetArgs([]string{"payments-api"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not called")
	}
}

func TestGate_RenderTable_NoConditions(t *testing.T) {
	t.Parallel()

	g := &portal.SonarGate{
		ProjectStatus: portal.SonarGateProjectStatus{
			Status:     portal.QualityGateOK,
			Conditions: nil,
		},
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, "payments-api", g); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "OK") {
		t.Errorf("expected OK status in output, got: %s", out)
	}

	if !strings.Contains(out, "payments-api") {
		t.Errorf("expected project name in output, got: %s", out)
	}

	if !strings.Contains(out, "no conditions") {
		t.Errorf("expected no-conditions message, got: %s", out)
	}
}

func TestGate_RenderTable_WithConditions(t *testing.T) {
	t.Parallel()

	g := &portal.SonarGate{
		ProjectStatus: portal.SonarGateProjectStatus{
			Status: portal.QualityGateError,
			Conditions: []portal.SonarGateCondition{
				{
					MetricKey:      "coverage",
					Comparator:     "LT",
					ErrorThreshold: "80",
					ActualValue:    "74.5",
					Status:         portal.QualityGateError,
				},
				{
					MetricKey:      "duplicated_lines_density",
					Comparator:     "GT",
					ErrorThreshold: "3",
					ActualValue:    "1.2",
					Status:         portal.QualityGateOK,
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, "payments-api", g); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ERROR") {
		t.Errorf("expected ERROR status in output, got: %s", out)
	}

	if !strings.Contains(out, "coverage") {
		t.Errorf("expected metric key in output, got: %s", out)
	}
}

func TestGate_RenderTable_EmptyStatus(t *testing.T) {
	t.Parallel()

	g := &portal.SonarGate{
		ProjectStatus: portal.SonarGateProjectStatus{
			Status:     "",
			Conditions: nil,
		},
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, "my-proj", g); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, string(portal.QualityGateNone)) {
		t.Errorf("expected NONE status for empty gate, got: %s", out)
	}
}
