package portal

import (
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

func TestNewK8sResourceConfig_APIVersion(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		rc   restapi.K8sListJSONBody
		want string
	}{
		"a kind of the core group": {rc: podResourceConfig, want: "v1"},
		"a kind of a named group":  {rc: stageResourceConfig, want: "v2.edp.epam.com/v1"},
	}

	for name, tc := range cases {
		if got := tc.rc.ResourceConfig.ApiVersion; got != tc.want {
			t.Errorf("%s: apiVersion = %q, want %q", name, got, tc.want)
		}
	}
}
