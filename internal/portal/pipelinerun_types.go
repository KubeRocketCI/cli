package portal

import (
	"fmt"
	"strings"

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
	annotationCDPipeline        = "app.edp.epam.com/cdpipeline"
	annotationCDStage           = "app.edp.epam.com/cdstage"
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

// isFinishedStatus reports whether a run has reached a final status. A run
// with no status yet (the reconciler has not started it) is not finished.
func isFinishedStatus(status string) bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusTimeout, StatusCancelled:
		return true
	default:
		return false
	}
}

var statusDisplay = map[string]string{
	resultStatusSuccess:   StatusSucceeded,
	resultStatusFailure:   StatusFailed,
	resultStatusTimeout:   StatusTimeout,
	resultStatusCancelled: StatusCancelled,
	resultStatusUnknown:   StatusRunning,
}

// statusFilter pairs a user-facing status filter keyword with the display
// label and the Tekton Results CEL numeric value it selects.
type statusFilter struct {
	keyword string
	display string
	cel     string
}

// statusFilters is in the order help and error messages list the keywords.
var statusFilters = []statusFilter{
	{keyword: "succeeded", display: StatusSucceeded, cel: "1"},
	{keyword: "failed", display: StatusFailed, cel: "2"},
	{keyword: "running", display: StatusRunning, cel: "0"},
	{keyword: "timeout", display: StatusTimeout, cel: "3"},
	{keyword: "cancelled", display: StatusCancelled, cel: "4"},
}

// findStatusFilter looks a status filter keyword up, ignoring case.
func findStatusFilter(keyword string) (statusFilter, bool) {
	keyword = strings.ToLower(keyword)

	for _, f := range statusFilters {
		if f.keyword == keyword {
			return f, true
		}
	}

	return statusFilter{}, false
}

// PipelineRunStatusKeywords returns the keywords the status filter accepts.
func PipelineRunStatusKeywords() []string {
	keywords := make([]string, len(statusFilters))
	for i, f := range statusFilters {
		keywords[i] = f.keyword
	}

	return keywords
}

// ValidatePipelineRunStatus rejects a status filter keyword that is not one of
// PipelineRunStatusKeywords; case is ignored. name is how the error refers to
// the value, such as the flag that carries it.
func ValidatePipelineRunStatus(name, keyword string) error {
	if _, ok := findStatusFilter(keyword); ok {
		return nil
	}

	return fmt.Errorf("invalid %s=%s; must be one of %s",
		name, keyword, strings.Join(PipelineRunStatusKeywords(), ", "))
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

// finished reports whether the summarised run has ended. The Tekton Results
// (v0.20) watcher leaves Status UNKNOWN for a run that ended with a reason it
// does not classify (CouldntGetPipeline, PipelineValidationFailed,
// CreateRunFailed, ...) and sets EndTime from the run's completionTime
// regardless.
func (sum *tektonResultSummary) finished() bool {
	return sum.Status != resultStatusUnknown || sum.EndTime != ""
}

// summaryStatus is the display status of a Tekton Results summary. UNKNOWN
// with an EndTime is Failed: the run ended with Succeeded=False.
func summaryStatus(sum *tektonResultSummary) string {
	if sum.Status == resultStatusUnknown && sum.finished() {
		return StatusFailed
	}

	return displayStatus(sum.Status)
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
	Deployment   string `json:"deployment,omitempty"`
	Env          string `json:"env,omitempty"`

	// Results maps the run's pipeline results (status.results) by name, e.g.
	// VCS_TAG of a build. Only a run still in the cluster carries them: Tekton
	// Results summaries do not.
	Results map[string]any `json:"results,omitempty"`
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
	// TasksUnavailable is the reason a --reason result carries no Tasks.
	TasksUnavailable string `json:"tasksUnavailable,omitempty"`
}

// Values of PipelineRunListResult.TasksUnavailable.
const (
	// TasksRunNotFinished: the run is pending or still running, and task data
	// is read from Tekton Results once it has finished.
	TasksRunNotFinished = "run_not_finished"
	// TasksNotIndexed: the run has finished and Tekton Results has no task
	// data for it yet.
	TasksNotIndexed = "not_indexed"
	// TasksNone: the run has finished and its Tekton Results record lists no
	// TaskRun; it never scheduled a task.
	TasksNone = "no_tasks"
)
