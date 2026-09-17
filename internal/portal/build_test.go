package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const (
	buildCodebase = "my-app"
	buildBranch   = "feat/x"
)

func newBuildService(t *testing.T, handler http.HandlerFunc) (*ProjectBuildService, func()) {
	t.Helper()

	client, closer := newTestClient(t, handler)

	return NewProjectBuildService(client, "edp"), closer
}

func errorBody(code, reason string) string {
	return fmt.Sprintf(`{"error":{"code":%q,"reason":%q,"message":%q}}`, code, reason, http.StatusText(statusForCode(code)))
}

func statusForCode(code string) int {
	switch code {
	case "BAD_REQUEST":
		return http.StatusBadRequest
	case "NOT_FOUND":
		return http.StatusNotFound
	case "CONFLICT":
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func TestCheckBuildResponse_Reasons(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		status   int
		body     string
		in       BuildInput
		sentinel error
		wantMsg  string
	}{
		{
			name:     "managed param override names the offending param",
			status:   http.StatusBadRequest,
			body:     errorBody("BAD_REQUEST", "managed_param_override"),
			in:       BuildInput{Codebase: buildCodebase, Params: map[string]string{"git-source-url": "x"}},
			sentinel: ErrManagedParam,
			wantMsg:  "parameter 'git-source-url' is set from the project and branch by 'project build'; use 'krci pipelinerun start' for raw overrides",
		},
		{
			name:     "managed param override falls back when the server set is wider",
			status:   http.StatusBadRequest,
			body:     errorBody("BAD_REQUEST", "managed_param_override"),
			in:       BuildInput{Codebase: buildCodebase, Params: map[string]string{"SOME_NEW_PARAM": "x"}},
			sentinel: ErrManagedParam,
			wantMsg:  "a parameter is set from the project and branch by 'project build'; use 'krci pipelinerun start' for raw overrides",
		},
		{
			name:     "codebase not found",
			status:   http.StatusNotFound,
			body:     errorBody("NOT_FOUND", "codebase_not_found"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrProjectNotFound,
			wantMsg:  "project 'my-app' not found",
		},
		{
			name:     "gitlab ci not supported",
			status:   http.StatusBadRequest,
			body:     errorBody("BAD_REQUEST", "gitlab_ci_not_supported"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrGitLabCIUnsupported,
			wantMsg:  "project 'my-app' builds via GitLab CI; 'krci project build' does not support it yet, trigger it from the portal",
		},
		{
			name:     "branch not specified",
			status:   http.StatusBadRequest,
			body:     errorBody("BAD_REQUEST", "branch_not_specified"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrPlatformReject,
			wantMsg:  "project 'my-app' has no default branch; pass --branch",
		},
		{
			name:     "codebase branch not found with --branch",
			status:   http.StatusNotFound,
			body:     errorBody("NOT_FOUND", "codebase_branch_not_found"),
			in:       BuildInput{Codebase: buildCodebase, Branch: buildBranch},
			sentinel: ErrBranchNotFound,
			wantMsg:  "branch 'feat/x' of project 'my-app' not found",
		},
		{
			name:     "codebase branch not found without --branch",
			status:   http.StatusNotFound,
			body:     errorBody("NOT_FOUND", "codebase_branch_not_found"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrBranchNotFound,
			wantMsg:  "the default branch of project 'my-app' not found",
		},
		{
			name:     "codebase branch ambiguous with --branch",
			status:   http.StatusConflict,
			body:     errorBody("CONFLICT", "codebase_branch_ambiguous"),
			in:       BuildInput{Codebase: buildCodebase, Branch: buildBranch},
			sentinel: ErrPlatformReject,
			wantMsg:  "branch 'feat/x' of project 'my-app' matches more than one CodebaseBranch",
		},
		{
			name:     "codebase branch ambiguous without --branch",
			status:   http.StatusConflict,
			body:     errorBody("CONFLICT", "codebase_branch_ambiguous"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrPlatformReject,
			wantMsg:  "the default branch of project 'my-app' matches more than one CodebaseBranch",
		},
		{
			name:     "build pipeline not configured with --branch",
			status:   http.StatusBadRequest,
			body:     errorBody("BAD_REQUEST", "build_pipeline_not_configured"),
			in:       BuildInput{Codebase: buildCodebase, Branch: buildBranch},
			sentinel: ErrPlatformReject,
			wantMsg:  "branch 'feat/x' of project 'my-app' has no build pipeline configured",
		},
		{
			name:     "build pipeline not configured without --branch",
			status:   http.StatusBadRequest,
			body:     errorBody("BAD_REQUEST", "build_pipeline_not_configured"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrPlatformReject,
			wantMsg:  "the default branch of project 'my-app' has no build pipeline configured",
		},
		{
			name:     "git server not found",
			status:   http.StatusNotFound,
			body:     errorBody("NOT_FOUND", "git_server_not_found"),
			in:       BuildInput{Codebase: buildCodebase, Branch: buildBranch},
			sentinel: ErrPlatformReject,
			wantMsg:  "project 'my-app' references a git server that does not exist",
		},
		{
			name:     "trigger template not found",
			status:   http.StatusNotFound,
			body:     errorBody("NOT_FOUND", "trigger_template_not_found"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrTriggerTemplateNotFound,
			wantMsg:  "no usable build TriggerTemplate for project 'my-app' (platform misconfiguration)",
		},
		{
			name:     "build template misconfigured",
			status:   http.StatusBadRequest,
			body:     errorBody("BAD_REQUEST", "build_template_misconfigured"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrPlatformReject,
			wantMsg:  "no usable build TriggerTemplate for project 'my-app' (platform misconfiguration)",
		},
		{
			name:     "codebase branch not ready with --branch",
			status:   http.StatusConflict,
			body:     errorBody("CONFLICT", "codebase_branch_not_ready"),
			in:       BuildInput{Codebase: buildCodebase, Branch: buildBranch},
			sentinel: ErrBranchNotReady,
			wantMsg:  "branch 'feat/x' of project 'my-app' is not ready (status must be 'created'); check: krci project get my-app",
		},
		{
			name:     "codebase branch not ready without --branch",
			status:   http.StatusConflict,
			body:     errorBody("CONFLICT", "codebase_branch_not_ready"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrBranchNotReady,
			wantMsg:  "the default branch of project 'my-app' is not ready (status must be 'created'); check: krci project get my-app",
		},
		{
			name:     "build in progress with --branch",
			status:   http.StatusConflict,
			body:     errorBody("CONFLICT", "build_in_progress"),
			in:       BuildInput{Codebase: buildCodebase, Branch: buildBranch},
			sentinel: ErrBuildInProgress,
			wantMsg:  "a build is already running for branch 'feat/x' of project 'my-app'; check: krci pipelinerun list --project my-app --status running",
		},
		{
			name:     "build in progress without --branch",
			status:   http.StatusConflict,
			body:     errorBody("CONFLICT", "build_in_progress"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrBuildInProgress,
			wantMsg:  "a build is already running for the default branch of project 'my-app'; check: krci pipelinerun list --project my-app --status running",
		},
		{
			name:     "list truncated",
			status:   http.StatusInternalServerError,
			body:     errorBody("INTERNAL_SERVER_ERROR", "list_truncated"),
			in:       BuildInput{Codebase: buildCodebase},
			sentinel: ErrUpstreamUnavailable,
			wantMsg:  "portal could not safely evaluate the build state for project 'my-app' (resource list truncated); retry or contact an operator",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkBuildResponse(tc.status, []byte(tc.body), tc.in)
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("want sentinel %v, got %v", tc.sentinel, err)
			}

			if err.Error() != tc.wantMsg {
				t.Fatalf("message:\n got  %q\n want %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

// TestCheckBuildResponse_NotFoundSentinelsWrapErrNotFound checks that every
// not-found reason stays matchable by generic not-found callers.
func TestCheckBuildResponse_NotFoundSentinelsWrapErrNotFound(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{"codebase_not_found", "codebase_branch_not_found", "trigger_template_not_found"} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()

			err := checkBuildResponse(http.StatusNotFound, []byte(errorBody("NOT_FOUND", reason)),
				BuildInput{Codebase: buildCodebase})
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("want ErrNotFound match, got %v", err)
			}
		})
	}
}

func TestCheckBuildResponse_Fallbacks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		status   int
		body     string
		sentinel error
		contains string
	}{
		{
			name:     "unknown reason on 404",
			status:   http.StatusNotFound,
			body:     errorBody("NOT_FOUND", "brand_new_reason"),
			sentinel: ErrProjectNotFound,
			contains: "project 'my-app' not found",
		},
		{
			name:     "plain text 404",
			status:   http.StatusNotFound,
			body:     "plain text 404 from a misbehaving proxy",
			sentinel: ErrProjectNotFound,
			contains: "project 'my-app' not found",
		},
		{
			name:     "route missing on an older portal",
			status:   http.StatusNotFound,
			body:     fastifyRouteMissing,
			sentinel: ErrPortalUnsupported,
			contains: "upgrade the portal",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkBuildResponse(tc.status, []byte(tc.body), BuildInput{Codebase: buildCodebase})
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("want sentinel %v, got %v", tc.sentinel, err)
			}

			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("message %q missing %q", err.Error(), tc.contains)
			}
		})
	}
}

func TestCheckBuildResponse_AuthStatuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status   int
		sentinel error
	}{
		{http.StatusOK, nil},
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusForbidden, ErrPermissionDenied},
	}

	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			t.Parallel()

			err := checkBuildResponse(tc.status, []byte("portal exploded"), BuildInput{Codebase: buildCodebase})
			if tc.sentinel == nil {
				if err != nil {
					t.Fatalf("want nil error, got %v", err)
				}

				return
			}

			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("want sentinel %v, got %v", tc.sentinel, err)
			}
		})
	}
}

