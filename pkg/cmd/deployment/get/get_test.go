package get

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

func TestPrintDeploymentDetail_Messages(t *testing.T) {
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
	if err := printDeploymentDetail(&buf, false, d); err != nil {
		t.Fatalf("printDeploymentDetail error: %v", err)
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

func TestPrintDeploymentDetail_NoMessages(t *testing.T) {
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
	if err := printDeploymentDetail(&buf, false, d); err != nil {
		t.Fatalf("printDeploymentDetail error: %v", err)
	}

	if strings.Contains(buf.String(), "Message") {
		t.Errorf("output should not mention messages without diagnostics:\n%s", buf.String())
	}
}

func TestGet_NotFoundKeepsTheSentinel(t *testing.T) {
	t.Parallel()

	f, _ := cmdtest.NewPortalFactory(t, cmdtest.PortalReply(http.StatusNotFound, `{"error":{"code":"NOT_FOUND","message":"Not Found"}}`))

	err := cmdtest.Execute(NewCmdGet, f, []string{"my-deployment"})
	if err == nil {
		t.Fatal("expected an error for a missing deployment")
	}

	if want := `deployment "my-deployment" not found`; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}

	if !errors.Is(err, portal.ErrNotFound) {
		t.Errorf("errors.Is(%q, portal.ErrNotFound) = false, want true", err)
	}
}

func TestGet_RejectsInvalidNameBeforeTheRequest(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"demo x", "demo=a", "Demo"} {
		f, _ := cmdtest.NewPortalFactory(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Errorf("%q: portal was called", name)
		}))

		err := cmdtest.Execute(NewCmdGet, f, []string{name})
		if err == nil || !strings.Contains(err.Error(), "DNS-1123") {
			t.Errorf("%q: error = %v, want a DNS-1123 message", name, err)
		}
	}
}
