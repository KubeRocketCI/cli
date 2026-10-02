package portal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
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

	deployRun := PipelineRunInfo{Name: "deploy-demo-dev-ab12", Type: "deploy", Deployment: "demo", Env: "dev"}

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
			name:   "deployment and env match",
			info:   deployRun,
			filter: PipelineRunFilter{Deployment: "demo", Env: "dev"},
			want:   true,
		},
		{
			name:   "deployment match without env",
			info:   deployRun,
			filter: PipelineRunFilter{Deployment: "demo"},
			want:   true,
		},
		{
			name:   "env mismatch",
			info:   deployRun,
			filter: PipelineRunFilter{Deployment: "demo", Env: "qa"},
			want:   false,
		},
		{
			name:   "deployment mismatch",
			info:   deployRun,
			filter: PipelineRunFilter{Deployment: "shop", Env: "dev"},
			want:   false,
		},
		{
			name:   "run without deployment labels never matches a deployment",
			info:   base,
			filter: PipelineRunFilter{Deployment: "demo"},
			want:   false,
		},
		{
			name:   "env without deployment is ignored, as in the Tekton Results filter",
			info:   deployRun,
			filter: PipelineRunFilter{Env: "qa"},
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

// --- buildCELFilter / stageEnv ---

func TestBuildCELFilter_DeploymentAndEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter PipelineRunFilter
		want   string
	}{
		{
			name:   "deployment only",
			filter: PipelineRunFilter{Deployment: "demo"},
			want:   `annotations["app.edp.epam.com/cdpipeline"] == "demo"`,
		},
		{
			name:   "dashes in deployment and env",
			filter: PipelineRunFilter{Deployment: "demo-app", Env: "qa-eu"},
			want: `annotations["app.edp.epam.com/cdpipeline"] == "demo-app" && ` +
				`annotations["app.edp.epam.com/cdstage"] == "demo-app-qa-eu"`,
		},
		{
			name:   "deployment and env select the stage",
			filter: PipelineRunFilter{Deployment: "demo", Env: "dev", Type: "deploy"},
			want: `annotations["app.edp.epam.com/pipelinetype"] == "deploy" && ` +
				`annotations["app.edp.epam.com/cdpipeline"] == "demo" && ` +
				`annotations["app.edp.epam.com/cdstage"] == "demo-dev"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, buildCELFilter(tt.filter))
		})
	}
}

func TestStageEnv(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "dev", stageEnv("demo", "demo-dev"))
	assert.Equal(t, "qa-eu", stageEnv("demo", "demo-qa-eu"), "stage names may contain dashes")
	assert.Equal(t, "", stageEnv("", "demo-dev"), "without the deployment the stage cannot be split safely")
	assert.Equal(t, "", stageEnv("shop", "demo-dev"), "a cdstage of another deployment is not an env of this one")
	assert.Equal(t, "", stageEnv("demo", ""))
}

func TestStageEnv_InvertsCDStage(t *testing.T) {
	t.Parallel()

	for _, env := range []string{"dev", "qa-eu"} {
		f := PipelineRunFilter{Deployment: "demo", Env: env}
		assert.Equal(t, f.Env, stageEnv(f.Deployment, f.cdStage()))
	}
}

// --- mapK8sPipelineRunInfo ---

func TestMapK8sPipelineRunInfo_DeploymentAndEnv(t *testing.T) {
	t.Parallel()

	labels := map[string]string{
		annotationPipelineType: "deploy",
		annotationCDPipeline:   "demo",
		annotationCDStage:      "demo-dev",
	}
	item := &restapi.K8sList_200_Items_Item{
		Metadata: restapi.K8sList_200_Items_Metadata{Name: "deploy-demo-dev-ab12", Labels: &labels},
	}

	got := mapK8sPipelineRunInfo(item)
	assert.Equal(t, "demo", got.Deployment)
	assert.Equal(t, "dev", got.Env, "env is the stage name, not the Stage resource name")
}

func TestMapK8sPipelineRunInfo(t *testing.T) {
	t.Parallel()

	makeItem := func(name string, labels, annotations map[string]string, spec map[string]any, status map[string]any) *restapi.K8sList_200_Items_Item {
		item := &restapi.K8sList_200_Items_Item{
			Metadata: restapi.K8sList_200_Items_Metadata{
				Name:              name,
				CreationTimestamp: ptr.To("2024-01-01T10:00:00Z"),
			},
		}
		if labels != nil {
			item.Metadata.Labels = &labels
		}
		if annotations != nil {
			item.Metadata.Annotations = &annotations
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
				map[string]string{
					annotationCodebase:        "my-app",
					annotationPipelineType:    "review",
					annotationGitBranch:       "main",
					annotationGitAuthor:       "alice",
					annotationGitChangeNumber: "7",
					annotationGitTargetBranch: "main",
				},
				map[string]string{
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

	labels := map[string]string{
		annotationCodebase:        "my-app",
		annotationPipelineType:    "review",
		annotationGitBranch:       "feature-x",
		annotationGitAuthor:       "bob",
		annotationGitChangeNumber: "99",
		annotationGitTargetBranch: "main",
	}
	annotations := map[string]string{
		annotationGitChangeURL: "https://github.com/org/repo/pull/99",
		annotationGitCommitSHA: "deadbeef",
	}
	item := &restapi.K8sList_200_Items_Item{
		Metadata: restapi.K8sList_200_Items_Metadata{
			Name:        "run-labels",
			Labels:      &labels,
			Annotations: &annotations,
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

func TestMapK8sPipelineRunInfo_Results(t *testing.T) {
	t.Parallel()

	status := map[string]any{
		"conditions": []any{map[string]any{"type": "Succeeded", "status": conditionStatusTrue}},
		"results": []any{
			map[string]any{"name": "VCS_TAG", "value": "build/1.0.0-SNAPSHOT.3"},
			map[string]any{"name": "IMAGES", "value": []any{"app:1.0.0", "app:latest"}},
			map[string]any{"value": "nameless results are dropped"},
			"not an object",
		},
	}
	item := &restapi.K8sList_200_Items_Item{
		Metadata: restapi.K8sList_200_Items_Metadata{Name: "run-results"},
		Status:   &status,
	}

	got := mapK8sPipelineRunInfo(item)
	assert.Equal(t, map[string]any{
		"VCS_TAG": "build/1.0.0-SNAPSHOT.3",
		"IMAGES":  []any{"app:1.0.0", "app:latest"},
	}, got.Results)

	withoutResults := map[string]any{"conditions": status["conditions"]}
	item.Status = &withoutResults
	assert.Nil(t, mapK8sPipelineRunInfo(item).Results, "a run without results must omit the field")
}

func TestParseResultAnnotations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want map[string]string
	}{
		{name: "empty input", raw: "", want: nil},
		{name: "invalid JSON", raw: "not json", want: nil},
		{
			name: "skips empty, null, and template placeholders",
			raw: `{
				"a": "kept",
				"b": "",
				"c": null,
				"d": "${tt.params.foo}",
				"e": 42
			}`,
			want: map[string]string{"a": "kept"},
		},
		{
			name: "realistic blob",
			raw: `{
				"app.edp.epam.com/git-author": "bob",
				"app.edp.epam.com/git-change-number": "99"
			}`,
			want: map[string]string{
				"app.edp.epam.com/git-author":        "bob",
				"app.edp.epam.com/git-change-number": "99",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, parseResultAnnotations(tt.raw))
		})
	}
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

	r := &tektonResult{
		UID:        "uuid-1",
		Name:       "results/ns/records/r1",
		CreateTime: recordCreate,
		UpdateTime: "2024-01-01T10:05:00Z",
		Summary: &tektonResultSummary{
			Record:    "results/ns/records/r1",
			Status:    "SUCCESS",
			StartTime: summaryStart,
			EndTime:   "2024-01-01T10:04:30Z",
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
		r := &tektonResult{
			UID:        "uuid-1",
			Name:       "results/ns/records/r1",
			CreateTime: "2024-01-01T10:05:00Z",
			UpdateTime: "2024-01-01T10:05:00Z",
		}
		got := mapPipelineRunInfo(r)
		assert.Equal(t, "2024-01-01T10:05:00Z", got.StartTime)
	})

	t.Run("summary without start_time", func(t *testing.T) {
		t.Parallel()
		r := &tektonResult{
			UID:        "uuid-1",
			Name:       "results/ns/records/r1",
			CreateTime: "2024-01-01T10:05:00Z",
			UpdateTime: "2024-01-01T10:05:00Z",
			Summary: &tektonResultSummary{
				Record: "results/ns/records/r1",
				Status: "SUCCESS",
			},
		}
		got := mapPipelineRunInfo(r)
		assert.Equal(t, "2024-01-01T10:05:00Z", got.StartTime)
	})
}

func TestMapPipelineRunInfo_DeploymentAndEnv(t *testing.T) {
	t.Parallel()

	r := &tektonResult{
		UID: "u1",
		Annotations: map[string]any{
			annotationObjectName:   "deploy-demo-dev-ab12",
			annotationPipelineType: "deploy",
			annotationCDPipeline:   "demo",
			annotationCDStage:      "demo-dev",
		},
	}

	got := mapPipelineRunInfo(r)
	assert.Equal(t, "demo", got.Deployment)
	assert.Equal(t, "dev", got.Env)
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

// TestMergePipelineRuns_TopIsMostRecent verifies that after merging live (K8s) and
// history (Tekton Results) runs, the top entry is the most recent by StartTime.
// Expansion (logs/tasks) is fetched by name for allRuns[0] in List, so this is the
// only ordering invariant the merge needs to guarantee.
func TestMergePipelineRuns_TopIsMostRecent(t *testing.T) {
	t.Parallel()

	live := []PipelineRunInfo{
		{Name: "run-live", StartTime: "2024-01-01T12:00:00Z"},
	}
	history := []PipelineRunInfo{
		{Name: "run-h1", StartTime: "2024-01-01T11:00:00Z"},
		{Name: "run-h2", StartTime: "2024-01-01T10:00:00Z"},
	}

	allRuns := mergePipelineRuns(live, history)

	require.Len(t, allRuns, 3)
	assert.Equal(t, "run-live", allRuns[0].Name, "most recent run must be first regardless of source")
	assert.Equal(t, "run-h1", allRuns[1].Name)
	assert.Equal(t, "run-h2", allRuns[2].Name)

	// No live runs — history's most recent stays at top.
	allRuns2 := mergePipelineRuns(nil, history)
	require.Len(t, allRuns2, 2)
	assert.Equal(t, "run-h1", allRuns2[0].Name)
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

func TestGet_CompletedRunFromResultsKeepsLiveResults(t *testing.T) {
	t.Parallel()

	const (
		resultUID = "11111111-1111-1111-1111-111111111111"
		recordUID = "22222222-2222-2222-2222-222222222222"
	)

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[{
				"metadata":{"name":"run-done"},
				"status":{"startTime":"2024-01-01T10:00:00Z","conditions":[
					{"type":"Succeeded","status":"True","reason":"Succeeded"}
				],"results":[{"name":"VCS_TAG","value":"build/1.0.0"}]}
			}]}`)
		},
		"/v1/pipeline-runs": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, `{"results":[{
				"name":"ns/results/`+resultUID+`","uid":"`+resultUID+`",
				"create_time":"2024-01-01T10:00:00Z","update_time":"2024-01-01T10:05:00Z",
				"annotations":{"object.metadata.name":"run-done"},
				"summary":{"record":"ns/results/`+resultUID+`/records/`+recordUID+`","status":"SUCCESS"}
			}]}`)
		},
		"/v1/pipeline-runs/" + resultUID + "/logs": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, `{"logs":"build log"}`)
		},
	})

	result, err := s.Get(context.Background(), "run-done", PipelineRunGetOptions{IncludeLogs: true})
	require.NoError(t, err)
	assert.Equal(t, "build log", result.Logs, "the expansion must come from Tekton Results")
	require.Len(t, result.PipelineRuns, 1)
	assert.Equal(t, map[string]any{"VCS_TAG": "build/1.0.0"}, result.PipelineRuns[0].Results,
		"Tekton Results summaries carry no pipeline results; the live run's must survive")
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