func TestBuildService_Build_RequestBody(t *testing.T) {
	t.Parallel()

	type request struct {
		Namespace string            `json:"namespace"`
		Codebase  string            `json:"codebase"`
		Branch    *string           `json:"branch"`
		Params    map[string]string `json:"params"`
		DryRun    *bool             `json:"dryRun"`
	}

	cases := []struct {
		name   string
		in     BuildInput
		assert func(t *testing.T, got request)
	}{
		{
			name: "branch omitted",
			in:   BuildInput{Codebase: buildCodebase},
			assert: func(t *testing.T, got request) {
				t.Helper()

				if got.Branch != nil {
					t.Errorf("branch must be omitted, got %q", *got.Branch)
				}

				if got.Params != nil {
					t.Errorf("params must be omitted when empty, got %v", got.Params)
				}

				if got.DryRun != nil {
					t.Errorf("dryRun must be omitted when false, got %v", *got.DryRun)
				}
			},
		},
		{
			name: "branch, params and dry-run supplied",
			in: BuildInput{
				Codebase: buildCodebase,
				Branch:   buildBranch,
				Params:   map[string]string{"COMMIT_MESSAGE": "hello"},
				DryRun:   true,
			},
			assert: func(t *testing.T, got request) {
				t.Helper()

				if got.Branch == nil || *got.Branch != buildBranch {
					t.Errorf("branch not forwarded: %v", got.Branch)
				}

				if got.Params["COMMIT_MESSAGE"] != "hello" {
					t.Errorf("params not forwarded: %v", got.Params)
				}

				if got.DryRun == nil || !*got.DryRun {
					t.Errorf("dryRun not forwarded: %v", got.DryRun)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/rest/v1/pipelineruns/build" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}

				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}

				raw, _ := io.ReadAll(r.Body)

				var got request
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Fatalf("unmarshal request: %v", err)
				}

				if got.Namespace != "edp" || got.Codebase != buildCodebase {
					t.Errorf("identity fields not forwarded: %+v", got)
				}

				tc.assert(t, got)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"kind":"dryRun","manifest":{"kind":"PipelineRun"}}`))
			}

			svc, closer := newBuildService(t, handler)
			defer closer()

			if _, err := svc.Build(context.Background(), tc.in); err != nil {
				t.Fatalf("Build: %v", err)
			}
		})
	}
}

func TestBuildService_Build_DecodesCreatedRow(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"kind": "created",
			"row": {
				"name": "build-my-app-main-x9k2p",
				"status": "Pending",
				"project": "my-app",
				"pr": "",
				"author": "",
				"type": "build",
				"started": "",
				"duration": ""
			}
		}`))
	}

	svc, closer := newBuildService(t, handler)
	defer closer()

	got, err := svc.Build(context.Background(), BuildInput{Codebase: buildCodebase})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got.Name != "build-my-app-main-x9k2p" || got.Type != "build" || got.Project != buildCodebase {
		t.Fatalf("unexpected row: %+v", got)
	}

	if len(got.DryRunManifest) != 0 {
		t.Fatalf("dry-run manifest must be empty on the live path: %v", got.DryRunManifest)
	}
}

