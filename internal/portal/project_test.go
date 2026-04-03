package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectService_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		handler   http.HandlerFunc
		wantErr   bool
		wantCount int
		wantFirst string
	}{
		{
			name: "returns projects",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tRPCResponse(k8sList{
					Items: []map[string]any{
						makeCodebase("my-app", "edp"),
						makeCodebase("my-lib", "edp"),
					},
				}))
			},
			wantCount: 2,
			wantFirst: "my-app",
		},
		{
			name: "empty list",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tRPCResponse(k8sList{Items: []map[string]any{}}))
			},
			wantCount: 0,
		},
		{
			name: "portal error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, map[string]any{
					"error": map[string]any{
						"message": "internal error",
						"code":    -32603,
						"data": map[string]any{
							"code":       "INTERNAL_SERVER_ERROR",
							"httpStatus": 500,
						},
					},
				})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewTLSServer(tt.handler)
			defer srv.Close()

			svc := NewProjectService(newTestClient(t, srv), "test-cluster", "edp")

			projects, err := svc.List(context.Background())

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, projects, tt.wantCount)

			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirst, projects[0].Name)
			}
		})
	}
}

func TestProjectService_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		project string
		handler http.HandlerFunc
		wantErr bool
		want    *Project
	}{
		{
			name:    "returns project",
			project: "my-app",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tRPCResponse(makeCodebase("my-app", "edp")))
			},
			want: &Project{
				Name:      "my-app",
				Namespace: "edp",
				Type:      "application",
				Language:  "go",
				BuildTool: "go",
				Framework: "gin",
				GitServer: "github",
				GitURL:    "https://github.com/example/my-app",
				Status:    "active",
				Available: true,
			},
		},
		{
			name:    "returns error for non-existent project",
			project: "missing",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				writeJSON(w, map[string]any{
					"error": map[string]any{
						"message": "not found",
						"code":    -32004,
						"data": map[string]any{
							"code":       "NOT_FOUND",
							"httpStatus": 404,
						},
					},
				})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewTLSServer(tt.handler)
			defer srv.Close()

			svc := NewProjectService(newTestClient(t, srv), "test-cluster", "edp")

			project, err := svc.Get(context.Background(), tt.project)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, project)
		})
	}
}

func TestMapProject(t *testing.T) {
	t.Parallel()

	obj := makeCodebase("my-app", "edp")
	p := mapProject(obj)

	assert.Equal(t, "my-app", p.Name)
	assert.Equal(t, "edp", p.Namespace)
	assert.Equal(t, "application", p.Type)
	assert.Equal(t, "go", p.Language)
	assert.Equal(t, "go", p.BuildTool)
	assert.Equal(t, "gin", p.Framework)
	assert.Equal(t, "github", p.GitServer)
	assert.Equal(t, "https://github.com/example/my-app", p.GitURL)
	assert.Equal(t, "active", p.Status)
	assert.True(t, p.Available)
}

func TestMapProject_DifferentNamespace(t *testing.T) {
	t.Parallel()

	obj := makeCodebase("my-lib", "platform")
	p := mapProject(obj)

	assert.Equal(t, "my-lib", p.Name)
	assert.Equal(t, "platform", p.Namespace)
	assert.Equal(t, "application", p.Type)
	assert.True(t, p.Available)
}

func TestMapProject_EmptyObject(t *testing.T) {
	t.Parallel()

	obj := map[string]any{}
	p := mapProject(obj)

	assert.Equal(t, "", p.Name)
	assert.Equal(t, "", p.Namespace)
	assert.False(t, p.Available)
}

// makeCodebase creates an unstructured Codebase resource for testing.
func makeCodebase(name, ns string) map[string]any {
	return map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]any{
			"type":      "application",
			"lang":      "go",
			"buildTool": "go",
			"framework": "gin",
			"gitServer": "github",
		},
		"status": map[string]any{
			"gitWebUrl": "https://github.com/example/" + name,
			"status":    "active",
			"available": true,
		},
	}
}
