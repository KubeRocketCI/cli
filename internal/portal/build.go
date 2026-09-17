package portal

import (
	"context"
	"fmt"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

// BuildInput is the `project build` request: the portal resolves the branch,
// build pipeline, params, labels and service account from the project itself.
type BuildInput struct {
	Codebase string            // Codebase (project) name
	Branch   string            // "" → server resolves codebase.spec.defaultBranch
	Params   map[string]string // non-managed parameter overrides (may be nil)
	DryRun   bool              // true → render manifest without create
}

// MaxBranchLength is the portal's bound on BuildInput.Branch.
const MaxBranchLength = 253

// BuildManagedParams are the PipelineRun params the portal derives from the
// project and its branch; `--param` may not override them. Mirrors
// `x-krci-managed-params` in the vendored OpenAPI document.
var BuildManagedParams = []string{
	"git-source-url",
	"git-source-revision",
	"targetBranch",
	"CODEBASE_NAME",
	"CODEBASEBRANCH_NAME",
	"gitfullrepositoryname",
	"changeNumber",
	"patchsetNumber",
}

// Stable machine-readable reasons the portal reports under `error.reason` for
// `POST /rest/v1/pipelineruns/build`; trigger_template_not_found is shared with
// start.
const (
	reasonManagedParamOverride       = "managed_param_override"
	reasonCodebaseNotFound           = "codebase_not_found"
	reasonGitLabCINotSupported       = "gitlab_ci_not_supported"
	reasonBranchNotSpecified         = "branch_not_specified"
	reasonCodebaseBranchNotFound     = "codebase_branch_not_found"
	reasonCodebaseBranchAmbiguous    = "codebase_branch_ambiguous"
	reasonBuildPipelineNotConfigured = "build_pipeline_not_configured"
	reasonGitServerNotFound          = "git_server_not_found"
	reasonBuildTemplateMisconfigured = "build_template_misconfigured"
	reasonCodebaseBranchNotReady     = "codebase_branch_not_ready"
	reasonBuildInProgress            = "build_in_progress"
	reasonListTruncated              = "list_truncated"
)

const triggerTemplateFormat = "no usable build TriggerTemplate for project '%[1]s' (platform misconfiguration)"

// buildReasons renders with the args of buildReasonArgs: %[1]s = project,
// %[2]s = branch phrase, %[3]s = managed-param phrase. The portal never
// echoes resource names; every message is built from the request.
var buildReasons = reasonTable{
	reasonManagedParamOverride: {
		sentinel: ErrManagedParam,
		format:   "%[3]s",
	},
	reasonCodebaseNotFound: {
		sentinel: ErrProjectNotFound,
		format:   "project '%[1]s' not found",
	},
	reasonGitLabCINotSupported: {
		sentinel: ErrGitLabCIUnsupported,
		format: "project '%[1]s' builds via GitLab CI; " +
			"'krci project build' does not support it yet, trigger it from the portal",
	},
	reasonBranchNotSpecified: {
		sentinel: ErrPlatformReject,
		format:   "project '%[1]s' has no default branch; pass --branch",
	},
	reasonCodebaseBranchNotFound: {
		sentinel: ErrBranchNotFound,
		format:   "%[2]s of project '%[1]s' not found",
	},
	reasonCodebaseBranchAmbiguous: {
		sentinel: ErrPlatformReject,
		format:   "%[2]s of project '%[1]s' matches more than one CodebaseBranch",
	},
	reasonBuildPipelineNotConfigured: {
		sentinel: ErrPlatformReject,
		format:   "%[2]s of project '%[1]s' has no build pipeline configured",
	},
	reasonGitServerNotFound: {
		sentinel: ErrPlatformReject,
		format:   "project '%[1]s' references a git server that does not exist",
	},
	reasonTriggerTemplateNotFound: {
		sentinel: ErrTriggerTemplateNotFound,
		format:   triggerTemplateFormat,
	},
	reasonBuildTemplateMisconfigured: {
		sentinel: ErrPlatformReject,
		format:   triggerTemplateFormat,
	},
	reasonCodebaseBranchNotReady: {
		sentinel: ErrBranchNotReady,
		format: "%[2]s of project '%[1]s' is not ready " +
			"(status must be 'created'); check: krci project get %[1]s",
	},
	reasonBuildInProgress: {
		sentinel: ErrBuildInProgress,
		format: "a build is already running for %[2]s of project '%[1]s'; " +
			"check: krci pipelinerun list --project %[1]s --status running",
	},
	reasonListTruncated: {
		sentinel: ErrUpstreamUnavailable,
		format: "portal could not safely evaluate the build state " +
			"for project '%[1]s' (resource list truncated); retry or contact an operator",
	},
}

// buildReasonArgs are the positional format args every buildReasons entry
// renders with.
func buildReasonArgs(in BuildInput) []any {
	return []any{in.Codebase, branchPhrase(in), managedParamPhrase(in.Params)}
}

// branchPhrase names the branch the request targets. An omitted --branch
// renders as "the default branch": the error body carries only a reason tag,
// not the branch the server resolved.
func branchPhrase(in BuildInput) string {
	if in.Branch == "" {
		return "the default branch"
	}

	return fmt.Sprintf("branch '%s'", in.Branch)
}

const managedParamHint = "is set from the project and branch by 'project build'; " +
	"use 'krci pipelinerun start' for raw overrides"

// firstManagedParam returns the first BuildManagedParams entry present in params.
func firstManagedParam(params map[string]string) (string, bool) {
	for _, p := range BuildManagedParams {
		if _, ok := params[p]; ok {
			return p, true
		}
	}

	return "", false
}

// managedParamPhrase names the managed param the request carries. The
// generic form only fires when the server's managed set is wider than
// BuildManagedParams.
func managedParamPhrase(params map[string]string) string {
	if p, ok := firstManagedParam(params); ok {
		return fmt.Sprintf("parameter '%s' %s", p, managedParamHint)
	}

	return "a parameter " + managedParamHint
}

// ValidateBuildParams rejects overrides of the params the portal derives from
// the project and its branch, before any network call. The server enforces the
// same set and reports reason=managed_param_override.
func ValidateBuildParams(params map[string]string) error {
	if _, ok := firstManagedParam(params); !ok {
		return nil
	}

	return newRichErr(managedParamPhrase(params), ErrManagedParam)
}

// ProjectBuildService starts the build pipeline of a project branch, the way
// the portal's Build button does.
type ProjectBuildService struct {
	client    *restapi.ClientWithResponses
	namespace string
}

func NewProjectBuildService(client *restapi.ClientWithResponses, namespace string) *ProjectBuildService {
	return &ProjectBuildService{client: client, namespace: namespace}
}

// Build calls `POST /rest/v1/pipelineruns/build`. The 200 body is the same
// discriminated union `pipelinerun start` returns, so decoding is shared.
func (s *ProjectBuildService) Build(ctx context.Context, in BuildInput) (*StartResult, error) {
	body := restapi.PipelineRunBuildJSONRequestBody{
		Namespace: s.namespace,
		Codebase:  in.Codebase,
	}

	if in.Branch != "" {
		body.Branch = ptr.To(in.Branch)
	}

	if len(in.Params) > 0 {
		body.Params = ptr.To(in.Params)
	}

	if in.DryRun {
		body.DryRun = ptr.To(true)
	}

	resp, err := s.client.PipelineRunBuildWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("calling project build: %w", err)
	}

	if err := checkBuildResponse(resp.StatusCode(), resp.Body, in); err != nil {
		return nil, err
	}

	return decodeStartBody(resp.Body)
}

// checkBuildResponse maps a build response. A 404 without a reason tag (e.g.
// a plain-text 404 from a proxy) maps to ErrProjectNotFound.
func checkBuildResponse(statusCode int, body []byte, in BuildInput) error {
	return checkReasonedResponse(statusCode, body, buildReasons, buildReasons[reasonCodebaseNotFound],
		buildReasonArgs(in)...)
}
