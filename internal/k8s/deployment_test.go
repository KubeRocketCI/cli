package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

const (
	testPipelineName  = "my-app-pipe"
	testPipelineName2 = "other-pipe"
)

func newFakeCDPipeline(name string, spec, status map[string]any) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "v2.edp.epam.com",
		Version: "v1",
		Kind:    "CDPipeline",
	})
	obj.SetName(name)
	obj.SetNamespace(testNamespace)

	if spec != nil {
		obj.Object["spec"] = spec
	}

	if status != nil {
		obj.Object["status"] = status
	}

	return obj
}

func newFakeStage(name string, spec, status map[string]any) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "v2.edp.epam.com",
		Version: "v1",
		Kind:    "Stage",
	})
	obj.SetName(name)
	obj.SetNamespace(testNamespace)

	if spec != nil {
		obj.Object["spec"] = spec
	}

	if status != nil {
		obj.Object["status"] = status
	}

	return obj
}

func newFakeDeploymentClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "v2.edp.epam.com", Version: "v1", Kind: "CDPipelineList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "v2.edp.epam.com", Version: "v1", Kind: "StageList"},
		&unstructured.UnstructuredList{},
	)

	return dynamicfake.NewSimpleDynamicClient(scheme, objects...)
}

func TestDeploymentService_List(t *testing.T) {
	t.Parallel()

	pipe1 := newFakeCDPipeline(testPipelineName, map[string]any{
		"applications": []any{"user-svc", "order-svc"},
		"description":  "Main pipeline",
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	pipe2 := newFakeCDPipeline(testPipelineName2, map[string]any{
		"applications": []any{"frontend"},
	}, map[string]any{
		"status":    "in_progress",
		"available": false,
	})

	stage1 := newFakeStage("dev-stage", map[string]any{
		"name":        "dev",
		"cdPipeline":  testPipelineName,
		"order":       int64(0),
		"triggerType": "Auto",
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	stage2 := newFakeStage("prod-stage", map[string]any{
		"name":        "prod",
		"cdPipeline":  testPipelineName,
		"order":       int64(2),
		"triggerType": "Manual",
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	stage3 := newFakeStage("qa-stage", map[string]any{
		"name":        "qa",
		"cdPipeline":  testPipelineName,
		"order":       int64(1),
		"triggerType": "Manual",
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	client := newFakeDeploymentClient(pipe1, pipe2, stage1, stage2, stage3)
	svc := NewDeploymentService(client, testNamespace)

	deployments, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, deployments, 2)

	d1 := deployments[0]
	assert.Equal(t, testPipelineName, d1.Name)
	assert.Equal(t, testNamespace, d1.Namespace)
	assert.Equal(t, []string{"user-svc", "order-svc"}, d1.Applications)
	assert.Equal(t, "Main pipeline", d1.Description)
	assert.Equal(t, "created", d1.Status)
	assert.True(t, d1.Available)
	assert.Equal(t, []string{"dev", "qa", "prod"}, d1.StageNames)

	d2 := deployments[1]
	assert.Equal(t, testPipelineName2, d2.Name)
	assert.Equal(t, []string{"frontend"}, d2.Applications)
	assert.Equal(t, "in_progress", d2.Status)
	assert.False(t, d2.Available)
	assert.Empty(t, d2.StageNames)
}

func TestDeploymentService_List_Empty(t *testing.T) {
	t.Parallel()

	client := newFakeDeploymentClient()
	svc := NewDeploymentService(client, testNamespace)

	deployments, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, deployments)
}

func TestDeploymentService_Get(t *testing.T) {
	t.Parallel()

	pipe := newFakeCDPipeline(testPipelineName, map[string]any{
		"applications": []any{"user-svc", "order-svc"},
		"description":  "Main pipeline",
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	stageProd := newFakeStage("prod-stage", map[string]any{
		"name":        "prod",
		"cdPipeline":  testPipelineName,
		"order":       int64(2),
		"triggerType": "Manual",
		"namespace":   "my-project-prod",
		"clusterName": "prod-cluster",
		"description": "Production stage",
		"qualityGates": []any{
			map[string]any{"stepName": "sca", "qualityGateType": "autotests"},
			map[string]any{"stepName": "approval", "qualityGateType": "manual"},
		},
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	stageDev := newFakeStage("dev-stage", map[string]any{
		"name":        "dev",
		"cdPipeline":  testPipelineName,
		"order":       int64(0),
		"triggerType": "Auto",
		"namespace":   "my-project-dev",
		"qualityGates": []any{
			map[string]any{"stepName": "smoke-tests", "qualityGateType": "autotests"},
		},
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	client := newFakeDeploymentClient(pipe, stageProd, stageDev)
	svc := NewDeploymentService(client, testNamespace)

	detail, err := svc.Get(context.Background(), testPipelineName)
	require.NoError(t, err)
	assert.Equal(t, testPipelineName, detail.Name)
	assert.Equal(t, testNamespace, detail.Namespace)
	assert.Equal(t, []string{"user-svc", "order-svc"}, detail.Applications)
	assert.Equal(t, "Main pipeline", detail.Description)
	assert.Equal(t, "created", detail.Status)
	assert.True(t, detail.Available)

	require.Len(t, detail.Stages, 2)
	assert.Equal(t, "dev", detail.Stages[0].Name)
	assert.Equal(t, int64(0), detail.Stages[0].Order)
	assert.Equal(t, "Auto", detail.Stages[0].TriggerType)
	assert.Equal(t, "my-project-dev", detail.Stages[0].Namespace)

	assert.Equal(t, "prod", detail.Stages[1].Name)
	assert.Equal(t, int64(2), detail.Stages[1].Order)
	assert.Equal(t, "Manual", detail.Stages[1].TriggerType)
	assert.Equal(t, "my-project-prod", detail.Stages[1].Namespace)
	assert.Equal(t, "prod-cluster", detail.Stages[1].ClusterName)
	assert.Equal(t, "Production stage", detail.Stages[1].Description)
}

func TestDeploymentService_Get_NotFound(t *testing.T) {
	t.Parallel()

	client := newFakeDeploymentClient()
	svc := NewDeploymentService(client, testNamespace)

	_, err := svc.Get(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestDeploymentService_StageFilteringByCDPipeline(t *testing.T) {
	t.Parallel()

	pipe := newFakeCDPipeline(testPipelineName, map[string]any{
		"applications": []any{"app1"},
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	matchingStage := newFakeStage("my-stage", map[string]any{
		"name":        "dev",
		"cdPipeline":  testPipelineName,
		"order":       int64(0),
		"triggerType": "Auto",
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	otherStage := newFakeStage("other-stage", map[string]any{
		"name":        "staging",
		"cdPipeline":  "different-pipeline",
		"order":       int64(0),
		"triggerType": "Auto",
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	client := newFakeDeploymentClient(pipe, matchingStage, otherStage)
	svc := NewDeploymentService(client, testNamespace)

	detail, err := svc.Get(context.Background(), testPipelineName)
	require.NoError(t, err)
	require.Len(t, detail.Stages, 1)
	assert.Equal(t, "dev", detail.Stages[0].Name)
}

func TestDeploymentService_QualityGateExtraction(t *testing.T) {
	t.Parallel()

	pipe := newFakeCDPipeline(testPipelineName, map[string]any{
		"applications": []any{"app1"},
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	stage := newFakeStage("qa-stage", map[string]any{
		"name":        "qa",
		"cdPipeline":  testPipelineName,
		"order":       int64(0),
		"triggerType": "Manual",
		"qualityGates": []any{
			map[string]any{"stepName": "integration", "qualityGateType": "autotests"},
			map[string]any{"stepName": "approval", "qualityGateType": "manual"},
			map[string]any{"stepName": "sca", "qualityGateType": "autotests"},
		},
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	client := newFakeDeploymentClient(pipe, stage)
	svc := NewDeploymentService(client, testNamespace)

	detail, err := svc.Get(context.Background(), testPipelineName)
	require.NoError(t, err)
	require.Len(t, detail.Stages, 1)
	require.Len(t, detail.Stages[0].QualityGates, 3)

	assert.Equal(t, QualityGate{Name: "integration", Type: "autotests"}, detail.Stages[0].QualityGates[0])
	assert.Equal(t, QualityGate{Name: "approval", Type: "manual"}, detail.Stages[0].QualityGates[1])
	assert.Equal(t, QualityGate{Name: "sca", Type: "autotests"}, detail.Stages[0].QualityGates[2])
}

func TestNewDeploymentService_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	client := newFakeDeploymentClient()
	svc := NewDeploymentService(client, testNamespace)

	require.NotNil(t, svc)
}

func TestCDPipelineGVR(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "v2.edp.epam.com", cdPipelineGVR.Group)
	assert.Equal(t, "v1", cdPipelineGVR.Version)
	assert.Equal(t, "cdpipelines", cdPipelineGVR.Resource)
}

func TestStageGVR(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "v2.edp.epam.com", stageGVR.Group)
	assert.Equal(t, "v1", stageGVR.Version)
	assert.Equal(t, "stages", stageGVR.Resource)
}

func TestMapDeployment_MissingFields(t *testing.T) {
	t.Parallel()

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v2.edp.epam.com/v1",
			"kind":       "CDPipeline",
			"metadata": map[string]any{
				"name":      "empty",
				"namespace": testNamespace,
			},
		},
	}

	d := mapDeployment(obj)
	assert.Equal(t, "empty", d.Name)
	assert.Equal(t, testNamespace, d.Namespace)
	assert.Nil(t, d.Applications)
	assert.Empty(t, d.Description)
	assert.Empty(t, d.Status)
	assert.False(t, d.Available)
}

func TestExtractQualityGates_Empty(t *testing.T) {
	t.Parallel()

	gates := extractQualityGates(map[string]any{})
	assert.Nil(t, gates)
}

func TestExtractQualityGates_InvalidType(t *testing.T) {
	t.Parallel()

	gates := extractQualityGates(map[string]any{
		"qualityGates": "not-a-slice",
	})
	assert.Nil(t, gates)
}

func TestOrderedStageNames_Empty(t *testing.T) {
	t.Parallel()

	names := orderedStageNames(nil)
	assert.Empty(t, names)
}

func TestMapStage_MissingFields(t *testing.T) {
	t.Parallel()

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v2.edp.epam.com/v1",
			"kind":       "Stage",
			"metadata": map[string]any{
				"name":      "empty-stage",
				"namespace": testNamespace,
			},
		},
	}

	s := mapStage(obj)
	assert.Empty(t, s.Name)
	assert.Equal(t, int64(0), s.Order)
	assert.Empty(t, s.TriggerType)
	assert.Nil(t, s.QualityGates)
	assert.Empty(t, s.Status)
	assert.False(t, s.Available)
}
