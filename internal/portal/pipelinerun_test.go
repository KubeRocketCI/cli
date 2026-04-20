package portal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// --- matchesFilter ---

func TestMatchesFilter(t *testing.T) {
	t.Parallel()

	base := PipelineRunInfo{
		Name:     "review-my-app-main-abc123",
		Project:  "my-app",
		Status:   StatusSucceeded,
		Type:     "review",
		Branch:   "main",
		Author:   "alice",
		PRNumber: "42",
	}

	tests := []struct {
		name   string
		info   PipelineRunInfo
		filter PipelineRunFilter
		want   bool
	}{
		{
			name:   "empty filter passes everything",
			info:   base,
			filter: PipelineRunFilter{},
			want:   true,
		},
		{
			name:   "project match",
			info:   base,
			filter: PipelineRunFilter{Project: "my-app"},
			want:   true,
		},
		{
			name:   "project mismatch",
			info:   base,
			filter: PipelineRunFilter{Project: "other-app"},
			want:   false,
		},
		{
			name:   "status match succeeded",
			info:   base,
			filter: PipelineRunFilter{Status: "succeeded"},
			want:   true,
		},
		{
			name:   "status mismatch",
			info:   base,
			filter: PipelineRunFilter{Status: "failed"},
			want:   false,
		},
		{
			name:   "type match",
			info:   base,
			filter: PipelineRunFilter{Type: "review"},
			want:   true,
		},
		{
			name:   "type mismatch",
			info:   base,
			filter: PipelineRunFilter{Type: "build"},
			want:   false,
		},
		{
			name:   "branch match",
			info:   base,
			filter: PipelineRunFilter{Branch: "main"},
			want:   true,
		},
		{
			name:   "branch mismatch",
			info:   base,
			filter: PipelineRunFilter{Branch: "feature-x"},
			want:   false,
		},
		{
			name:   "author match",
			info:   base,
			filter: PipelineRunFilter{Author: "alice"},
			want:   true,
		},
		{
			name:   "author mismatch",
			info:   base,
			filter: PipelineRunFilter{Author: "bob"},
			want:   false,
		},
		{
			name:   "PR number match",
			info:   base,
			filter: PipelineRunFilter{PRNumber: 42},
			want:   true,
		},
		{
			name:   "PR number mismatch",
			info:   base,
			filter: PipelineRunFilter{PRNumber: 99},
			want:   false,
		},
		{
			name:   "all fields match",
			info:   base,
			filter: PipelineRunFilter{Project: "my-app", Status: "succeeded", Type: "review", Branch: "main", Author: "alice", PRNumber: 42},
			want:   true,
		},
		{
			name:   "one field mismatch among many",
			info:   base,
			filter: PipelineRunFilter{Project: "my-app", Status: "failed"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := matchesFilter(tt.info, tt.filter)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- matchesStatus ---

func TestMatchesStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		actualStatus string
		filterStatus string
		want         bool
	}{
		{name: "succeeded lowercase", actualStatus: StatusSucceeded, filterStatus: "succeeded", want: true},
		{name: "succeeded mixed case", actualStatus: StatusSucceeded, filterStatus: "Succeeded", want: true},
		{name: "succeeded uppercase", actualStatus: StatusSucceeded, filterStatus: "SUCCEEDED", want: true},
		{name: "failed", actualStatus: StatusFailed, filterStatus: "failed", want: true},
		{name: "timeout", actualStatus: StatusTimeout, filterStatus: "timeout", want: true},
		{name: "cancelled", actualStatus: StatusCancelled, filterStatus: "cancelled", want: true},
		{name: "running", actualStatus: StatusRunning, filterStatus: "running", want: true},
		{name: "status mismatch", actualStatus: StatusSucceeded, filterStatus: "failed", want: false},
		{name: "unknown filter keyword", actualStatus: StatusSucceeded, filterStatus: "pending", want: false},
		{name: "empty filter always false for non-empty actual", actualStatus: StatusSucceeded, filterStatus: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := matchesStatus(tt.actualStatus, tt.filterStatus)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- mapK8sPipelineRunInfo ---

func TestMapK8sPipelineRunInfo(t *testing.T) {
	t.Parallel()

	makeItem := func(name string, labels, annotations map[string]any, spec map[string]any, status map[string]any) *restapi.K8sList_200_Items_Item {
		item := &restapi.K8sList_200_Items_Item{
			Metadata: restapi.K8sList_200_Items_Metadata{
				Name: name,
				AdditionalProperties: map[string]any{
					"labels":            labels,
					"annotations":       annotations,
					"creationTimestamp": "2024-01-01T10:00:00Z",
				},
			},
		}
		if spec != nil {
			item.Spec = &spec
		}
		if status != nil {
			item.Status = &status
		}
		return item
	}

	makeCondition := func(condStatus, reason string) []any {
		return []any{map[string]any{"type": "Succeeded", "status": condStatus, "reason": reason}}
	}

	tests := []struct {
		name       string
		item       *restapi.K8sList_200_Items_Item
		wantStatus string
		wantStart  string
	}{
		{
			name: "Succeeded condition True → StatusSucceeded",
			item: makeItem("run-1", nil, nil, nil, map[string]any{
				"startTime":  "2024-01-01T10:01:00Z",
				"conditions": makeCondition(conditionStatusTrue, ""),
			}),
			wantStatus: StatusSucceeded,
			wantStart:  "2024-01-01T10:01:00Z",
		},
		{
			name: "condition False default reason → StatusFailed",
			item: makeItem("run-2", nil, nil, nil, map[string]any{
				"startTime":  "2024-01-01T10:02:00Z",
				"conditions": makeCondition(conditionStatusFalse, "TaskRunFailed"),
			}),
			wantStatus: StatusFailed,
			wantStart:  "2024-01-01T10:02:00Z",
		},
		{
			name: "condition False reason PipelineRunTimeout → StatusTimeout",
			item: makeItem("run-3", nil, nil, nil, map[string]any{
				"startTime":  "2024-01-01T10:03:00Z",
				"conditions": makeCondition(conditionStatusFalse, "PipelineRunTimeout"),
			}),
			wantStatus: StatusTimeout,
			wantStart:  "2024-01-01T10:03:00Z",
		},
		{
			name: "condition False reason PipelineRunCancelled → StatusCancelled",
			item: makeItem("run-4", nil, nil, nil, map[string]any{
				"startTime":  "2024-01-01T10:04:00Z",
				"conditions": makeCondition(conditionStatusFalse, "PipelineRunCancelled"),
			}),
			wantStatus: StatusCancelled,
			wantStart:  "2024-01-01T10:04:00Z",
		},
		{
			name: "condition False reason Cancelled → StatusCancelled",
			item: makeItem("run-5", nil, nil, nil, map[string]any{
				"startTime":  "2024-01-01T10:05:00Z",
				"conditions": makeCondition(conditionStatusFalse, "Cancelled"),
			}),
			wantStatus: StatusCancelled,
			wantStart:  "2024-01-01T10:05:00Z",
		},
		{
			name: "condition Unknown → StatusRunning",
			item: makeItem("run-6", nil, nil, nil, map[string]any{
				"startTime":  "2024-01-01T10:06:00Z",
				"conditions": makeCondition("Unknown", "Running"),
			}),
			wantStatus: StatusRunning,
			wantStart:  "2024-01-01T10:06:00Z",
		},
		{
			name: "no conditions → empty status",
			item: makeItem("run-7", nil, nil, nil, map[string]any{
				"startTime": "2024-01-01T10:07:00Z",
			}),
			wantStatus: "",
			wantStart:  "2024-01-01T10:07:00Z",
		},
		{
			name: "no startTime → fallback to creationTimestamp",
			item: makeItem("run-8", nil, nil, nil, map[string]any{
				"conditions": makeCondition(conditionStatusTrue, ""),
			}),
			wantStatus: StatusSucceeded,
			wantStart:  "2024-01-01T10:00:00Z",
		},
		{
			// Regression: when item.Status is nil (brand-new run, reconciler hasn't
			// attached status yet), creationTimestamp must still populate StartTime.
			name:       "nil status → fallback to creationTimestamp",
			item:       makeItem("run-fresh", nil, nil, nil, nil),
			wantStatus: "",
			wantStart:  "2024-01-01T10:00:00Z",
		},
		{
			name: "labels extracted correctly",
			item: makeItem("run-9",
				map[string]any{
					annotationCodebase:        "my-app",
					annotationPipelineType:    "review",
					annotationGitBranch:       "main",
					annotationGitAuthor:       "alice",
					annotationGitChangeNumber: "7",
					annotationGitTargetBranch: "main",
				},
				map[string]any{
					annotationGitChangeURL: "https://github.com/org/repo/pull/7",
					annotationGitCommitSHA: "abc123",
				},
				map[string]any{"pipelineRef": map[string]any{"name": "review-pipeline"}},
				map[string]any{
					"startTime":  "2024-01-01T10:09:00Z",
					"conditions": makeCondition(conditionStatusTrue, ""),
				},
			),
			wantStatus: StatusSucceeded,
			wantStart:  "2024-01-01T10:09:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapK8sPipelineRunInfo(tt.item)
			assert.Equal(t, tt.wantStatus, got.Status, "Status")
			assert.Equal(t, tt.wantStart, got.StartTime, "StartTime")
		})
	}
}

func TestMapK8sPipelineRunInfo_Labels(t *testing.T) {
	t.Parallel()

	item := &restapi.K8sList_200_Items_Item{
		Metadata: restapi.K8sList_200_Items_Metadata{
			Name: "run-labels",
			AdditionalProperties: map[string]any{
				"labels": map[string]any{
					annotationCodebase:        "my-app",
					annotationPipelineType:    "review",
					annotationGitBranch:       "feature-x",
					annotationGitAuthor:       "bob",
					annotationGitChangeNumber: "99",
					annotationGitTargetBranch: "main",
				},
				"annotations": map[string]any{
					annotationGitChangeURL: "https://github.com/org/repo/pull/99",
					annotationGitCommitSHA: "deadbeef",
				},
			},
		},
		Spec: func() *map[string]any {
			m := map[string]any{"pipelineRef": map[string]any{"name": "review-pipeline"}}
			return &m
		}(),
	}

	got := mapK8sPipelineRunInfo(item)
	assert.Equal(t, "run-labels", got.Name)
	assert.Equal(t, "my-app", got.Project)
	assert.Equal(t, "review", got.Type)
	assert.Equal(t, "feature-x", got.Branch)
	assert.Equal(t, "bob", got.Author)
	assert.Equal(t, "99", got.PRNumber)
	assert.Equal(t, "main", got.TargetBranch)
	assert.Equal(t, "https://github.com/org/repo/pull/99", got.PRURL)
	assert.Equal(t, "deadbeef", got.CommitSHA)
	assert.Equal(t, "review-pipeline", got.Pipeline)
}

// --- mapPipelineRunInfo (Tekton Results source) ---

// TestMapPipelineRunInfo_PrefersSummaryStartTime verifies that StartTime comes
// from summary.start_time (actual pipeline start) rather than CreateTime (when
// the record was stored in Results, near completion). Using CreateTime would
// cause merged lists to sort Results entries alongside their completion
// timestamps, inverting order against live K8s runs that use status.startTime.
func TestMapPipelineRunInfo_PrefersSummaryStartTime(t *testing.T) {
	t.Parallel()

	summaryStart := "2024-01-01T10:00:00Z"
	recordCreate := "2024-01-01T10:05:00Z" // record stored 5 min after start

	r := &restapi.TektonResult{
		Uid:        "uuid-1",
		Name:       "results/ns/records/r1",
		CreateTime: recordCreate,
		UpdateTime: "2024-01-01T10:05:00Z",
		Summary: &restapi.TektonResultSummary{
			Record:    "results/ns/records/r1",
			Status:    "SUCCESS",
			StartTime: &summaryStart,
			EndTime:   func() *string { s := "2024-01-01T10:04:30Z"; return &s }(),
		},
	}

	got := mapPipelineRunInfo(r)
	assert.Equal(t, summaryStart, got.StartTime, "StartTime must come from summary.start_time")
}

// TestMapPipelineRunInfo_FallsBackToCreateTime ensures the create_time fallback
// still works when summary or summary.start_time is absent.
func TestMapPipelineRunInfo_FallsBackToCreateTime(t *testing.T) {
	t.Parallel()

	t.Run("no summary", func(t *testing.T) {
		t.Parallel()
		r := &restapi.TektonResult{
			Uid:        "uuid-1",
			Name:       "results/ns/records/r1",
			CreateTime: "2024-01-01T10:05:00Z",
			UpdateTime: "2024-01-01T10:05:00Z",
		}
		got := mapPipelineRunInfo(r)
		assert.Equal(t, "2024-01-01T10:05:00Z", got.StartTime)
	})

	t.Run("summary without start_time", func(t *testing.T) {
		t.Parallel()
		r := &restapi.TektonResult{
			Uid:        "uuid-1",
			Name:       "results/ns/records/r1",
			CreateTime: "2024-01-01T10:05:00Z",
			UpdateTime: "2024-01-01T10:05:00Z",
			Summary: &restapi.TektonResultSummary{
				Record: "results/ns/records/r1",
				Status: "SUCCESS",
			},
		}
		got := mapPipelineRunInfo(r)
		assert.Equal(t, "2024-01-01T10:05:00Z", got.StartTime)
	})
}

// --- mergePipelineRuns ---

func TestMergePipelineRuns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		live      []PipelineRunInfo
		history   []PipelineRunInfo
		wantNames []string // ordered by StartTime desc
	}{
		{
			name:      "both empty",
			live:      nil,
			history:   nil,
			wantNames: []string{},
		},
		{
			name: "only live runs",
			live: []PipelineRunInfo{
				{Name: "run-a", StartTime: "2024-01-01T10:02:00Z"},
				{Name: "run-b", StartTime: "2024-01-01T10:01:00Z"},
			},
			history:   nil,
			wantNames: []string{"run-a", "run-b"},
		},
		{
			name: "only history runs",
			live: nil,
			history: []PipelineRunInfo{
				{Name: "run-c", StartTime: "2024-01-01T10:02:00Z"},
				{Name: "run-d", StartTime: "2024-01-01T10:01:00Z"},
			},
			wantNames: []string{"run-c", "run-d"},
		},
		{
			name: "live run deduplicates history entry with same name",
			live: []PipelineRunInfo{
				{Name: "run-a", StartTime: "2024-01-01T10:03:00Z"},
			},
			history: []PipelineRunInfo{
				{Name: "run-a", StartTime: "2024-01-01T10:03:00Z"}, // duplicate
				{Name: "run-b", StartTime: "2024-01-01T10:01:00Z"},
			},
			wantNames: []string{"run-a", "run-b"},
		},
		{
			name: "sort order: newest first",
			live: []PipelineRunInfo{
				{Name: "run-old", StartTime: "2024-01-01T09:00:00Z"},
			},
			history: []PipelineRunInfo{
				{Name: "run-new", StartTime: "2024-01-01T11:00:00Z"},
				{Name: "run-mid", StartTime: "2024-01-01T10:00:00Z"},
			},
			wantNames: []string{"run-new", "run-mid", "run-old"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			all := mergePipelineRuns(tt.live, tt.history)

			require.Len(t, all, len(tt.wantNames), "result length")
			for i, wantName := range tt.wantNames {
				assert.Equal(t, wantName, all[i].Name, "all[%d].Name", i)
			}
		})
	}
}

// TestMergePipelineRuns_ExpansionAttachment verifies that expansion data (logs/tasks)
// is NOT attached when a history run sorts to top but was NOT the run whose expansion
// was fetched (i.e., historyResult.PipelineRuns[0] differs from allRuns[0]).
func TestMergePipelineRuns_ExpansionAttachment(t *testing.T) {
	t.Parallel()

	// History has two entries; expansion was fetched for entry[0] ("run-h1").
	// A live run sorts ahead of "run-h1" so "run-h1" is NOT allRuns[0].
	live := []PipelineRunInfo{
		{Name: "run-live", StartTime: "2024-01-01T12:00:00Z"},
	}
	history := []PipelineRunInfo{
		{Name: "run-h1", StartTime: "2024-01-01T11:00:00Z"}, // expansion fetched for this
		{Name: "run-h2", StartTime: "2024-01-01T10:00:00Z"},
	}

	allRuns := mergePipelineRuns(live, history)

	require.Len(t, allRuns, 3)
	// run-live is first — different from history[0] ("run-h1")
	assert.Equal(t, "run-live", allRuns[0].Name)

	// Simulate what List does with the expansion guard.
	historyFirstName := history[0].Name
	shouldAttach := len(allRuns) > 0 && len(history) > 0 && allRuns[0].Name == historyFirstName
	assert.False(t, shouldAttach, "expansion must NOT be attached when live run sorted ahead")

	// Scenario 2: no live run — history[0] stays at top → expansion SHOULD be attached.
	allRuns2 := mergePipelineRuns(nil, history)
	require.Len(t, allRuns2, 2)
	assert.Equal(t, "run-h1", allRuns2[0].Name)
	shouldAttach2 := len(allRuns2) > 0 && len(history) > 0 && allRuns2[0].Name == historyFirstName
	assert.True(t, shouldAttach2, "expansion SHOULD be attached when history[0] is still top")
}

// --- Sort precision regression ---

// TestMergePipelineRuns_SortAcrossPrecisions verifies that RFC3339 timestamps
// with different precisions (Tekton Results emits nanoseconds, K8s emits
// seconds) sort correctly when merged. Lexicographic string comparison
// mis-orders them because '.' (0x2E) < 'Z' (0x5A) in ASCII.
func TestMergePipelineRuns_SortAcrossPrecisions(t *testing.T) {
	t.Parallel()

	// Same wall-clock instant expressed at different precisions; nano form is later.
	live := []PipelineRunInfo{
		{Name: "k8s-run", StartTime: "2024-01-01T10:00:00Z"},
	}
	history := []PipelineRunInfo{
		{Name: "results-run", StartTime: "2024-01-01T10:00:00.500000000Z"},
	}

	all := mergePipelineRuns(live, history)

	require.Len(t, all, 2)
	assert.Equal(t, "results-run", all[0].Name,
		"run with later nanosecond timestamp must sort first; got %v", []string{all[0].Name, all[1].Name})
}

// --- Get / List error paths (mock HTTP server) ---

// pathHandlers routes requests by URL path to per-path handlers. Unmatched
// paths fail the test.
type pathHandlers map[string]http.HandlerFunc

func newMockPortal(t *testing.T, handlers pathHandlers) *PipelineRunService {
	t.Helper()

	mux := http.NewServeMux()
	for path, h := range handlers {
		mux.HandleFunc(path, h)
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := restapi.NewClientWithResponses(srv.URL)
	require.NoError(t, err)

	return NewPipelineRunService(client, "", "", "ns")
}

func writeJSONOK(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func TestGet_KubernetesHardErrorPropagates(t *testing.T) {
	t.Parallel()

	// K8s returns 401 — must NOT silently fall through to Tekton Results.
	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
		"/v1/pipeline-runs": func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("Tekton Results must not be called when K8s returns a hard error")
		},
	})

	_, err := s.Get(context.Background(), "run-x", PipelineRunGetOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnauthorized, "hard K8s error must propagate to caller")
}

func TestGet_RunAbsentInK8sFallsThroughToResults(t *testing.T) {
	t.Parallel()

	tektonCalled := false
	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			// Empty items list → getLiveRun returns ErrNotFound.
			writeJSONOK(w, `{"apiVersion":"v1","kind":"List","items":[],"metadata":{}}`)
		},
		"/v1/pipeline-runs": func(w http.ResponseWriter, _ *http.Request) {
			tektonCalled = true
			writeJSONOK(w, `{"results":[{
				"name":"results/ns/records/uuid-1","uid":"uuid-1",
				"create_time":"2024-01-01T10:00:00Z","update_time":"2024-01-01T10:05:00Z",
				"annotations":{"object.metadata.name":"run-x"},
				"summary":{"record":"results/a/records/b","status":"SUCCESS"}
			}]}`)
		},
	})

	result, err := s.Get(context.Background(), "run-x", PipelineRunGetOptions{})
	require.NoError(t, err)
	assert.True(t, tektonCalled, "ErrNotFound from K8s should trigger Tekton Results fallback")
	require.Len(t, result.PipelineRuns, 1)
	assert.Equal(t, "run-x", result.PipelineRuns[0].Name)
}

