package get

import (
	"errors"
	"net/http"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

func TestGet_NotFoundKeepsTheSentinel(t *testing.T) {
	t.Parallel()

	f, _ := cmdtest.NewPortalFactory(t, cmdtest.PortalReply(http.StatusNotFound, `{"error":{"code":"NOT_FOUND","message":"Not Found"}}`))

	err := cmdtest.Execute(NewCmdGet, f, []string{"my-app"})
	if err == nil {
		t.Fatal("expected an error for a missing project")
	}

	if want := `project "my-app" not found`; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}

	if !errors.Is(err, portal.ErrNotFound) {
		t.Errorf("errors.Is(%q, portal.ErrNotFound) = false, want true", err)
	}
}
