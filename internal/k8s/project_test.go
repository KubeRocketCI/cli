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
	ktesting "k8s.io/client-go/testing"
)

const (
	testCodebaseName = "my-app"
	testNamespace    = "edp"
)

func newFakeCodebase(name, ns string, spec, status map[string]any) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "v2.edp.epam.com",
		Version: "v1",
		Kind:    "Codebase",
	})
	obj.SetName(name)
	obj.SetNamespace(ns)

	if spec != nil {
		obj.Object["spec"] = spec
	}

	if status != nil {
		obj.Object["status"] = status
	}

	return obj
}

func newFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "v2.edp.epam.com", Version: "v1", Kind: "CodebaseList"},
		&unstructured.UnstructuredList{},
	)

	return dynamicfake.NewSimpleDynamicClient(scheme, objects...)
}

func TestProjectService_List(t *testing.T) {
	t.Parallel()

	cb1 := newFakeCodebase(testCodebaseName, testNamespace, map[string]any{
		"type":      "application",
		"lang":      "go",
		"buildTool": "go",
		"framework": "gin",
		"gitServer": "github",
	}, map[string]any{
		"status":    "created",
		"available": true,
		"gitWebUrl": "https://github.com/org/my-app",
	})

	cb2 := newFakeCodebase("my-lib", testNamespace, map[string]any{
		"type":      "library",
		"lang":      "java",
		"buildTool": "maven",
		"gitServer": "gerrit",
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	client := newFakeClient(cb1, cb2)
	svc := NewProjectService(client, testNamespace)

	projects, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, projects, 2)

	// Verify first project fields.
	p := projects[0]
	assert.Equal(t, testCodebaseName, p.Name)
	assert.Equal(t, "application", p.Type)
	assert.Equal(t, "go", p.Language)
	assert.Equal(t, "go", p.BuildTool)
	assert.Equal(t, "gin", p.Framework)
	assert.Equal(t, "created", p.Status)
	assert.True(t, p.Available)
	assert.Equal(t, "https://github.com/org/my-app", p.GitURL)
}

func TestProjectService_List_Empty(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	svc := NewProjectService(client, testNamespace)

	projects, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, projects)
}

func TestProjectService_Get(t *testing.T) {
	t.Parallel()

	cb := newFakeCodebase(testCodebaseName, testNamespace, map[string]any{
		"type":      "application",
		"lang":      "go",
		"buildTool": "go",
		"gitServer": "github",
	}, map[string]any{
		"status":    "created",
		"available": true,
	})

	client := newFakeClient(cb)
	svc := NewProjectService(client, testNamespace)

	p, err := svc.Get(context.Background(), testCodebaseName)
	require.NoError(t, err)
	assert.Equal(t, testCodebaseName, p.Name)
	assert.Equal(t, testNamespace, p.Namespace)
}

func TestProjectService_Get_NotFound(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	svc := NewProjectService(client, testNamespace)

	_, err := svc.Get(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestMapProject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		expected Project
	}{
		{
			name: "missing fields",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v2.edp.epam.com/v1",
					"kind":       "Codebase",
					"metadata": map[string]any{
						"name":      "empty",
						"namespace": testNamespace,
					},
				},
			},
			expected: Project{
				Name:      "empty",
				Namespace: testNamespace,
			},
		},
		{
			name: "spec only",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v2.edp.epam.com/v1",
					"kind":       "Codebase",
					"metadata": map[string]any{
						"name":      "partial",
						"namespace": testNamespace,
					},
					"spec": map[string]any{
						"type":      "library",
						"lang":      "python",
						"buildTool": "pip",
					},
				},
			},
			expected: Project{
				Name:      "partial",
				Namespace: testNamespace,
				Type:      "library",
				Language:  "python",
				BuildTool: "pip",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := mapProject(tt.obj)

			assert.Equal(t, tt.expected.Name, p.Name)
			assert.Equal(t, tt.expected.Type, p.Type)
			assert.Equal(t, tt.expected.Language, p.Language)
			assert.Equal(t, tt.expected.BuildTool, p.BuildTool)
			assert.Equal(t, tt.expected.Status, p.Status)
			assert.Equal(t, tt.expected.Available, p.Available)
		})
	}
}

func TestNewProjectService_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	svc := NewProjectService(client, testNamespace)

	require.NotNil(t, svc)
}

func TestCodebaseGVR(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "v2.edp.epam.com", codebaseGVR.Group)
	assert.Equal(t, "v1", codebaseGVR.Version)
	assert.Equal(t, "codebases", codebaseGVR.Resource)
}

func TestProjectService_Get_VerifyNamespace(t *testing.T) {
	t.Parallel()

	cb := newFakeCodebase(testCodebaseName, "other-ns", map[string]any{
		"type": "application",
	}, nil)

	client := newFakeClient(cb)
	svc := NewProjectService(client, testNamespace)

	// Should not find the codebase in testNamespace namespace.
	_, err := svc.Get(context.Background(), testCodebaseName)
	require.Error(t, err)

	// But works for "other-ns".
	otherSvc := NewProjectService(client, "other-ns")
	p, err := otherSvc.Get(context.Background(), testCodebaseName)
	require.NoError(t, err)
	assert.Equal(t, "other-ns", p.Namespace)
}

func TestProjectService_List_UsesCorrectGVR(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	svc := NewProjectService(client, testNamespace)

	_, err := svc.List(context.Background())
	require.NoError(t, err)

	actions := client.Actions()
	require.Len(t, actions, 1)

	action := actions[0]
	assert.Equal(t, codebaseGVR, action.GetResource())
	assert.Equal(t, testNamespace, action.GetNamespace())
	assert.Equal(t, "list", action.GetVerb())
}

func TestProjectService_Get_UsesCorrectGVR(t *testing.T) {
	t.Parallel()

	cb := newFakeCodebase(testCodebaseName, testNamespace, nil, nil)
	client := newFakeClient(cb)
	svc := NewProjectService(client, testNamespace)

	_, err := svc.Get(context.Background(), testCodebaseName)
	require.NoError(t, err)

	actions := client.Actions()
	require.Len(t, actions, 1)

	action := actions[0]
	assert.Equal(t, codebaseGVR, action.GetResource())
	assert.Equal(t, "get", action.GetVerb())

	// Verify the get action targets the right resource name.
	getAction, ok := action.(ktesting.GetAction)
	require.True(t, ok, "expected GetAction, got %T", action)
	assert.Equal(t, testCodebaseName, getAction.GetName())
}
