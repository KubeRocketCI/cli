package list

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/pkg/cmd/sca/internal/scatestutil"
)

func TestList_RejectsPositionalArgs(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{"frobnicate"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for stray positional arg")
	}
}

func TestList_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	cmd := NewCmdList(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{"-o", "yaml"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList_PageBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--page", "0"}, "--page must be >= 1"},
		{[]string{"--page-size", "0"}, "--page-size must be between 1 and 500"},
		{[]string{"--page-size", "1000"}, "--page-size must be between 1 and 500"},
	}
	for _, tc := range cases {
		cmd := NewCmdList(scatestutil.NewFactory(), nil)
		cmd.SetArgs(tc.args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v: got %v, want substr %q", tc.args, err, tc.want)
		}
	}
}

func TestList_RunFInjection(t *testing.T) {
	t.Parallel()

	var captured *ListOptions
	runF := func(o *ListOptions) error {
		captured = o
		return nil
	}

	cmd := NewCmdList(scatestutil.NewFactory(), runF)
	cmd.SetArgs([]string{"--page", "3", "--page-size", "50", "--search", "pay",
		"--include-inactive", "--include-children", "-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if captured == nil {
		t.Fatal("runF not invoked")
	}
	if captured.Page != 3 || captured.PageSize != 50 || captured.Search != "pay" ||
		!captured.IncludeInactive || !captured.IncludeChildren || captured.OutputFormat != "json" {
		t.Errorf("captured: %+v", captured)
	}
}

func TestFormatLastBom_ZeroIsDash(t *testing.T) {
	t.Parallel()

	if got := formatLastBom(0); got != "—" {
		t.Errorf("formatLastBom(0) = %q; want dash", got)
	}
}