// liveRunJSON is a resources/list body holding run-x with the given
// Succeeded condition status and reason.
func liveRunJSON(condStatus, reason string) string {
	return `{"apiVersion":"v1","kind":"List","metadata":{},"items":[{
		"metadata":{"name":"run-x"},
		"status":{"startTime":"2024-01-01T10:00:00Z","conditions":[
			{"type":"Succeeded","status":"` + condStatus + `","reason":"` + reason + `"}
		],"results":[{"name":"VCS_TAG","value":"build/1.0.0"}]}
	}]}`
}

func TestWait_PollsUntilTheRunFinishes(t *testing.T) {
	t.Parallel()

	var polls atomic.Int32

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			if polls.Add(1) < 3 {
				writeJSONOK(w, liveRunJSON("Unknown", "Running"))
				return
			}

			writeJSONOK(w, liveRunJSON("True", "Succeeded"))
		},
		"/v1/pipeline-runs": func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("Tekton Results must not be called without an expansion")
		},
	})

	result, err := s.Wait(context.Background(), "run-x", PipelineRunGetOptions{}, time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, int32(3), polls.Load(), "Wait must poll until the run leaves Running")
	assert.Equal(t, StatusSucceeded, result.PipelineRuns[0].Status)
	assert.Equal(t, "build/1.0.0", result.PipelineRuns[0].Results["VCS_TAG"])
}

