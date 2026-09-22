package get

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

var newFactory = cmdtest.NewFactory

func TestGet_RequiresExactlyTwoPositionals(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{},
		{"only-one"},
		{"a", "b", "c"},
	}

	for _, args := range cases {
		cmd := NewCmdGet(newFactory(), nil)
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		if err := cmd.Execute(); err == nil {
			t.Errorf("expected error for args=%v", args)
		}
	}
}

func TestGet_RejectsInvalidDeployment(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{"BAD_NAME", "dev"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "DNS-1123") {
		t.Errorf("expected DNS-1123 message, got: %v", err)
	}
}

func TestGet_RejectsInvalidEnv(t *testing.T) {
	t.Parallel()

	cmd := NewCmdGet(newFactory(), nil)
	cmd.SetArgs([]string{"my-pipeline", "Bad_Env"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "DNS-1123") {
		t.Errorf("expected DNS-1123 message, got: %v", err)
	}
}

func TestGet_AcceptsValidArgs(t *testing.T) {
	t.Parallel()

	called := false

	cmd := NewCmdGet(newFactory(), func(opts *GetOptions) error {
		called = true

		if opts.Deployment != "my-pipeline" {
			t.Errorf("Deployment = %q", opts.Deployment)
		}

		if opts.Env != "prod" {
			t.Errorf("Env = %q", opts.Env)
		}

		return nil
	})

	cmd.SetArgs([]string{"my-pipeline", "prod", "-o", "json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !called {
		t.Error("runF was not invoked")
	}
}

func TestRenderDetail_ConditionsBlock(t *testing.T) {
	t.Parallel()

	msg := "namespace creation failed"
	opMsg := "cluster unreachable"
	unknown := "unknown"
	healthy := "healthy"

	d := &portal.EnvDetail{
		Deployment:      "my-pipeline",
		Env:             "dev",
		Status:          "failed",
		DetailedMessage: &msg,
		Infrastructure:  portal.Infrastructure{Cluster: "remote", Namespace: "my-pipeline-dev", TriggerType: "Auto", DeployPipeline: "deploy"},
		QualityGates:    []portal.QualityGateDetail{},
		Projects: []portal.EnvProject{
			{Name: "bar", Status: &healthy, Sync: &healthy, IngressURLs: []string{}, Conditions: []portal.AppCondition{}},
			{
				Name: "foo", Status: &unknown, Sync: &unknown, IngressURLs: []string{},
				Conditions: []portal.AppCondition{{Type: "ComparisonError", Message: "Failed to load live state"}},
				Operation:  &portal.AppOperation{Phase: "Error", Message: &opMsg},
			},
		},
	}

	var buf bytes.Buffer
	if err := renderDetail(&buf, false, d); err != nil {
		t.Fatalf("renderDetail error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"Message:",
		"namespace creation failed",
		"Conditions (2):",
		"  - foo: ComparisonError: Failed to load live state",
		"  - foo: operation Error: cluster unreachable",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderDetail_NoConditionsBlock(t *testing.T) {
	t.Parallel()

	healthy := "healthy"
	d := &portal.EnvDetail{
		Deployment:     "my-pipeline",
		Env:            "dev",
		Status:         "created",
		Infrastructure: portal.Infrastructure{Cluster: "in-cluster", Namespace: "my-pipeline-dev", TriggerType: "Auto", DeployPipeline: "deploy"},
		QualityGates:   []portal.QualityGateDetail{},
		Projects: []portal.EnvProject{
			{Name: "bar", Status: &healthy, Sync: &healthy, IngressURLs: []string{}, Conditions: []portal.AppCondition{}},
		},
	}

	var buf bytes.Buffer
	if err := renderDetail(&buf, false, d); err != nil {
		t.Fatalf("renderDetail error: %v", err)
	}

	out := buf.String()
	for _, absent := range []string{"Conditions", "Message:"} {
		if strings.Contains(out, absent) {
			t.Errorf("output should not contain %q without diagnostics:\n%s", absent, out)
		}
	}
}
