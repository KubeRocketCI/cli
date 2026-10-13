package portal

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// Stages of equal order keep the portal's list order: deployment get and the
// ENVS column of deployment list print the same order on every run.
func TestStageOrder_EqualOrderKeepsListOrder(t *testing.T) {
	t.Parallel()

	// Above 12: sort.Slice is an insertion sort, so stable, up to 12 elements.
	const n = 50

	items := make([]k8sItem, 0, n)
	want := make([]string, 0, n)

	for i := range n {
		name := fmt.Sprintf("env-%02d", n-i)
		spec := map[string]any{"cdPipeline": "demo", "name": name, "order": int64(i % 2)}
		items = append(items, k8sItem{Spec: &spec})
	}

	for _, order := range []int{0, 1} {
		for i := order; i < n; i += 2 {
			want = append(want, fmt.Sprintf("env-%02d", n-i))
		}
	}

	names := func(stages []Stage) []string {
		out := make([]string, 0, len(stages))
		for _, s := range stages {
			out = append(out, s.Name)
		}

		return out
	}

	assert.Equal(t, want, names(filterAndSortStages(items, "demo")))
	assert.Equal(t, want, orderedStageNames(groupStagesByPipeline(items)["demo"]))
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

func TestDeploymentService_Get_ListsOnlyItsStages(t *testing.T) {
	t.Parallel()

	rec := envWithStage(t, "in-cluster")

	svc, closer := newDeploymentServiceForTest(t, rec)
	defer closer()

	detail, err := svc.Get(t.Context(), "my-pipeline")
	require.NoError(t, err)
	assert.Len(t, detail.Stages, 1)

	calls := callsOfKind(rec, "Stage")
	require.Len(t, calls, 1)
	assert.Equal(t, map[string]string{labelCDPipelineName: "my-pipeline"}, calls[0].LabelSelectors)
}