func TestBuildService_Build_DecodesDryRunManifest(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		payload := map[string]any{
			"kind": "dryRun",
			"manifest": map[string]any{
				"apiVersion": "tekton.dev/v1",
				"kind":       "PipelineRun",
				"metadata":   map[string]any{"generateName": "build-my-app-main-"},
			},
		}

		_ = json.NewEncoder(w).Encode(payload)
	}

	svc, closer := newBuildService(t, handler)
	defer closer()

	got, err := svc.Build(context.Background(), BuildInput{Codebase: buildCodebase, DryRun: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	metadata, ok := got.DryRunManifest["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata not parsed as object: %T", got.DryRunManifest["metadata"])
	}

	if metadata["generateName"] != "build-my-app-main-" {
		t.Fatalf("generateName mismatch: %v", metadata["generateName"])
	}

	if got.Name != "" {
		t.Fatalf("name must be empty on dry-run: %q", got.Name)
	}
}

func TestBuildService_Build_MapsReason(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(errorBody("CONFLICT", "build_in_progress")))
	}

	svc, closer := newBuildService(t, handler)
	defer closer()

	_, err := svc.Build(context.Background(), BuildInput{Codebase: buildCodebase, Branch: "main"})
	if !errors.Is(err, ErrBuildInProgress) {
		t.Fatalf("want ErrBuildInProgress, got %v", err)
	}
}