func TestWait_FetchesTheExpansionOnceTheRunFinishes(t *testing.T) {
	t.Parallel()

	const (
		resultUID = "33333333-3333-3333-3333-333333333333"
		recordUID = "44444444-4444-4444-4444-444444444444"
	)

	var polls, resultsCalls atomic.Int32

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			if polls.Add(1) < 2 {
				writeJSONOK(w, liveRunJSON("Unknown", "Running"))
				return
			}

			writeJSONOK(w, liveRunJSON("False", "Failed"))
		},
		"/v1/pipeline-runs": func(w http.ResponseWriter, _ *http.Request) {
			resultsCalls.Add(1)
			writeJSONOK(w, `{"results":[{
				"name":"ns/results/`+resultUID+`","uid":"`+resultUID+`",
				"create_time":"2024-01-01T10:00:00Z","update_time":"2024-01-01T10:05:00Z",
				"annotations":{"object.metadata.name":"run-x"},
				"summary":{"record":"ns/results/`+resultUID+`/records/`+recordUID+`","status":"FAILURE"}
			}]}`)
		},
		"/v1/pipeline-runs/" + resultUID + "/logs": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, `{"logs":"step failed"}`)
		},
	})

	result, err := s.Wait(context.Background(), "run-x", PipelineRunGetOptions{IncludeLogs: true}, time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, int32(1), resultsCalls.Load(), "the expansion is fetched once, after the run finished")
	assert.Equal(t, StatusFailed, result.PipelineRuns[0].Status)
	assert.Equal(t, "step failed", result.Logs)
}

