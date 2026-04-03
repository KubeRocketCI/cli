package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentService_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantErr    bool
		wantCount  int
		wantFirst  string
		wantStages []string
	}{
		{
			name: "returns deployments with stages ordered by spec.order",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Query().Get("input") != "" && strings.Contains(r.URL.Query().Get("input"), "cdpipeline"):
					writeJSON(w, tRPCResponse(k8sList{
						Items: []map[string]any{
							makeCDPipeline("deploy-prod", "edp", []string{"app1", "app2"}),
						},
					}))
				case r.URL.Query().Get("input") != "" && strings.Contains(r.URL.Query().Get("input"), "stage"):
					writeJSON(w, tRPCResponse(k8sList{
						Items: []map[string]any{
							makeStage("sit", "deploy-prod", 1),
							makeStage("qa", "deploy-prod", 0),
							makeStage("prod", "deploy-prod", 2),
						},
					}))
				}
			},
			wantCount:  1,
			wantFirst:  "deploy-prod",
			wantStages: []string{"qa", "sit", "prod"},
		},
		{
			name: "handles empty pipeline list",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, tRPCResponse(k8sList{Items: []map[string]any{}}))
			},
			wantCount: 0,
		},
		{
			name: "handles pipelines with no matching stages",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Query().Get("input"), "cdpipeline"):
					writeJSON(w, tRPCResponse(k8sList{
						Items: []map[string]any{
							makeCDPipeline("deploy-dev", "edp", []string{"app1"}),
						},
					}))
				case strings.Contains(r.URL.Query().Get("input"), "stage"):
					writeJSON(w, tRPCResponse(k8sList{
						Items: []map[string]any{
							makeStage("sit", "other-pipeline", 0),
						},
					}))
				}
			},
			wantCount:  1,
			wantFirst:  "deploy-dev",
			wantStages: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewTLSServer(tt.handler)
			defer srv.Close()

			svc := NewDeploymentService(newTestClient(t, srv), "test-cluster", "edp")

			deployments, err := svc.List(context.Background())

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, deployments, tt.wantCount)

			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirst, deployments[0].Name)
				assert.Equal(t, tt.wantStages, deployments[0].StageNames)
			}
		})
	}
}

func TestDeploymentService_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pipelineName   string
		handler        http.HandlerFunc
		wantErr        bool
		wantName       string
		wantStageCount int
		wantFirstStage string
	}{
		{
			name:         "returns deployment detail with stages filtered and sorted",
			pipelineName: "deploy-prod",
			handler: func(w http.ResponseWriter, r *http.Request) {
				input := r.URL.Query().Get("input")

				switch {
				case strings.Contains(input, "k8s.get") || (strings.Contains(input, "cdpipeline") && strings.Contains(input, `"name"`)):
					writeJSON(w, tRPCResponse(makeCDPipeline("deploy-prod", "edp", []string{"app1"})))
				case strings.Contains(input, "stage"):
					writeJSON(w, tRPCResponse(k8sList{
						Items: []map[string]any{
							makeStage("prod", "deploy-prod", 2),
							makeStage("sit", "deploy-prod", 0),
							makeStage("qa", "deploy-prod", 1),
							makeStage("unrelated", "other-pipeline", 0),
						},
					}))
				}
			},
			wantName:       "deploy-prod",
			wantStageCount: 3,
			wantFirstStage: "sit",
		},
		{
			name:         "returns error for non-existent pipeline",
			pipelineName: "missing",
			handler: func(w http.ResponseWriter, r *http.Request) {
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

			svc := NewDeploymentService(newTestClient(t, srv), "test-cluster", "edp")

			detail, err := svc.Get(context.Background(), tt.pipelineName)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, detail.Name)
			assert.Len(t, detail.Stages, tt.wantStageCount)

			if tt.wantStageCount > 0 {
				assert.Equal(t, tt.wantFirstStage, detail.Stages[0].Name)
			}
		})
	}
}

func TestMapDeployment(t *testing.T) {
	t.Parallel()

	obj := makeCDPipeline("my-deploy", "platform", []string{"app1", "app2"})
	d := mapDeployment(obj)

	assert.Equal(t, "my-deploy", d.Name)
	assert.Equal(t, "platform", d.Namespace)
	assert.Equal(t, []string{"app1", "app2"}, d.Applications)
	assert.Equal(t, "active", d.Status)
	assert.True(t, d.Available)
}

