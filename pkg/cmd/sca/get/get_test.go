package get

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/sca/internal/scatestutil"
)

const (
	yesLabel = "yes"
	noLabel  = "no"
	dashPad  = "—"
)

func TestGet_RequiresExactlyOneArg(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing codebase")
	}
}

func TestGet_RejectsInvalidCodebase(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{"Not_A_DNS_Label"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "DNS-1123") {
		t.Errorf("expected DNS-1123 error, got %v", err)
	}
}

func TestGet_RejectsPRFlag(t *testing.T) {
	t.Parallel()
	// --pr is intentionally not registered on sca verbs. cobra must reject it.
	cmd := NewCmdGet(scatestutil.NewFactory(), nil)
	cmd.SetArgs([]string{"svc", "--pr", "42"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected unknown flag error, got %v", err)
	}
}

func TestGet_RunFInjectionWithBranch(t *testing.T) {
	t.Parallel()

	var captured *GetOptions
	cmd := NewCmdGet(scatestutil.NewFactory(), func(o *GetOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"svc", "--branch", "release/1.0"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if captured == nil || captured.Codebase != "svc" || captured.Branch != "release/1.0" {
		t.Errorf("captured = %+v", captured)
	}
}

func TestGet_BranchFlagUsageStringIsVerbatim(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(scatestutil.NewFactory(), nil)
	flag := cmd.Flag("branch")
	if flag == nil {
		t.Fatal("--branch flag missing")
	}
	if !strings.Contains(flag.Usage, "Dep-Track project 'version'") {
		t.Errorf("--branch usage must reference Dep-Track version field: %q", flag.Usage)
	}
	if !strings.Contains(flag.Usage, "krci sca list --search=<codebase>") {
		t.Errorf("--branch usage must hint discovery path: %q", flag.Usage)
	}
}

func TestPrintDetail_NONE(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := printDetail(&buf, "svc", "main", &portal.SCAProjectDetail{Status: portal.SCAStatusNone}, false)
	if err != nil {
		t.Fatalf("printDetail: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "status: NONE") {
		t.Errorf("expected NONE banner, got %q", out)
	}
	if !strings.Contains(out, "svc") || !strings.Contains(out, "main") {
		t.Errorf("output must include codebase and branch: %q", out)
	}
}

func TestPrintDetail_HappyPath(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	detail := &portal.SCAProjectDetail{
		Status: portal.SCAStatusOK,
		Project: &portal.SCAProject{
			UUID:       "u1",
			Name:       "svc",
			Version:    "main",
			Classifier: "APPLICATION",
			Active:     true,
			IsLatest:   true,
			RiskScore:  12.5,
		},
		Metrics: &portal.SCAMetrics{
			Critical:             3,
			High:                 4,
			Medium:               5,
			Low:                  6,
			Unassigned:           2,
			Vulnerabilities:      20,
			Components:           100,
			VulnerableComponents: 10,
		},
	}

	if err := printDetail(&buf, "svc", "", detail, false); err != nil {
		t.Fatalf("printDetail: %v", err)
	}
	out := buf.String()
	wants := []string{"svc @ main", "Codebase", "Branch", "Classifier", "Risk score",
		"Vulnerabilities", "Critical", "Components", "Total"}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output must contain %q, got %q", w, out)
		}
	}
}

func TestFormatters(t *testing.T) {
	t.Parallel()
	if formatActive(true, false) != yesLabel {
		t.Error("formatActive true")
	}
	if formatActive(false, true) != noLabel {
		t.Error("formatActive false")
	}
	if formatTimestamp(0, "") != dashPad {
		t.Error("formatTimestamp zero")
	}
	if formatRelativeTimestamp(0) != dashPad {
		t.Error("formatRelativeTimestamp zero")
	}
}

func TestSeverityRow(t *testing.T) {
	t.Parallel()

	// Plain (non-styled) renders just label + numeric value.
	p := severityRow("Critical", "CRITICAL", 3, false)
	if p.label != "Critical" || p.value != "3" || p.display != "" {
		t.Errorf("plain severityRow: %+v", p)
	}

	// Styled + count>0 attaches a coloured badge in display; the numeric value
	// must stay as the leading token so the JSON layer keeps using it.
	p = severityRow("Critical", "CRITICAL", 3, true)
	if !strings.HasPrefix(p.display, "3 ") {
		t.Errorf("styled display must start with count, got %q", p.display)
	}

	// Styled but count==0 must not attach a badge (0 counts stay neutral).
	p = severityRow("Critical", "CRITICAL", 0, true)
	if p.display != "" {
		t.Errorf("zero count must not render a badge, got %q", p.display)
	}

	// Empty canonical severity is a no-op on display.
	p = severityRow("Other", "", 5, true)
	if p.display != "" {
		t.Errorf("empty canonical must not render a badge, got %q", p.display)
	}
}