func TestGet_RunningPipelineSkipsResults(t *testing.T) {
	t.Parallel()

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			// Item with Unknown condition → StatusRunning
			writeJSONOK(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[{
				"metadata":{"name":"run-running"},
				"status":{"startTime":"2024-01-01T10:00:00Z","conditions":[
					{"type":"Succeeded","status":"Unknown","reason":"Running"}
				]}
			}]}`)
		},
		"/v1/pipeline-runs": func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("Tekton Results must not be called for still-running pipeline")
		},
	})

	result, err := s.Get(context.Background(), "run-running", PipelineRunGetOptions{IncludeLogs: true})
	require.NoError(t, err)
	require.Len(t, result.PipelineRuns, 1)
	assert.Equal(t, StatusRunning, result.PipelineRuns[0].Status)
}

func TestGet_CompletedK8sResultsNotFoundFallsBackToK8s(t *testing.T) {
	t.Parallel()

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[{
				"metadata":{"name":"run-done"},
				"status":{"startTime":"2024-01-01T10:00:00Z","conditions":[
					{"type":"Succeeded","status":"True","reason":"Succeeded"}
				]}
			}]}`)
		},
		"/v1/pipeline-runs": func(w http.ResponseWriter, _ *http.Request) {
			// No results yet indexed → ErrNotFound from getFromResults, expected fallback.
			writeJSONOK(w, `{"results":[]}`)
		},
	})

	result, err := s.Get(context.Background(), "run-done", PipelineRunGetOptions{IncludeLogs: true})
	require.NoError(t, err, "Tekton ErrNotFound must fall through to K8s result")
	require.Len(t, result.PipelineRuns, 1)
	assert.Equal(t, "run-done", result.PipelineRuns[0].Name)
	assert.Equal(t, StatusSucceeded, result.PipelineRuns[0].Status)
}

