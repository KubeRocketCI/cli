package components

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/sca/internal/scatestutil"
)

func TestComponents_RejectsInvalidSeverity(t *testing.T) {
	t.Parallel()

	cmd := NewCmdComponents(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{"svc", "--severity", "garbage"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid --severity") {
		t.Errorf("expected severity error, got %v", err)
	}
}

func TestComponents_RunFCapturesFlags(t *testing.T) {
	t.Parallel()

	var captured *ComponentsOptions
	cmd := NewCmdComponents(scatestutil.NewFactory(), func(o *ComponentsOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"svc", "--severity", "high", "--only-outdated", "--branch", "main"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if captured == nil {
		t.Fatal("runF not invoked")
	}
	if !captured.OnlyOutdated || captured.Branch != "main" || len(captured.Severity) != 1 {
		t.Errorf("flag capture: %+v", captured)
	}
}

func TestComponents_PageBounds(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"svc", "--page", "0"},
		{"svc", "--page-size", "0"},
		{"svc", "--page-size", "1000"},
	} {
		cmd := NewCmdComponents(scatestutil.NewFactory(), nil)
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err == nil {
			t.Errorf("args %v: expected error", args)
		}
	}
}

func TestComponents_RejectsPRFlag(t *testing.T) {
	t.Parallel()

	cmd := NewCmdComponents(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{"svc", "--pr", "42"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected unknown flag error, got %v", err)
	}
}

func TestApplySeverityFilter(t *testing.T) {
	t.Parallel()

	items := []portal.SCAComponent{
		{Name: "a", Metrics: &portal.SCAMetrics{Critical: 1}},
		{Name: "b", Metrics: &portal.SCAMetrics{Medium: 2}},
		{Name: "c", Metrics: &portal.SCAMetrics{Low: 1}},
		{Name: "d", Metrics: nil},
	}

	// No filter → passthrough (all four, including nil-metrics component).
	got := applySeverityFilter(items, nil)
	if len(got) != 4 {
		t.Errorf("nil filter must return all items, got %d", len(got))
	}

	// "HIGH" → only components with CRITICAL or HIGH metrics.
	got = applySeverityFilter(items, []string{"CRITICAL", "HIGH"})
	if len(got) != 1 || got[0].Name != "a" {
		t.Errorf("filter got %v; want ['a']", got)
	}

	// "MEDIUM" → CRITICAL + HIGH + MEDIUM-bearing rows.
	got = applySeverityFilter(items, []string{"CRITICAL", "HIGH", "MEDIUM"})
	if len(got) != 2 {
		t.Errorf("filter got %v; want 2 rows", got)
	}
}

func TestMetricsCountFor(t *testing.T) {
	t.Parallel()
	m := &portal.SCAMetrics{Critical: 1, High: 2, Medium: 3, Low: 4, Unassigned: 5}

	cases := map[string]int{
		"CRITICAL":   1,
		"HIGH":       2,
		"MEDIUM":     3,
		"LOW":        4,
		"INFO":       5,
		"UNASSIGNED": 5,
		"OTHER":      0,
	}
	for sev, want := range cases {
		if got := metricsCountFor(m, sev); got != want {
			t.Errorf("metricsCountFor(%s) = %d; want %d", sev, got, want)
		}
	}
}

func TestRenderTable_NONE(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := renderTable(&buf, false, "svc", 1, 50,
		&portal.SCAComponentList{Status: portal.SCAStatusNone}); err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	if !strings.Contains(buf.String(), "status: NONE") {
		t.Errorf("want NONE banner, got %q", buf.String())
	}
}
