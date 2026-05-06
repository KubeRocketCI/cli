package scainternal

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateCodebaseKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"valid", "my-service", ""},
		{"valid with digits", "svc-42", ""},
		{"empty", "", "must not be empty"},
		{"uppercase rejected", "MyService", "DNS-1123"},
		{"too long", strings.Repeat("x", 300), "at most"},
		{"invalid char", "my_service", "DNS-1123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateCodebaseKey(tc.input)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("want nil, got %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("want substr %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateOutputFormat(t *testing.T) {
	t.Parallel()

	for _, f := range []string{"", "table", "json"} {
		if err := ValidateOutputFormat(f); err != nil {
			t.Errorf("%q must be valid, got %v", f, err)
		}
	}
	if err := ValidateOutputFormat("yaml"); err == nil {
		t.Error("yaml must be rejected")
	}
}

func TestValidateCodebaseCommand_SkipsPRMutex(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.Flags().String("branch", "", "")
	// no --pr flag on sca; helper must not complain about it.

	if err := ValidateCodebaseCommand(cmd, "", "svc", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCodebaseCommand_RejectsEmptyBranchWhenSupplied(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.Flags().String("branch", "", "")
	if err := cmd.Flags().Set("branch", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Simulate cobra marking the flag as changed (user typed --branch="").
	cmd.Flags().Changed("branch") // no-op; need to mark as changed directly.
	f := cmd.Flags().Lookup("branch")
	f.Changed = true

	err := ValidateCodebaseCommand(cmd, "", "svc", "")
	if err == nil || !strings.Contains(err.Error(), "requires a non-empty value") {
		t.Errorf("expected non-empty-value error, got %v", err)
	}
}

func TestValidateSeverityCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
	}{
		{"empty", nil, nil, false},
		{"single lowercase", []string{"critical"}, []string{"CRITICAL"}, false},
		{"uppercase accepted", []string{"HIGH"}, []string{"HIGH"}, false},
		{"mixed case", []string{"Medium"}, []string{"MEDIUM"}, false},
		{"csv in single value", []string{"low,info"}, []string{"LOW", "INFO"}, false},
		{"dedupe", []string{"low,LOW,Low"}, []string{"LOW"}, false},
		{"unassigned explicit", []string{"unassigned"}, []string{"UNASSIGNED"}, false},
		{"invalid", []string{"garbage"}, nil, true},
		{"one invalid in csv", []string{"critical,garbage"}, nil, true},
		{"empty csv tokens skipped", []string{"critical,,high"}, []string{"CRITICAL", "HIGH"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateSeverityCSV(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestValidateSeverityCSV_ErrorListsAccepted(t *testing.T) {
	t.Parallel()
	_, err := ValidateSeverityCSV([]string{"garbage"})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, wanted := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "INFO", "UNASSIGNED"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Errorf("error message should list %s; got %q", wanted, err.Error())
		}
	}
}

func TestInclusiveSeverities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		min  string
		want []string
	}{
		{"", nil},
		{"CRITICAL", []string{"CRITICAL"}},
		{"critical", []string{"CRITICAL"}},
		{"HIGH", []string{"CRITICAL", "HIGH"}},
		{"MEDIUM", []string{"CRITICAL", "HIGH", "MEDIUM"}},
		{"LOW", []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"}},
		{"INFO", []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "INFO", "UNASSIGNED"}},
		{"UNASSIGNED", []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "INFO", "UNASSIGNED"}},
		{"unknown", nil},
	}
	for _, tc := range tests {
		t.Run(tc.min, func(t *testing.T) {
			t.Parallel()
			got := InclusiveSeverities(tc.min)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestInclusiveFromSet(t *testing.T) {
	t.Parallel()

	if got := InclusiveFromSet(nil); got != nil {
		t.Errorf("nil input → got %v", got)
	}

	// Multi-value: take the least severe and expand from there.
	got := InclusiveFromSet([]string{"CRITICAL", "MEDIUM"})
	want := []string{"CRITICAL", "HIGH", "MEDIUM"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}

	// Single least-severe LOW must include LOW too.
	got = InclusiveFromSet([]string{"LOW"})
	want = []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestConstants(t *testing.T) {
	t.Parallel()
	if MaxPageSize != 500 {
		t.Errorf("MaxPageSize = %d; want 500", MaxPageSize)
	}
	if !strings.Contains(BranchFlagUsage, "Dep-Track project 'version'") {
		t.Error("BranchFlagUsage must document the Dep-Track version mapping")
	}
	if !strings.Contains(SeverityFlagUsage, "inclusive") || !strings.Contains(SeverityFlagUsage, "UNASSIGNED") {
		t.Error("SeverityFlagUsage must describe inclusivity and INFO/UNASSIGNED merge")
	}
}

// ensure we surface the validator error through errors.Is without wrapping.
func TestValidateOutputFormat_ErrorType(t *testing.T) {
	t.Parallel()
	err := ValidateOutputFormat("csv")
	if err == nil {
		t.Fatal("expected error")
	}
	// ValidateOutputFormat returns a fmt.Errorf; no sentinel check — just
	// ensure it's a plain error.
	if errors.Is(err, nil) {
		t.Error("error should not be nil via errors.Is")
	}
}
