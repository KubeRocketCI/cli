package issues

import (
	"bytes"
	"slices"
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
			// Not exercised because tests stop at validation.
			return nil, nil
		},
	}
}

func TestIssues_InvalidProjectKey(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
	cmd.SetArgs([]string{"UPPER_CASE_BAD"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "DNS-1123") {
		t.Errorf("expected DNS-1123 error, got %v", err)
	}
}

func TestIssues_InvalidSeverity(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--severity", "CATASTROPHIC"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid --severity=CATASTROPHIC") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIssues_RejectsPageZero(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--page", "0"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --page 0")
	}
	if !strings.Contains(err.Error(), "--page must be >= 1") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIssues_RejectsPageSizeZero(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--page-size", "0"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --page-size 0")
	}
	if !strings.Contains(err.Error(), "--page-size must be between 1 and 500") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIssues_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
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

func TestIssues_PageSizeOutOfRange(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--page-size", "1000"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--page-size") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIssues_ValidInvocation_NormalizedReachesRunF(t *testing.T) {
	t.Parallel()

	called := false
	cmd := NewCmdIssues(newFactory(), func(opts *IssuesOptions) error {
		called = true
		if opts.Project != "my-proj" {
			t.Errorf("project = %q", opts.Project)
		}

		// Raw slices still visible for inspection…
		if len(opts.Severities) != 2 || opts.Severities[0] != "BLOCKER" || opts.Severities[1] != "CRITICAL" {
			t.Errorf("raw severities = %v, want [BLOCKER CRITICAL]", opts.Severities)
		}

		// …and the normalized portal payload carries the same slice.
		p := opts.Normalized()
		if !slices.Equal(p.Severities, []string{"BLOCKER", "CRITICAL"}) {
			t.Errorf("normalized severities = %v, want [BLOCKER CRITICAL]", p.Severities)
		}

		return nil
	})
	cmd.SetArgs([]string{"my-proj", "--severity", "BLOCKER,CRITICAL"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !called {
		t.Error("runF not called")
	}
}

func TestIssues_ResolvedOnlyForwardedWhenChanged(t *testing.T) {
	t.Parallel()

	t.Run("not passed → omitted from payload", func(t *testing.T) {
		cmd := NewCmdIssues(newFactory(), func(opts *IssuesOptions) error {
			if opts.Normalized().Resolved != nil {
				t.Errorf("resolved should be nil when flag not set; got %v", *opts.Normalized().Resolved)
			}
			return nil
		})
		cmd.SetArgs([]string{"my-proj"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute error: %v", err)
		}
	})

	t.Run("--resolved → resolved=true", func(t *testing.T) {
		cmd := NewCmdIssues(newFactory(), func(opts *IssuesOptions) error {
			got := opts.Normalized().Resolved
			if got == nil || *got != true {
				t.Errorf("resolved = %v, want *true", got)
			}
			return nil
		})
		cmd.SetArgs([]string{"my-proj", "--resolved"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute error: %v", err)
		}
	})

	t.Run("--resolved=false → resolved=false (explicit)", func(t *testing.T) {
		cmd := NewCmdIssues(newFactory(), func(opts *IssuesOptions) error {
			got := opts.Normalized().Resolved
			if got == nil || *got != false {
				t.Errorf("resolved = %v, want *false", got)
			}
			return nil
		})
		cmd.SetArgs([]string{"my-proj", "--resolved=false"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute error: %v", err)
		}
	})
}

func TestIssues_InvalidType(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--type", "CATASTROPHIC"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid type")
	}

	if !strings.Contains(err.Error(), "invalid --type=CATASTROPHIC") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIssues_InvalidStatus(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
	cmd.SetArgs([]string{"my-proj", "--status", "UNKNOWN"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid status")
	}

	if !strings.Contains(err.Error(), "invalid --status=UNKNOWN") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIssues_RenderTable_EmptyPullRequest(t *testing.T) {
	t.Parallel()

	result := &portal.SonarIssueList{
		Paging: portal.SonarPaging{PageIndex: 1, PageSize: 25, Total: 0},
		Issues: nil,
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, "my-proj", "pr-123", "", result); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "verify the pull-request id") {
		t.Errorf("expected hint about pull-request id in output, got:\n%s", out)
	}

	if !strings.Contains(out, "pr-123") {
		t.Errorf("expected pull-request id in hint, got:\n%s", out)
	}
}

func TestIssues_RenderTable_EmptyBranch(t *testing.T) {
	t.Parallel()

	result := &portal.SonarIssueList{
		Paging: portal.SonarPaging{PageIndex: 1, PageSize: 25, Total: 0},
		Issues: nil,
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, "my-proj", "", "feat/x", result); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "verify the branch name") {
		t.Errorf("expected hint about branch name in output, got:\n%s", out)
	}

	if !strings.Contains(out, "feat/x") {
		t.Errorf("expected branch name in hint, got:\n%s", out)
	}
}

func TestIssues_RenderTable_EmptyNoPullRequest(t *testing.T) {
	t.Parallel()

	result := &portal.SonarIssueList{
		Paging: portal.SonarPaging{PageIndex: 1, PageSize: 25, Total: 0},
		Issues: nil,
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, "my-proj", "", "", result); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()
	// No pull-request / branch hint when no scope was specified
	if strings.Contains(out, "verify the pull-request id") {
		t.Errorf("should not show pull-request hint when no PR given, got:\n%s", out)
	}

	if strings.Contains(out, "verify the branch name") {
		t.Errorf("should not show branch hint when no branch given, got:\n%s", out)
	}

	if !strings.Contains(out, "0 total") {
		t.Errorf("expected total count in output, got:\n%s", out)
	}
}

func TestIssues_RenderTable_WithIssues(t *testing.T) {
	t.Parallel()

	result := &portal.SonarIssueList{
		Paging: portal.SonarPaging{PageIndex: 1, PageSize: 25, Total: 2},
		Issues: []portal.SonarIssue{
			{
				Key:       "AXz123",
				Severity:  "CRITICAL",
				Type:      "BUG",
				Status:    "OPEN",
				Rule:      "java:S1234",
				Component: "src/main/Foo.java",
				Line:      42,
				Message:   "This is a short message",
			},
			{
				Key:       "AXz456",
				Severity:  "MAJOR",
				Type:      "CODE_SMELL",
				Status:    "CONFIRMED",
				Rule:      "java:S5678",
				Component: "src/main/Bar.java",
				Line:      0, // no line
				Message:   "Another issue",
			},
		},
	}

	var buf bytes.Buffer
	if err := renderTable(&buf, false, "my-proj", "", "", result); err != nil {
		t.Fatalf("renderTable error: %v", err)
	}

	out := buf.String()

	wantSubstrings := []string{
		"AXz123", "CRITICAL", "BUG", "OPEN",
		"AXz456", "MAJOR", "CODE_SMELL", "CONFIRMED",
		"42",
	}

	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, not found in:\n%s", want, out)
		}
	}
}

func TestIssues_RenderTable_MessageTruncationInTTY(t *testing.T) {
	t.Parallel()

	longMsg := strings.Repeat("x", maxMessageLength+20)

	result := &portal.SonarIssueList{
		Paging: portal.SonarPaging{PageIndex: 1, PageSize: 25, Total: 1},
		Issues: []portal.SonarIssue{
			{
				Key:       "AXz999",
				Severity:  "MINOR",
				Type:      "CODE_SMELL",
				Status:    "OPEN",
				Rule:      "java:S9999",
				Component: "src/main/Long.java",
				Message:   longMsg,
			},
		},
	}

	// TTY mode — message should be truncated
	var ttyBuf bytes.Buffer
	if err := renderTable(&ttyBuf, true, "my-proj", "", "", result); err != nil {
		t.Fatalf("renderTable TTY error: %v", err)
	}

	ttyOut := ttyBuf.String()

	// Non-TTY mode — full message preserved
	var plainBuf bytes.Buffer
	if err := renderTable(&plainBuf, false, "my-proj", "", "", result); err != nil {
		t.Fatalf("renderTable plain error: %v", err)
	}

	plainOut := plainBuf.String()

	// Plain output must contain the full long message
	if !strings.Contains(plainOut, longMsg) {
		t.Errorf("non-TTY output should preserve full message, got:\n%s", plainOut)
	}

	// TTY output must not contain the full long message
	if strings.Contains(ttyOut, longMsg) {
		t.Errorf("TTY output should truncate long message, but it was not truncated")
	}
}

func TestIssues_AscForwardedWhenChanged(t *testing.T) {
	t.Parallel()

	t.Run("not passed → omitted from payload", func(t *testing.T) {
		cmd := NewCmdIssues(newFactory(), func(opts *IssuesOptions) error {
			if opts.Normalized().Asc != nil {
				t.Errorf("asc should be nil when flag not set; got %v", *opts.Normalized().Asc)
			}
			return nil
		})
		cmd.SetArgs([]string{"my-proj"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute error: %v", err)
		}
	})

	t.Run("--asc → asc=true", func(t *testing.T) {
		cmd := NewCmdIssues(newFactory(), func(opts *IssuesOptions) error {
			got := opts.Normalized().Asc
			if got == nil || !*got {
				t.Errorf("asc = %v, want *true", got)
			}
			return nil
		})
		cmd.SetArgs([]string{"my-proj", "--asc"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute error: %v", err)
		}
	})
}

func TestIssues_ValidInvocationAllFields(t *testing.T) {
	t.Parallel()

	called := false
	cmd := NewCmdIssues(newFactory(), func(opts *IssuesOptions) error {
		called = true

		p := opts.Normalized()

		if p.ProjectKey != "my-proj" {
			t.Errorf("ProjectKey = %q, want %q", p.ProjectKey, "my-proj")
		}

		if p.PullRequest != "55" {
			t.Errorf("PullRequest = %q, want %q", p.PullRequest, "55")
		}

		if !slices.Equal(p.Types, []string{"BUG"}) {
			t.Errorf("Types = %v, want [BUG]", p.Types)
		}

		if !slices.Equal(p.Statuses, []string{"OPEN", "CONFIRMED"}) {
			t.Errorf("Statuses = %v, want [OPEN CONFIRMED]", p.Statuses)
		}

		return nil
	})

	cmd.SetArgs([]string{
		"my-proj",
		"--pr", "55",
		"--type", "BUG",
		"--status", "OPEN,CONFIRMED",
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not called")
	}
}

func TestIssues_BranchFlagForwarded(t *testing.T) {
	t.Parallel()

	called := false
	cmd := NewCmdIssues(newFactory(), func(opts *IssuesOptions) error {
		called = true
		p := opts.Normalized()
		if p.Branch != "main" {
			t.Errorf("Branch = %q, want %q", p.Branch, "main")
		}
		if p.PullRequest != "" {
			t.Errorf("PullRequest must stay empty when only --branch is set, got %q", p.PullRequest)
		}
		return nil
	})

	cmd.SetArgs([]string{"my-proj", "--branch", "main", "--severity", "BLOCKER"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !called {
		t.Error("runF was not called")
	}
}

func TestIssues_RejectsEmptyBranch(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
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

func TestIssues_RejectsBothScopes(t *testing.T) {
	t.Parallel()

	cmd := NewCmdIssues(newFactory(), nil)
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
