package list

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
)

func TestListRun_ReasonMessageWhenTasksUnavailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  string
		wantMsg string
	}{
		{
			name:    "running pipeline shows still-running message",
			status:  "UNKNOWN",
			wantMsg: "Pipeline is still running",
		},
		{
			name:    "failed pipeline with no task data shows not-indexed message",
			status:  "FAILURE",
			wantMsg: "Task data is not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "tektonResults.getPipelineRunResults"):
					writeJSON(w, tRPCResponse(map[string]any{
						"results": []any{
							map[string]any{
								"uid":         "uid-1",
								"name":        "edp/results/uid-1",
								"create_time": "2026-04-03T15:31:00Z",
								"update_time": "2026-04-03T15:35:12Z",
								"summary": map[string]any{
									"record": "edp/results/uid-1/records/rec-1",
									"type":   "tekton.dev/v1.PipelineRun",
									"status": tt.status,
								},
								"annotations": map[string]any{
									"object.metadata.name":      "review-abc",
									"app.edp.epam.com/codebase": "edp-tekton",
								},
							},
						},
					}))
				case strings.HasSuffix(r.URL.Path, "tektonResults.getTaskRunRecords"):
					writeJSON(w, tRPCResponse(map[string]any{
						"taskRuns": []any{},
					}))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			client, err := portal.NewClient(srv.URL, func(_ context.Context) (string, error) {
				return "test-token", nil
			}, portal.WithHTTPClient(srv.Client()))
			require.NoError(t, err)

			var buf bytes.Buffer
			opts := &ListOptions{
				IO:            &iostreams.IOStreams{Out: &buf},
				PortalClient:  func() (*portal.Client, error) { return client, nil },
				Config:        func() (*config.Config, error) { return &config.Config{Namespace: "edp"}, nil },
				IncludeReason: true,
			}

			err = listRun(context.Background(), opts)
			require.NoError(t, err)
			assert.Contains(t, buf.String(), tt.wantMsg)
		})
	}
}

func tRPCResponse(data any) map[string]any {
	return map[string]any{
		"result": map[string]any{
			"data": data,
		},
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