func TestGet_CompletedK8sResultsHardErrorPropagates(t *testing.T) {
	t.Parallel()

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[{
				"metadata":{"name":"run-done"},
				"status":{"startTime":"2024-01-01T10:00:00Z","conditions":[
					{"type":"Succeeded","status":"True","reason":"Succeeded"}
				]}
			}]}`)
		},
		"/v1/pipeline-runs": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	})

	_, err := s.Get(context.Background(), "run-done", PipelineRunGetOptions{IncludeLogs: true})
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotFound), "500 from Tekton Results must not be reduced to ErrNotFound")
}

// TestList_SourceErrorPropagates covers both legs of the errgroup fan-out.
func TestList_SourceErrorPropagates(t *testing.T) {
	t.Parallel()

	emptyK8sOK := `{"apiVersion":"v1","kind":"List","items":[],"metadata":{}}`
	emptyResultsOK := `{"results":[]}`

	tests := []struct {
		name       string
		k8sHandler http.HandlerFunc
		tekHandler http.HandlerFunc
		wantErrIs  error
	}{
		{
			name: "K8s list 401",
			k8sHandler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			tekHandler: func(w http.ResponseWriter, _ *http.Request) { writeJSONOK(w, emptyResultsOK) },
			wantErrIs:  ErrUnauthorized,
		},
		{
			name:       "Tekton Results 401",
			k8sHandler: func(w http.ResponseWriter, _ *http.Request) { writeJSONOK(w, emptyK8sOK) },
			tekHandler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErrIs: ErrUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newMockPortal(t, pathHandlers{
				"/v1/resources/list": tt.k8sHandler,
				"/v1/pipeline-runs":  tt.tekHandler,
			})

			_, err := s.List(context.Background(), PipelineRunListOptions{})
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErrIs)
		})
	}
}

// TestList_RunningFilterSkipsResults verifies that the "running" status filter
// short-circuits the Tekton Results round trip.
func TestList_RunningFilterSkipsResults(t *testing.T) {
	t.Parallel()

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[{
				"metadata":{"name":"run-running"},
				"status":{"startTime":"2024-01-01T10:00:00Z","conditions":[
					{"type":"Succeeded","status":"Unknown","reason":"Running"}
				]}
			}]}`)
		},
		"/v1/pipeline-runs": func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("Tekton Results must not be called when filter=running")
		},
	})

	result, err := s.List(context.Background(), PipelineRunListOptions{
		Filter: PipelineRunFilter{Status: strings.ToLower(StatusRunning)},
	})
	require.NoError(t, err)
	require.Len(t, result.PipelineRuns, 1)
	assert.Equal(t, StatusRunning, result.PipelineRuns[0].Status)
}