func TestMapStage(t *testing.T) {
	t.Parallel()

	obj := makeStage("sit", "deploy-prod", 1)
	s := mapStage(obj)

	assert.Equal(t, "sit", s.Name)
	assert.Equal(t, int64(1), s.Order)
	assert.Equal(t, "Manual", s.TriggerType)
	assert.Equal(t, "deploy-prod-sit", s.Namespace)
	assert.Equal(t, "active", s.Status)
	assert.True(t, s.Available)
}

func TestGroupStagesByPipeline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		items     []map[string]any
		wantKeys  []string
		wantCount map[string]int
	}{
		{
			name: "groups correctly",
			items: []map[string]any{
				makeStage("sit", "pipeline-a", 0),
				makeStage("qa", "pipeline-a", 1),
				makeStage("prod", "pipeline-b", 0),
			},
			wantKeys:  []string{"pipeline-a", "pipeline-b"},
			wantCount: map[string]int{"pipeline-a": 2, "pipeline-b": 1},
		},
		{
			name: "skips items without cdPipeline",
			items: []map[string]any{
				makeStage("sit", "pipeline-a", 0),
				{
					"spec":   map[string]any{"name": "orphan", "order": float64(0)},
					"status": map[string]any{"status": "active"},
				},
			},
			wantKeys:  []string{"pipeline-a"},
			wantCount: map[string]int{"pipeline-a": 1},
		},
		{
			name:  "empty input",
			items: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			grouped := groupStagesByPipeline(tt.items)

			if tt.wantKeys == nil {
				assert.Empty(t, grouped)

				return
			}

			for _, key := range tt.wantKeys {
				assert.Contains(t, grouped, key)
				assert.Len(t, grouped[key], tt.wantCount[key])
			}
		})
	}
}

func TestOrderedStageNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stages []Stage
		want   []string
	}{
		{
			name: "sorts by order",
			stages: []Stage{
				{Name: "prod", Order: 2},
				{Name: "dev", Order: 0},
				{Name: "qa", Order: 1},
			},
			want: []string{"dev", "qa", "prod"},
		},
		{
			name:   "empty input",
			stages: nil,
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := orderedStageNames(tt.stages)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterAndSortStages(t *testing.T) {
	t.Parallel()

	items := []map[string]any{
		makeStage("prod", "deploy-prod", 2),
		makeStage("sit", "deploy-prod", 0),
		makeStage("qa", "deploy-prod", 1),
		makeStage("unrelated", "other-pipeline", 0),
	}

	stages := filterAndSortStages(items, "deploy-prod")

	require.Len(t, stages, 3)
	assert.Equal(t, "sit", stages[0].Name)
	assert.Equal(t, "qa", stages[1].Name)
	assert.Equal(t, "prod", stages[2].Name)
}

func TestFilterAndSortStages_NoMatch(t *testing.T) {
	t.Parallel()

	items := []map[string]any{
		makeStage("sit", "pipeline-a", 0),
	}

	stages := filterAndSortStages(items, "nonexistent")
	assert.Empty(t, stages)
}

// --- test helpers ---

func makeCDPipeline(name, ns string, apps []string) map[string]any {
	return map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]any{
			"applications": toAnySlice(apps),
			"description":  "test pipeline",
		},
		"status": map[string]any{
			"status":    "active",
			"available": true,
		},
	}
}

func makeStage(name, cdPipeline string, order int) map[string]any {
	return map[string]any{
		"spec": map[string]any{
			"name":        name,
			"cdPipeline":  cdPipeline,
			"order":       float64(order),
			"triggerType": "Manual",
			"namespace":   cdPipeline + "-" + name,
			"qualityGates": []any{
				map[string]any{
					"stepName":        "autotests",
					"qualityGateType": "autotests",
				},
			},
		},
		"status": map[string]any{
			"status":    "active",
			"available": true,
		},
	}
}

func toAnySlice(ss []string) []any {
	result := make([]any, len(ss))
	for i, s := range ss {
		result[i] = s
	}

	return result
}
