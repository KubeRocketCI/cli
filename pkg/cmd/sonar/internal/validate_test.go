package sonarinternal

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMaxPageSize(t *testing.T) {
	t.Parallel()

	if MaxPageSize != 500 {
		t.Errorf("MaxPageSize = %d, want 500", MaxPageSize)
	}
}

func TestValidateProjectKey(t *testing.T) {
	t.Parallel()

	valid := []string{
		"a",
		"payments-api",
		"krci-portal",
		"service-123",
	}
	for _, v := range valid {
		if err := ValidateProjectKey(v); err != nil {
			t.Errorf("ValidateProjectKey(%q) unexpected error: %v", v, err)
		}
	}

	invalid := []struct {
		in  string
		msg string
	}{
		{"", "empty"},
		{"UPPER_CASE_BAD", "DNS-1123"},
		{"-leading-dash", "DNS-1123"},
		{"trailing-dash-", "DNS-1123"},
		{"has_underscore", "DNS-1123"},
		{strings.Repeat("a", 300), "at most 253"},
	}
	for _, tc := range invalid {
		err := ValidateProjectKey(tc.in)
		if err == nil {
			t.Errorf("ValidateProjectKey(%q) expected error", tc.in)
			continue
		}
		if !strings.Contains(err.Error(), tc.msg) {
			t.Errorf("ValidateProjectKey(%q): want error containing %q, got %v", tc.in, tc.msg, err)
		}
	}
}

func TestValidateEnumCSV(t *testing.T) {
	t.Parallel()

	t.Run("empty returns nil", func(t *testing.T) {
		got, err := ValidateEnumCSV("severity", nil, IssueSeverities)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("want empty, got %v", got)
		}
	})

	t.Run("csv and repeated expanded", func(t *testing.T) {
		got, err := ValidateEnumCSV("severity", []string{"BLOCKER,CRITICAL", "MAJOR"}, IssueSeverities)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(got, []string{"BLOCKER", "CRITICAL", "MAJOR"}) {
			t.Errorf("want [BLOCKER CRITICAL MAJOR], got %v", got)
		}
	})

	t.Run("invalid enum rejected", func(t *testing.T) {
		_, err := ValidateEnumCSV("severity", []string{"CATASTROPHIC"}, IssueSeverities)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "invalid --severity=CATASTROPHIC") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("whitespace tolerated", func(t *testing.T) {
		got, err := ValidateEnumCSV("severity", []string{" BLOCKER , CRITICAL "}, IssueSeverities)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(got, []string{"BLOCKER", "CRITICAL"}) {
			t.Errorf("want [BLOCKER CRITICAL], got %v", got)
		}
	})

	t.Run("duplicate in single csv deduplicated", func(t *testing.T) {
		t.Parallel()

		got, err := ValidateEnumCSV("severity", []string{"BLOCKER,BLOCKER"}, IssueSeverities)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(got, []string{"BLOCKER"}) {
			t.Errorf("want [BLOCKER], got %v", got)
		}
	})

	t.Run("duplicate across repeated flags deduplicated preserving order", func(t *testing.T) {
		t.Parallel()

		got, err := ValidateEnumCSV("severity", []string{"BLOCKER,CRITICAL", "BLOCKER"}, IssueSeverities)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// BLOCKER appears first in the first element, CRITICAL second; the second
		// "BLOCKER" must be dropped.
		if !slices.Equal(got, []string{"BLOCKER", "CRITICAL"}) {
			t.Errorf("want [BLOCKER CRITICAL], got %v", got)
		}
	})
}

func TestValidateOutputFormat(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"", "table", "json"} {
		if err := ValidateOutputFormat(v); err != nil {
			t.Errorf("ValidateOutputFormat(%q): want nil, got %v", v, err)
		}
	}

	for _, v := range []string{"yaml", "csv", "JSON", "Table", "xml"} {
		err := ValidateOutputFormat(v)
		if err == nil {
			t.Errorf("ValidateOutputFormat(%q): want error", v)
		}
	}
}

func TestValidatePullRequest(t *testing.T) {
	t.Parallel()

	// Flag not set — empty value is fine (flag just omitted).
	if err := ValidatePullRequest(false, ""); err != nil {
		t.Errorf("unchanged empty: want nil, got %v", err)
	}

	// Flag set to an explicit empty string — rejected.
	err := ValidatePullRequest(true, "")
	if err == nil {
		t.Fatal("changed empty: expected error")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("want 'non-empty' in error, got %v", err)
	}

	// Flag set to a non-empty value — accepted.
	if err := ValidatePullRequest(true, "123"); err != nil {
		t.Errorf("changed non-empty: want nil, got %v", err)
	}
}

// newTestCmd builds a minimal cobra.Command with a --pull-request string flag
// so ValidateProjectCommand can call cmd.Flags().Changed("pull-request").
func newTestCmd(pullRequestChanged bool, pullRequestValue string) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("pull-request", "", "")

	if pullRequestChanged {
		if err := cmd.Flags().Set("pull-request", pullRequestValue); err != nil {
			panic("test setup: " + err.Error())
		}
	}

	return cmd
}

func TestValidateProjectCommand(t *testing.T) {
	t.Parallel()

	t.Run("invalid output format short-circuits before project validation", func(t *testing.T) {
		t.Parallel()

		cmd := newTestCmd(false, "")
		// Pass an empty project — if project validation ran it would also error,
		// but the output-format check must fire first.
		err := ValidateProjectCommand(cmd, "yaml", "", "")
		if err == nil {
			t.Fatal("expected error for invalid output format")
		}
		if !strings.Contains(err.Error(), "unknown output format") {
			t.Errorf("want 'unknown output format' error, got %v", err)
		}
	})

	t.Run("invalid project short-circuits before pull-request validation", func(t *testing.T) {
		t.Parallel()

		// --pull-request is changed and empty (would be rejected by ValidatePullRequest),
		// but project error must fire first.
		cmd := newTestCmd(true, "")
		err := ValidateProjectCommand(cmd, "", "", "")
		if err == nil {
			t.Fatal("expected error for empty project")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("want 'empty' in error, got %v", err)
		}
	})

	t.Run("valid inputs return nil", func(t *testing.T) {
		t.Parallel()

		cmd := newTestCmd(true, "42")
		if err := ValidateProjectCommand(cmd, "json", "my-project", "42"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid inputs without pull-request return nil", func(t *testing.T) {
		t.Parallel()

		cmd := newTestCmd(false, "")
		if err := ValidateProjectCommand(cmd, "", "my-project", ""); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
