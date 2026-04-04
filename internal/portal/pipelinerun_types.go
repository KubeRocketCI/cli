package portal

// Tekton Result annotation keys.
// Source of truth: krci-portal/packages/shared/src/models/tektonResults/annotations.ts
const (
	annotationCodebase        = "app.edp.epam.com/codebase"
	annotationPipelineType    = "app.edp.epam.com/pipelinetype"
	annotationGitChangeNumber = "app.edp.epam.com/git-change-number"
	annotationGitChangeURL    = "app.edp.epam.com/git-change-url"
	annotationGitAuthor       = "app.edp.epam.com/git-author"
	annotationGitBranch       = "app.edp.epam.com/git-branch"
	annotationGitTargetBranch = "app.edp.epam.com/git-target-branch"
	annotationGitCommitSHA    = "app.edp.epam.com/git-commit-sha"
	annotationGitRepository   = "app.edp.epam.com/git-repository"
	annotationPipeline        = "tekton.dev/pipeline"
	annotationObjectName      = "object.metadata.name"
)

// Pipeline run status values from Tekton Results summary.
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

// statusDisplay maps Tekton Result summary status to human-readable display text.
// Matches portal's RESULT_STATUS_MAP (adapters.ts:142-148).
var statusDisplay = map[string]string{
	resultStatusSuccess:   StatusSucceeded,
	resultStatusFailure:   StatusFailed,
	resultStatusTimeout:   StatusTimeout,
	resultStatusCancelled: StatusCancelled,
	resultStatusUnknown:   StatusRunning,
}

// displayStatus returns the human-readable status text for a Tekton Result status.
func displayStatus(resultStatus string) string {
	if s, ok := statusDisplay[resultStatus]; ok {
		return s
	}

	return resultStatus
}

// --- tRPC input types ---

// pipelineRunResultsInput is the input for tektonResults.getPipelineRunResults.
type pipelineRunResultsInput struct {
	Namespace string `json:"namespace"`
	Filter    string `json:"filter,omitempty"`
	PageSize  int    `json:"pageSize,omitempty"`
	PageToken string `json:"pageToken,omitempty"`
}

// pipelineRunLogsInput is the input for tektonResults.getPipelineRunLogs.
type pipelineRunLogsInput struct {
	Namespace string `json:"namespace"`
	ResultUID string `json:"resultUid"`
	RecordUID string `json:"recordUid"`
}

// --- tRPC response types (match portal shared types) ---

