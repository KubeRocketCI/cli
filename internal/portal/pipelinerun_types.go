package portal

import (
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

// Tekton Result annotation keys.
const (
	annotationCodebase          = "app.edp.epam.com/codebase"
	annotationPipelineType      = "app.edp.epam.com/pipelinetype"
	annotationGitChangeNumber   = "app.edp.epam.com/git-change-number"
	annotationGitChangeURL      = "app.edp.epam.com/git-change-url"
	annotationGitAuthor         = "app.edp.epam.com/git-author"
	annotationGitBranch         = "app.edp.epam.com/git-branch"
	annotationGitTargetBranch   = "app.edp.epam.com/git-target-branch"
	annotationGitCommitSHA      = "app.edp.epam.com/git-commit-sha"
	annotationPipeline          = "tekton.dev/pipeline"
	annotationObjectName        = "object.metadata.name"
	annotationResultAnnotations = "results.tekton.dev/resultAnnotations"
)

const (
	resultStatusSuccess   = "SUCCESS"
	resultStatusFailure   = "FAILURE"
	resultStatusTimeout   = "TIMEOUT"
	resultStatusCancelled = "CANCELLED"
	resultStatusUnknown   = "UNKNOWN"
)

const (
	StatusSucceeded = "Succeeded"
	StatusFailed    = "Failed"
	StatusTimeout   = "Timeout"
	StatusCancelled = "Cancelled"
	StatusRunning   = "Running"
)

// K8s condition status values.
const (
	conditionStatusTrue  = "True"
	conditionStatusFalse = "False"
)

func IsFailureStatus(status string) bool {
	return status == StatusFailed || status == StatusTimeout
}

var statusDisplay = map[string]string{
	resultStatusSuccess:   StatusSucceeded,
	resultStatusFailure:   StatusFailed,
	resultStatusTimeout:   StatusTimeout,
	resultStatusCancelled: StatusCancelled,
	resultStatusUnknown:   StatusRunning,
}

// statusMapping pairs the display label and Tekton Results CEL numeric value
// for each user-facing status filter keyword.
type statusMapping struct {
	display string
	cel     string
}

var statusMappings = map[string]statusMapping{
	"succeeded": {display: StatusSucceeded, cel: "1"},
	"failed":    {display: StatusFailed, cel: "2"},
	"timeout":   {display: StatusTimeout, cel: "3"},
	"cancelled": {display: StatusCancelled, cel: "4"},
	"running":   {display: StatusRunning, cel: "0"},
}

func displayStatus(resultStatus string) string {
	if s, ok := statusDisplay[resultStatus]; ok {
		return s
	}

	return resultStatus
}

const conditionSucceeded = "Succeeded"

// tektonResult is a local projection of a Tekton Results record. The portal
// inlines these fields into the response schema rather than exposing a named
// component, so we copy them across the API boundary and keep the service code
// and tests free of the generated anonymous struct.
type tektonResult struct {
	UID         string
	Name        string
	CreateTime  string
	UpdateTime  string
	Annotations map[string]any
	Summary     *tektonResultSummary
}

type tektonResultSummary struct {
	Record    string
	Status    string
	StartTime string
	EndTime   string
}

func resultAnnotation(r *tektonResult, key string) string {
	if r.Annotations == nil {
		return ""
	}

	v, ok := r.Annotations[key]
	if !ok {
		return ""
	}

	s, ok := v.(string)
	if !ok {
		return ""
	}

	return s
}

func succeededCondition(t *restapi.TaskRun) *restapi.TaskRunCondition {
	if t.Status.Conditions == nil {
		return nil
	}

	for i := range *t.Status.Conditions {
		if (*t.Status.Conditions)[i].Type == conditionSucceeded {
			return &(*t.Status.Conditions)[i]
		}
	}

	return nil
}

func isFailed(t *restapi.TaskRun) bool {
	c := succeededCondition(t)
	return c != nil && c.Status == conditionStatusFalse
}

func conditionStatus(t *restapi.TaskRun) string {
	c := succeededCondition(t)
	if c == nil {
		return ""
	}

	switch c.Status {
	case conditionStatusTrue:
		return displayStatus(resultStatusSuccess)
	case conditionStatusFalse:
		return displayStatus(resultStatusFailure)
	default:
		return displayStatus(resultStatusUnknown)
	}
}

func conditionMessage(t *restapi.TaskRun) string {
	c := succeededCondition(t)
	if c == nil {
		return ""
	}

	return ptr.Deref(c.Message, "")
}

func failedStep(t *restapi.TaskRun) *restapi.TaskRunStep {
	if t.Status.Steps == nil {
		return nil
	}

	for i := range *t.Status.Steps {
		s := &(*t.Status.Steps)[i]
		if s.Terminated != nil && s.Terminated.ExitCode != 0 {
			return s
		}
	}

	return nil
}

// --- Domain output types ---

type PipelineRunInfo struct {
	Name         string `json:"name"`
	PortalURL    string `json:"portalUrl,omitempty"`
	Status       string `json:"status"`
	Pipeline     string `json:"pipeline"`
	Project      string `json:"project"`
	Branch       string `json:"branch,omitempty"`
	PRNumber     string `json:"prNumber,omitempty"`
	PRURL        string `json:"prUrl,omitempty"`
	Author       string `json:"author,omitempty"`
	Type         string `json:"type,omitempty"`
	StartTime    string `json:"startTime"`
	Duration     string `json:"duration,omitempty"`
	TargetBranch string `json:"targetBranch,omitempty"`
	CommitSHA    string `json:"commitSha,omitempty"`
}

type TaskRunInfo struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Duration   string `json:"duration,omitempty"`
	FailedStep string `json:"failedStep,omitempty"`
	ExitCode   int    `json:"exitCode,omitempty"`
	Message    string `json:"message,omitempty"`
	Logs       string `json:"logs,omitempty"`
}

type PipelineRunListResult struct {
	PipelineRuns []PipelineRunInfo `json:"pipelineRuns"`
	Logs         string            `json:"logs,omitempty"`
	Tasks        []TaskRunInfo     `json:"tasks,omitempty"`
}
