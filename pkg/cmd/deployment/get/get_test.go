package get

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
)

func TestPrintPlainDeploymentDetail_Messages(t *testing.T) {
	t.Parallel()

	d := &portal.DeploymentDetail{
		Name:            "my-pipeline",
		Namespace:       "ns",
		Applications:    []string{"foo"},
		Status:          "failed",
		DetailedMessage: "gitops repository unreachable",
		Stages: []portal.Stage{
			{Name: "dev", Order: 0, TriggerType: "Auto", Namespace: "ns-dev", Status: "failed", DetailedMessage: "quota exceeded"},
			{Name: "qa", Order: 1, TriggerType: "Manual", Namespace: "ns-qa", Status: "created"},
		},
	}

	var buf bytes.Buffer
	if err := printPlainDeploymentDetail(&buf, d); err != nil {
		t.Fatalf("printPlainDeploymentDetail error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"Message:", "gitops repository unreachable", "Messages:", "  dev: quota exceeded"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	if strings.Contains(out, "  qa:") {
		t.Errorf("stages without a message must not be listed:\n%s", out)
	}
}

func TestPrintPlainDeploymentDetail_NoMessages(t *testing.T) {
	t.Parallel()

	d := &portal.DeploymentDetail{
		Name:         "my-pipeline",
		Namespace:    "ns",
		Applications: []string{"foo"},
		Status:       "created",
		Available:    true,
		Stages: []portal.Stage{
			{Name: "dev", Order: 0, TriggerType: "Auto", Namespace: "ns-dev", Status: "created"},
		},
	}

	var buf bytes.Buffer
	if err := printPlainDeploymentDetail(&buf, d); err != nil {
		t.Fatalf("printPlainDeploymentDetail error: %v", err)
	}

	if strings.Contains(buf.String(), "Message") {
		t.Errorf("output should not mention messages without diagnostics:\n%s", buf.String())
	}
}
