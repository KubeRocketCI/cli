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
	if captured.Severity[0] != "high" {
		t.Errorf("raw severity should be preserved on options; got %q", captured.Severity[0])
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

func TestRenderTable_TruncatedFooter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := renderTable(&buf, false, "svc", 1, 50, &portal.SCAComponentList{
		Status: portal.SCAStatusOK,
		Items: []portal.SCAComponent{
			{Name: "log4j", Version: "2.11.2"},
		},
		TotalCount: 1,
		Truncated:  true,
	})
	if err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	if !strings.Contains(buf.String(), "truncated") {
		t.Errorf("expected truncation footer, got %q", buf.String())
	}
}

func TestRenderTable_NoTruncatedFooterByDefault(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := renderTable(&buf, false, "svc", 1, 50, &portal.SCAComponentList{
		Status:     portal.SCAStatusOK,
		Items:      []portal.SCAComponent{{Name: "log4j", Version: "2.11.2"}},
		TotalCount: 1,
		Truncated:  false,
	})
	if err != nil {
		t.Fatalf("renderTable: %v", err)
	}
	if strings.Contains(buf.String(), "truncated") {
		t.Errorf("unexpected truncation footer when Truncated=false: %q", buf.String())
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
