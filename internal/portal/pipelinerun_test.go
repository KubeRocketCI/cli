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

func TestPipelineRunService_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      PipelineRunListOptions
		handler   http.HandlerFunc
		wantErr   bool
		wantCount int
		wantFirst string
	}{
		{
			name: "returns pipeline runs",
			opts: PipelineRunListOptions{Filter: PipelineRunFilter{Project: "edp-tekton"}},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tRPCResponse(tektonResultsList{
					Results: []tektonResult{
						makeResult("uid-1", "review-edp-tekton-main-abc", resultStatusFailure, "2026-04-03T15:31:00Z", "2026-04-03T15:35:12Z"),
						makeResult("uid-2", "review-edp-tekton-main-def", resultStatusSuccess, "2026-04-02T10:15:00Z", "2026-04-02T10:18:45Z"),
					},
				}))
			},
			wantCount: 2,
			wantFirst: "review-edp-tekton-main-abc",
		},
		{
			name: "empty list",
			opts: PipelineRunListOptions{Filter: PipelineRunFilter{Project: "edp-tekton"}},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tRPCResponse(tektonResultsList{}))
			},
			wantCount: 0,
		},
		{
			name: "portal error",
			opts: PipelineRunListOptions{Filter: PipelineRunFilter{Project: "edp-tekton"}},
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

			svc := NewPipelineRunService(newTestClient(t, srv), "", "", "edp")

			result, err := svc.List(context.Background(), tt.opts)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, result.PipelineRuns, tt.wantCount)

			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirst, result.PipelineRuns[0].Name)
			}
		})
	}
}

func TestPipelineRunService_List_WithLogs(t *testing.T) {
	t.Parallel()

	callCount := 0

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		path := r.URL.Path

		switch {
		case strings.HasSuffix(path, "tektonResults.getPipelineRunResults"):
			writeJSON(w, tRPCResponse(tektonResultsList{
				Results: []tektonResult{
					makeResultWithRecord("uid-1", "review-abc", resultStatusSuccess,
						"2026-04-03T15:31:00Z", "2026-04-03T15:35:12Z",
						"edp/results/uid-1/records/rec-1"),
				},
			}))

		case strings.HasSuffix(path, "tektonResults.getPipelineRunLogs"):
			writeJSON(w, tRPCResponse(pipelineRunLogsResponse{
				Logs: "[compile] build succeeded\n",
			}))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	svc := NewPipelineRunService(newTestClient(t, srv), "", "", "edp")

	result, err := svc.List(context.Background(), PipelineRunListOptions{
		Filter:      PipelineRunFilter{Project: "edp-tekton"},
		IncludeLogs: true,
	})

	require.NoError(t, err)
	assert.Len(t, result.PipelineRuns, 1)
	assert.Contains(t, result.Logs, "build succeeded")
	assert.Equal(t, 2, callCount, "should make 2 API calls: results + logs")
}

func TestPipelineRunService_List_NoLogsWithoutFlag(t *testing.T) {
	t.Parallel()

	callCount := 0

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		writeJSON(w, tRPCResponse(tektonResultsList{
			Results: []tektonResult{
				makeResult("uid-1", "review-abc", resultStatusSuccess, "2026-04-03T15:31:00Z", "2026-04-03T15:35:12Z"),
			},
		}))
	}))
	defer srv.Close()

	svc := NewPipelineRunService(newTestClient(t, srv), "", "", "edp")

	result, err := svc.List(context.Background(), PipelineRunListOptions{
		Filter: PipelineRunFilter{Project: "edp-tekton"},
	})

	require.NoError(t, err)
	assert.Len(t, result.PipelineRuns, 1)
	assert.Empty(t, result.Logs)
	assert.Equal(t, 1, callCount, "should only make 1 API call without --logs")
}

func TestBuildCELFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter PipelineRunFilter
		want   string
	}{
		{
			name: "no filters",
			want: "",
		},
		{
			name:   "codebase only",
			filter: PipelineRunFilter{Project: "edp-tekton"},
			want:   `annotations["app.edp.epam.com/codebase"] == "edp-tekton"`,
		},
		{
			name:   "codebase and PR",
			filter: PipelineRunFilter{Project: "edp-tekton", PRNumber: 44},
			want:   `annotations["app.edp.epam.com/codebase"] == "edp-tekton" && annotations["app.edp.epam.com/git-change-number"] == "44"`,
		},
		{
			name:   "author only",
			filter: PipelineRunFilter{Author: "John Doe"},
			want:   `annotations["app.edp.epam.com/git-author"] == "John Doe"`,
		},
		{
			name:   "type and branch",
			filter: PipelineRunFilter{Type: "review", Branch: "main"},
			want:   `annotations["app.edp.epam.com/git-branch"] == "main" && annotations["app.edp.epam.com/pipelinetype"] == "review"`,
		},
		{
			name:   "all filters combined",
			filter: PipelineRunFilter{Project: "edp-tekton", Author: "Jane", Branch: "fix/bug", Type: "build", PRNumber: 10},
			want:   `annotations["app.edp.epam.com/codebase"] == "edp-tekton" && annotations["app.edp.epam.com/git-author"] == "Jane" && annotations["app.edp.epam.com/git-branch"] == "fix/bug" && annotations["app.edp.epam.com/pipelinetype"] == "build" && annotations["app.edp.epam.com/git-change-number"] == "10"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildCELFilter(tt.filter)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRecordName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantResult string
		wantRecord string
	}{
		{
			name:       "valid record name",
			input:      "edp/results/abc-123/records/def-456",
			wantResult: "abc-123",
			wantRecord: "def-456",
		},
		{
			name:       "empty string",
			input:      "",
			wantResult: "",
			wantRecord: "",
		},
		{
			name:       "too few parts",
			input:      "edp/results/abc",
			wantResult: "",
			wantRecord: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resultUID, recordUID := parseRecordName(tt.input)
			assert.Equal(t, tt.wantResult, resultUID)
			assert.Equal(t, tt.wantRecord, recordUID)
		})
	}
}

func TestDisplayStatus(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Succeeded", displayStatus("SUCCESS"))
	assert.Equal(t, "Failed", displayStatus("FAILURE"))
	assert.Equal(t, "Timeout", displayStatus("TIMEOUT"))
	assert.Equal(t, "Cancelled", displayStatus("CANCELLED"))
	assert.Equal(t, "Running", displayStatus("UNKNOWN"))
	assert.Equal(t, "WEIRD", displayStatus("WEIRD"))
}

func TestComputeDuration(t *testing.T) {
	t.Parallel()

	r := &tektonResult{
		CreateTime: "2026-04-03T15:31:00Z",
		UpdateTime: "2026-04-03T15:35:12Z",
		Summary: &tektonResultSummary{
			Status: resultStatusSuccess,
			End:    "2026-04-03T15:35:12Z",
		},
	}

	assert.Equal(t, "4m 12s", computeDuration(r))
}

func TestComputeDuration_FallbackToUpdateTime(t *testing.T) {
	t.Parallel()

	r := &tektonResult{
		CreateTime: "2026-04-03T15:31:00Z",
		UpdateTime: "2026-04-03T15:35:12Z",
		Summary: &tektonResultSummary{
			Status: resultStatusFailure,
			End:    "",
		},
	}

	assert.Equal(t, "4m 12s", computeDuration(r))
}

func TestComputeDuration_RunningReturnsEmpty(t *testing.T) {
	t.Parallel()

	r := &tektonResult{
		CreateTime: "2026-04-03T15:31:00Z",
		Summary: &tektonResultSummary{
			Status: resultStatusUnknown,
		},
	}

	assert.Equal(t, "", computeDuration(r))
}

func TestStripANSI(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "hello world", stripANSI("\x1b[31mhello\x1b[0m world"))
	assert.Equal(t, "plain text", stripANSI("plain text"))
}

func TestMapPipelineRunInfo(t *testing.T) {
	t.Parallel()

	r := makeResult("uid-1", "review-abc", resultStatusFailure, "2026-04-03T15:31:00Z", "2026-04-03T15:35:12Z")
	info := mapPipelineRunInfo(&r)

	assert.Equal(t, "review-abc", info.Name)
	assert.Equal(t, "Failed", info.Status)
	assert.Equal(t, "github-go-app-review", info.Pipeline)
	assert.Equal(t, "edp-tekton", info.Project)
	assert.Equal(t, "fix/trigger", info.Branch)
	assert.Equal(t, "44", info.PRNumber)
	assert.Equal(t, "https://github.com/org/repo/pull/44", info.PRURL)
	assert.Equal(t, "John Doe", info.Author)
	assert.Equal(t, "review", info.Type)
	assert.Equal(t, "2026-04-03T15:31:00Z", info.StartTime) // raw RFC3339 for JSON consumers
	assert.Equal(t, "4m 12s", info.Duration)
	assert.Equal(t, "main", info.TargetBranch)
}

func TestPipelineRunService_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		runName  string
		handler  http.HandlerFunc
		wantErr  bool
		wantName string
	}{
		{
			name:    "returns pipeline run by name",
			runName: "review-edp-tekton-main-abc",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tRPCResponse(tektonResultsList{
					Results: []tektonResult{
						makeResult("uid-1", "review-edp-tekton-main-abc", resultStatusFailure, "2026-04-03T15:31:00Z", "2026-04-03T15:35:12Z"),
					},
				}))
			},
			wantName: "review-edp-tekton-main-abc",
		},
		{
			name:    "not found",
			runName: "nonexistent-run",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tRPCResponse(tektonResultsList{}))
			},
			wantErr: true,
		},
		{
			name:    "portal error",
			runName: "review-abc",
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

			svc := NewPipelineRunService(newTestClient(t, srv), "", "", "edp")

			result, err := svc.Get(context.Background(), tt.runName, PipelineRunGetOptions{})

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Len(t, result.PipelineRuns, 1)
			assert.Equal(t, tt.wantName, result.PipelineRuns[0].Name)
		})
	}
}

// --- Test helpers ---

func makeResult(uid, name, status, createTime, updateTime string) tektonResult {
	return tektonResult{
		UID:        uid,
		Name:       "edp/results/" + uid,
		CreateTime: createTime,
		UpdateTime: updateTime,
		Summary: &tektonResultSummary{
			Record: "edp/results/" + uid + "/records/rec-" + uid,
			Type:   "tekton.dev/v1.PipelineRun",
			Status: status,
			Start:  createTime,
			End:    updateTime,
		},
		Annotations: map[string]any{
			annotationObjectName:      name,
			annotationPipeline:        "github-go-app-review",
			annotationCodebase:        "edp-tekton",
			annotationPipelineType:    "review",
			annotationGitChangeNumber: "44",
			annotationGitChangeURL:    "https://github.com/org/repo/pull/44",
			annotationGitAuthor:       "John Doe",
			annotationGitBranch:       "fix/trigger",
			annotationGitTargetBranch: "main",
			annotationGitCommitSHA:    "a1b2c3d4e5f6",
			annotationGitRepository:   "org/repo",
		},
	}
}

func makeResultWithRecord(uid, name, status, createTime, updateTime, record string) tektonResult {
	r := makeResult(uid, name, status, createTime, updateTime)
	r.Summary.Record = record

	return r
}