func TestWait_StopsWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, liveRunJSON("Unknown", "Running"))
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := s.Wait(ctx, "run-x", PipelineRunGetOptions{}, time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestWait_UnknownRunFailsWithoutPolling(t *testing.T) {
	t.Parallel()

	var polls atomic.Int32

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			polls.Add(1)
			writeJSONOK(w, `{"apiVersion":"v1","kind":"List","items":[],"metadata":{}}`)
		},
		"/v1/pipeline-runs": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, `{"results":[]}`)
		},
	})

	_, err := s.Wait(context.Background(), "run-ghost", PipelineRunGetOptions{}, time.Millisecond)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, int32(1), polls.Load())
}

func TestList_DeploymentAndEnvFilterBothSources(t *testing.T) {
	t.Parallel()

	var filter atomic.Value

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONOK(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[
				{"metadata":{"name":"deploy-demo-dev-live1","labels":{
					"app.edp.epam.com/pipelinetype":"deploy",
					"app.edp.epam.com/cdpipeline":"demo","app.edp.epam.com/cdstage":"demo-dev"}},
				 "status":{"startTime":"2024-01-01T10:10:00Z","conditions":[{"type":"Succeeded","status":"Unknown"}]}},
				{"metadata":{"name":"deploy-demo-qa-live2","labels":{
					"app.edp.epam.com/pipelinetype":"deploy",
					"app.edp.epam.com/cdpipeline":"demo","app.edp.epam.com/cdstage":"demo-qa"}},
				 "status":{"startTime":"2024-01-01T10:11:00Z","conditions":[{"type":"Succeeded","status":"Unknown"}]}}
			]}`)
		},
		"/v1/pipeline-runs": func(w http.ResponseWriter, r *http.Request) {
			filter.Store(r.URL.Query().Get("filter"))
			writeJSONOK(w, `{"results":[{
				"name":"ns/results/r1","uid":"r1",
				"create_time":"2024-01-01T09:00:00Z","update_time":"2024-01-01T09:05:00Z",
				"annotations":{"object.metadata.name":"deploy-demo-dev-old1",
					"app.edp.epam.com/pipelinetype":"deploy",
					"app.edp.epam.com/cdpipeline":"demo","app.edp.epam.com/cdstage":"demo-dev"},
				"summary":{"record":"ns/results/r1/records/r1","status":"FAILURE"}
			}]}`)
		},
	})

	result, err := s.List(context.Background(), PipelineRunListOptions{
		Filter: PipelineRunFilter{Type: "deploy", Deployment: "demo", Env: "dev"},
	})
	require.NoError(t, err)

	assert.Contains(t, filter.Load(), `annotations["app.edp.epam.com/cdstage"] == "demo-dev"`,
		"Tekton Results must be asked for the stage, not all deploy runs")

	names := make([]string, 0, len(result.PipelineRuns))
	for _, r := range result.PipelineRuns {
		names = append(names, r.Name)
		assert.Equal(t, "demo", r.Deployment)
		assert.Equal(t, "dev", r.Env)
	}
	assert.Equal(t, []string{"deploy-demo-dev-live1", "deploy-demo-dev-old1"}, names,
		"the live run of demo/qa is filtered out, the history run of demo/dev is kept")
}

func TestList_LiveRunsSelectedByStageLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter PipelineRunFilter
		want   map[string]string
	}{
		{
			name:   "deployment and env",
			filter: PipelineRunFilter{Deployment: "demo", Env: "dev"},
			want:   map[string]string{annotationCDPipeline: "demo", annotationCDStage: "demo-dev"},
		},
		{
			name:   "deployment only",
			filter: PipelineRunFilter{Deployment: "demo"},
			want:   map[string]string{annotationCDPipeline: "demo"},
		},
		{
			name:   "neither",
			filter: PipelineRunFilter{Project: "my-app", Type: "build"},
		},
		{
			name:   "stage over the label value limit",
			filter: PipelineRunFilter{Deployment: "demo", Env: strings.Repeat("e", 60)},
			want:   map[string]string{annotationCDPipeline: "demo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var labels atomic.Value

			s := newMockPortal(t, pathHandlers{
				"/v1/resources/list": func(w http.ResponseWriter, r *http.Request) {
					var body struct {
						Labels map[string]string `json:"labels"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}

					labels.Store(body.Labels)
					writeJSONOK(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`)
				},
				"/v1/pipeline-runs": func(w http.ResponseWriter, _ *http.Request) {
					writeJSONOK(w, `{"results":[]}`)
				},
			})

			_, err := s.List(context.Background(), PipelineRunListOptions{Filter: tt.filter})
			require.NoError(t, err)
			assert.Equal(t, tt.want, labels.Load())
		})
	}
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

// End-to-end guard that enriched columns survive the full HTTP→client→mapper decode path.
func TestList_EnrichedFieldsFromWireBytes(t *testing.T) {
	t.Parallel()

	// Raw body mirroring the portal's /v1/resources/list shape for a review
	// PipelineRun: EDP labels on metadata.labels, git URLs on metadata.annotations.
	k8sBody := `{
		"apiVersion": "v1",
		"kind": "List",
		"metadata": {},
		"items": [{
			"apiVersion": "tekton.dev/v1",
			"kind": "PipelineRun",
			"metadata": {
				"name": "review-my-app-main-abc12",
				"namespace": "test-ns",
				"creationTimestamp": "2024-01-01T09:59:00Z",
				"labels": {
					"app.edp.epam.com/codebase":          "my-app",
					"app.edp.epam.com/pipelinetype":      "review",
					"app.edp.epam.com/git-branch":        "feature-x",
					"app.edp.epam.com/git-author":        "alice",
					"app.edp.epam.com/git-change-number": "42",
					"app.edp.epam.com/git-target-branch": "main"
				},
				"annotations": {
					"app.edp.epam.com/git-change-url": "https://github.com/org/my-app/pull/42",
					"app.edp.epam.com/git-commit-sha": "deadbeefcafef00d"
				}
			},
			"spec": {"pipelineRef": {"name": "review-pipeline"}},
			"status": {
				"startTime": "2024-01-01T10:00:00Z",
				"completionTime": "2024-01-01T10:03:00Z",
				"conditions": [{"type": "Succeeded", "status": "True"}]
			}
		}]
	}`

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) { writeJSONOK(w, k8sBody) },
		"/v1/pipeline-runs":  func(w http.ResponseWriter, _ *http.Request) { writeJSONOK(w, `{"results":[]}`) },
	})

	result, err := s.List(context.Background(), PipelineRunListOptions{})
	require.NoError(t, err)
	require.Len(t, result.PipelineRuns, 1)

	got := result.PipelineRuns[0]
	assert.Equal(t, "review-my-app-main-abc12", got.Name)
	assert.Equal(t, StatusSucceeded, got.Status)
	assert.Equal(t, "review-pipeline", got.Pipeline)
	assert.Equal(t, "my-app", got.Project)
	assert.Equal(t, "review", got.Type)
	assert.Equal(t, "feature-x", got.Branch)
	assert.Equal(t, "alice", got.Author)
	assert.Equal(t, "42", got.PRNumber)
	assert.Equal(t, "main", got.TargetBranch)
	assert.Equal(t, "https://github.com/org/my-app/pull/42", got.PRURL)
	assert.Equal(t, "deadbeefcafef00d", got.CommitSHA)
	assert.Equal(t, "2024-01-01T10:00:00Z", got.StartTime)
	assert.Equal(t, "3m 0s", got.Duration)
}

// End-to-end guard for the resultAnnotations JSON blob path: git metadata only
// lives inside `results.tekton.dev/resultAnnotations`, no top-level labels.
// Mirrors what modern EDP pipeline templates emit for in-flight runs.
func TestList_ResultAnnotationsFromWireBytes(t *testing.T) {
	t.Parallel()

	k8sBody := `{
		"apiVersion": "v1",
		"kind": "List",
		"metadata": {},
		"items": [{
			"apiVersion": "tekton.dev/v1",
			"kind": "PipelineRun",
			"metadata": {
				"name": "review-my-app-main-8j8jt",
				"namespace": "test-ns",
				"labels": {
					"app.edp.epam.com/codebase":     "my-app",
					"app.edp.epam.com/pipelinetype": "review"
				},
				"annotations": {
					"results.tekton.dev/resultAnnotations": "{\"app.edp.epam.com/git-branch\":\"TICKET-1234\",\"app.edp.epam.com/git-author\":\"alice\",\"app.edp.epam.com/git-change-number\":\"3112\",\"app.edp.epam.com/git-change-url\":\"https://gitlab/ns/repo/-/merge_requests/3112\",\"app.edp.epam.com/git-commit-sha\":\"abc123\",\"app.edp.epam.com/git-target-branch\":\"main\"}"
				}
			},
			"status": {
				"startTime": "2024-01-01T10:00:00Z",
				"conditions": [{"type": "Succeeded", "status": "Unknown"}]
			}
		}]
	}`

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) { writeJSONOK(w, k8sBody) },
		"/v1/pipeline-runs": func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("Tekton Results must not be called when filter=running")
		},
	})

	result, err := s.List(context.Background(), PipelineRunListOptions{
		Filter: PipelineRunFilter{Status: strings.ToLower(StatusRunning)},
	})
	require.NoError(t, err)
	require.Len(t, result.PipelineRuns, 1)

	got := result.PipelineRuns[0]
	assert.Equal(t, "my-app", got.Project)
	assert.Equal(t, "review", got.Type)
	assert.Equal(t, "TICKET-1234", got.Branch)
	assert.Equal(t, "alice", got.Author)
	assert.Equal(t, "3112", got.PRNumber)
	assert.Equal(t, "https://gitlab/ns/repo/-/merge_requests/3112", got.PRURL)
	assert.Equal(t, "abc123", got.CommitSHA)
	assert.Equal(t, "main", got.TargetBranch)
}

func TestList_CreationTimestampFallbackFromWireBytes(t *testing.T) {
	t.Parallel()

	k8sBody := `{
		"apiVersion": "v1",
		"kind": "List",
		"metadata": {},
		"items": [{
			"metadata": {
				"name": "run-fresh",
				"creationTimestamp": "2024-01-01T09:58:00Z"
			}
		}]
	}`

	s := newMockPortal(t, pathHandlers{
		"/v1/resources/list": func(w http.ResponseWriter, _ *http.Request) { writeJSONOK(w, k8sBody) },
		"/v1/pipeline-runs":  func(w http.ResponseWriter, _ *http.Request) { writeJSONOK(w, `{"results":[]}`) },
	})

	result, err := s.List(context.Background(), PipelineRunListOptions{})
	require.NoError(t, err)
	require.Len(t, result.PipelineRuns, 1)
	assert.Equal(t, "2024-01-01T09:58:00Z", result.PipelineRuns[0].StartTime)
}
