package portal

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestFilterAndSortStages_NoMatchIsEmpty(t *testing.T) {
	t.Parallel()

	other := map[string]any{"cdPipeline": "other", "name": "dev"}

	cases := map[string][]k8sItem{
		"no stages":             nil,
		"stages of another one": {{Spec: &other}},
	}

	for name, items := range cases {
		got := filterAndSortStages(items, "demo")
		assert.NotNil(t, got, name)
		assert.Empty(t, got, name)
	}
}

func TestExtractQualityGates_NoGatesIsEmpty(t *testing.T) {
	t.Parallel()

	cases := map[string]map[string]any{
		"nil spec":     nil,
		"key missing":  {"name": "dev"},
		"not an array": {"qualityGates": "manual"},
		"empty array":  {"qualityGates": []any{}},
		"null value":   {"qualityGates": nil},
	}

	for name, spec := range cases {
		got := extractQualityGates(spec)
		assert.NotNil(t, got, name)
		assert.Empty(t, got, name)
	}
}