// tektonResultSummary is the summary section of a Tekton Result.
type tektonResultSummary struct {
	Record string `json:"record"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Start  string `json:"start_time,omitempty"`
	End    string `json:"end_time,omitempty"`
}

// tektonResult represents a single Result from the Tekton Results API.
type tektonResult struct {
	UID         string               `json:"uid"`
	Name        string               `json:"name"`
	CreateTime  string               `json:"create_time"`
	UpdateTime  string               `json:"update_time"`
	Summary     *tektonResultSummary `json:"summary,omitempty"`
	Annotations map[string]any       `json:"annotations,omitempty"`
}

// resultAnnotation returns the string value of an annotation, or "" if absent.
func (r *tektonResult) resultAnnotation(key string) string {
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

// tektonResultsList is the response from tektonResults.getPipelineRunResults.
type tektonResultsList struct {
	Results       []tektonResult `json:"results"`
	NextPageToken string         `json:"nextPageToken,omitempty"`
}

// pipelineRunLogsResponse is the response from tektonResults.getPipelineRunLogs.
type pipelineRunLogsResponse struct {
	Logs string `json:"logs"`
}

// taskRunRecordsInput is the input for tektonResults.getTaskRunRecords.
type taskRunRecordsInput struct {
	Namespace string `json:"namespace"`
	ResultUID string `json:"resultUid"`
}

// taskRunLogsInput is the input for tektonResults.getTaskRunLogs.
type taskRunLogsInput struct {
	Namespace   string `json:"namespace"`
	ResultUID   string `json:"resultUid"`
	TaskRunName string `json:"taskRunName"`
}

// K8s condition type for pipeline/task run completion.
const conditionSucceeded = "Succeeded"

// decodedTaskRunCondition is a K8s-style condition on a TaskRun.
type decodedTaskRunCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// decodedTaskRunStepTerminated holds termination state for a step.
type decodedTaskRunStepTerminated struct {
	ExitCode   int    `json:"exitCode"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

// decodedTaskRunStep represents a single step within a TaskRun.
type decodedTaskRunStep struct {
	Name       string                        `json:"name,omitempty"`
	Terminated *decodedTaskRunStepTerminated `json:"terminated,omitempty"`
}

// decodedTaskRunStatus is the status section of a decoded TaskRun.
type decodedTaskRunStatus struct {
	Conditions     []decodedTaskRunCondition `json:"conditions,omitempty"`
	Steps          []decodedTaskRunStep      `json:"steps,omitempty"`
	StartTime      string                    `json:"startTime,omitempty"`
	CompletionTime string                    `json:"completionTime,omitempty"`
}

// decodedTaskRunMetadata is the metadata section of a decoded TaskRun.
type decodedTaskRunMetadata struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// decodedTaskRun is a decoded TaskRun from Tekton Results records.
type decodedTaskRun struct {
	Metadata decodedTaskRunMetadata `json:"metadata"`
	Status   decodedTaskRunStatus   `json:"status"`
}

// succeededCondition returns the Succeeded condition, or nil if absent.
func (t *decodedTaskRun) succeededCondition() *decodedTaskRunCondition {
	for i := range t.Status.Conditions {
		if t.Status.Conditions[i].Type == conditionSucceeded {
			return &t.Status.Conditions[i]
		}
	}

	return nil
}

// isFailed returns true if the TaskRun's Succeeded condition is False.
func (t *decodedTaskRun) isFailed() bool {
	c := t.succeededCondition()
	return c != nil && c.Status == "False"
}

// conditionStatus returns the human-readable status from the Succeeded condition.
func (t *decodedTaskRun) conditionStatus() string {
	c := t.succeededCondition()
	if c == nil {
		return ""
	}

	switch c.Status {
	case "True":
		return displayStatus(resultStatusSuccess)
	case "False":
		return displayStatus(resultStatusFailure)
	default:
		return displayStatus(resultStatusUnknown)
	}
}

// conditionMessage returns the message from the Succeeded condition.
func (t *decodedTaskRun) conditionMessage() string {
	c := t.succeededCondition()
	if c == nil {
		return ""
	}

	return c.Message
}

// failedStep returns the first step with a non-zero exit code, or nil.
func (t *decodedTaskRun) failedStep() *decodedTaskRunStep {
	for i := range t.Status.Steps {
		s := &t.Status.Steps[i]
		if s.Terminated != nil && s.Terminated.ExitCode != 0 {
			return s
		}
	}

	return nil
}

// taskRunRecordsResponse is the response from tektonResults.getTaskRunRecords.
type taskRunRecordsResponse struct {
	TaskRuns []decodedTaskRun `json:"taskRuns"`
}

// taskRunLogsResponse is the response from tektonResults.getTaskRunLogs.
type taskRunLogsResponse struct {
	TaskRunName string `json:"taskRunName"`
	Logs        string `json:"logs"`
	HasLogs     bool   `json:"hasLogs"`
	Error       string `json:"error"`
}

// --- Domain output types ---

// PipelineRunInfo holds the display data for a single pipeline run.
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

// TaskRunInfo holds the display data for a single task run.
type TaskRunInfo struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Duration   string `json:"duration,omitempty"`
	FailedStep string `json:"failedStep,omitempty"`
	ExitCode   int    `json:"exitCode,omitempty"`
	Message    string `json:"message,omitempty"`
	Logs       string `json:"logs,omitempty"`
}

// PipelineRunListResult is the full output for the pipelinerun list command.
type PipelineRunListResult struct {
	PipelineRuns []PipelineRunInfo `json:"pipelineRuns"`
	Logs         string            `json:"logs,omitempty"`
	Tasks        []TaskRunInfo     `json:"tasks,omitempty"`
}
