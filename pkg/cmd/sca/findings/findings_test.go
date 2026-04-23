package findings

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/sca/internal/scatestutil"
)

func TestFindings_RunFInjection(t *testing.T) {
	t.Parallel()

	var captured *FindingsOptions
	cmd := NewCmdFindings(scatestutil.NewFactory(), func(o *FindingsOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"svc", "--severity", "critical", "--include-suppressed", "--source", "NVD", "--branch", "main"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if captured == nil {
		t.Fatal("runF not invoked")
	}
	if captured.Source != "NVD" || !captured.IncludeSuppressed || captured.Branch != "main" {
		t.Errorf("captured = %+v", captured)
	}
	if len(captured.Severity) != 1 || captured.Severity[0] != "critical" {
		t.Errorf("Severity = %v, want [critical]", captured.Severity)
	}
}

func TestFindings_RejectsInvalidSeverity(t *testing.T) {
	t.Parallel()

	cmd := NewCmdFindings(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{"svc", "--severity", "garbage"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid --severity") {
		t.Errorf("expected severity error, got %v", err)
	}
}

func TestFindings_RejectsEmptySource(t *testing.T) {
	t.Parallel()

	cmd := NewCmdFindings(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{"svc", "--source", ""})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--source requires a non-empty value") {
		t.Errorf("expected non-empty-source error, got %v", err)
	}
}

func TestFindings_RejectsPRFlag(t *testing.T) {
	t.Parallel()

	cmd := NewCmdFindings(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{"svc", "--pr", "42"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected unknown flag error, got %v", err)
	}
}

func TestFindings_RequiresCodebase(t *testing.T) {
	t.Parallel()

	cmd := NewCmdFindings(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestFindings_SeverityFlagUsageVerbatim(t *testing.T) {
	t.Parallel()

	cmd := NewCmdFindings(scatestutil.NewFactory(), nil)
	flag := cmd.Flag("severity")
	if flag == nil {
		t.Fatal("--severity missing")
	}
	if !strings.Contains(flag.Usage, "inclusive") || !strings.Contains(flag.Usage, "UNASSIGNED") {
		t.Errorf("--severity usage must document inclusive semantics + UNASSIGNED, got %q", flag.Usage)
	}
}

func TestRenderTable_NONE(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := renderTable(&buf, false, "svc",
		&portal.SCAFindingList{Status: portal.SCAStatusNone}); err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	if !strings.Contains(buf.String(), "status: NONE") {
		t.Errorf("want NONE banner, got %q", buf.String())
	}
}

func TestRenderTable_TruncationHint(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := renderTable(&buf, false, "svc", &portal.SCAFindingList{
		Status:    portal.SCAStatusOK,
		Items:     []portal.SCAFinding{},
		Truncated: true,
	})
	if err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	if !strings.Contains(buf.String(), "truncated to 1000 rows") {
		t.Errorf("expected truncation hint, got %q", buf.String())
	}
}

func TestRenderTable_NoHintWhenNotTruncated(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := renderTable(&buf, false, "svc", &portal.SCAFindingList{
		Status:    portal.SCAStatusOK,
		Items:     []portal.SCAFinding{},
		Truncated: false,
	})
	if err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	if strings.Contains(buf.String(), "truncated") {
		t.Errorf("truncation hint must be absent, got %q", buf.String())
	}
}

func TestFormatCVSS(t *testing.T) {
	t.Parallel()

	if got := formatCVSS(9.8, 0); !strings.Contains(got, "9.8") || !strings.Contains(got, "v3") {
		t.Errorf("v3 got %q", got)
	}
	if got := formatCVSS(0, 7.5); !strings.Contains(got, "7.5") || !strings.Contains(got, "v2") {
		t.Errorf("v2 got %q", got)
	}
	if got := formatCVSS(0, 0); got != "—" {
		t.Errorf("both zero got %q", got)
	}
}
