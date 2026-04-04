package portal

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// PipelineRunService provides access to pipeline run data via the portal's Tekton Results tRPC API.
type PipelineRunService struct {
	client      *Client
	portalURL   string
	clusterName string
	namespace   string
}

// NewPipelineRunService creates a PipelineRunService.
func NewPipelineRunService(client *Client, portalURL, clusterName, namespace string) *PipelineRunService {
	return &PipelineRunService{
		client:      client,
		portalURL:   strings.TrimSuffix(portalURL, "/"),
		clusterName: clusterName,
		namespace:   namespace,
	}
}

// pipelineRunPortalURL builds the portal URL for a pipeline run detail page.
// Pattern: /c/{clusterName}/cicd/pipelineruns/{namespace}/{name}
func (s *PipelineRunService) pipelineRunPortalURL(name string) string {
	if s.portalURL == "" || s.clusterName == "" || name == "" {
		return ""
	}

	return fmt.Sprintf("%s/c/%s/cicd/pipelineruns/%s/%s", s.portalURL, s.clusterName, s.namespace, name)
}

// PipelineRunFilter holds composable filter criteria for listing pipeline runs.
// Multiple filters are combined with AND logic in the CEL query.
type PipelineRunFilter struct {
	Project  string // app.edp.epam.com/codebase
	PRNumber int    // app.edp.epam.com/git-change-number
	Author   string // app.edp.epam.com/git-author
	Branch   string // app.edp.epam.com/git-branch
	Type     string // app.edp.epam.com/pipelinetype (review, build)
	Status   string // summary.status (succeeded, failed, running, timeout, cancelled)
}

// statusCELValue maps user-friendly status names to Tekton Results proto enum values.
// Proto: UNKNOWN=0, SUCCESS=1, FAILURE=2, TIMEOUT=3, CANCELLED=4
var statusCELValue = map[string]string{
	"succeeded": "1",
	"failed":    "2",
	"timeout":   "3",
	"cancelled": "4",
	"running":   "0",
}

// PipelineRunGetOptions controls expansion for Get.
type PipelineRunGetOptions struct {
	IncludeLogs   bool
	IncludeReason bool
}

// Get returns a single pipeline run by name.
func (s *PipelineRunService) Get(
	ctx context.Context, name string, opts PipelineRunGetOptions,
) (*PipelineRunListResult, error) {
	filter := fmt.Sprintf("annotations[%q] == %q", annotationObjectName, name)

	input := pipelineRunResultsInput{
		Namespace: s.namespace,
		Filter:    filter,
		PageSize:  5,
	}

	var list tektonResultsList
	if err := s.client.Query(ctx, procedurePipelineRunResults, input, &list); err != nil {
		return nil, fmt.Errorf("getting pipeline run: %w", err)
	}

	if len(list.Results) == 0 {
		return nil, fmt.Errorf("pipeline run %q: %w", name, ErrNotFound)
	}

	r := &list.Results[0]
	info := mapPipelineRunInfo(r)
	info.PortalURL = s.pipelineRunPortalURL(info.Name)

	result := &PipelineRunListResult{
		PipelineRuns: []PipelineRunInfo{info},
	}

	if err := s.fetchExpansion(ctx, r, result, opts.IncludeLogs, opts.IncludeReason); err != nil {
		return nil, err
	}

	return result, nil
}

// PipelineRunListOptions controls filtering and expansion for List.
type PipelineRunListOptions struct {
	Filter        PipelineRunFilter
	IncludeLogs   bool
	IncludeReason bool
}

// List returns pipeline runs matching the given filters.
// When PRNumber filter is set, the most recent run is auto-expanded with failure details.
func (s *PipelineRunService) List(ctx context.Context, opts PipelineRunListOptions) (*PipelineRunListResult, error) {
	cel := buildCELFilter(opts.Filter)

	input := pipelineRunResultsInput{
		Namespace: s.namespace,
		Filter:    cel,
		PageSize:  10,
	}

	var list tektonResultsList
	if err := s.client.Query(ctx, procedurePipelineRunResults, input, &list); err != nil {
		return nil, fmt.Errorf("listing pipeline runs: %w", err)
	}

	result := &PipelineRunListResult{
		PipelineRuns: make([]PipelineRunInfo, 0, len(list.Results)),
	}

	for i := range list.Results {
		info := mapPipelineRunInfo(&list.Results[i])
		info.PortalURL = s.pipelineRunPortalURL(info.Name)
		result.PipelineRuns = append(result.PipelineRuns, info)
	}

	if len(list.Results) > 0 {
		if err := s.fetchExpansion(ctx, &list.Results[0], result, opts.IncludeLogs, opts.IncludeReason); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// fetchExpansion fetches optional task tree or logs for the given pipeline run result.
func (s *PipelineRunService) fetchExpansion(
	ctx context.Context, r *tektonResult, out *PipelineRunListResult, includeLogs, includeReason bool,
) error {
	if includeReason {
		return s.fetchReason(ctx, r, out)
	}

	if includeLogs {
		logs, err := s.fetchLogs(ctx, r)
		if err != nil {
			return err
		}

		out.Logs = logs
	}

	return nil
}

// fetchLogs retrieves logs for a pipeline run from Tekton Results.
func (s *PipelineRunService) fetchLogs(ctx context.Context, r *tektonResult) (string, error) {
	if r.Summary == nil {
		return "", nil
	}

	resultUID, recordUID := parseRecordName(r.Summary.Record)
	if resultUID == "" || recordUID == "" {
		return "", nil
	}

	var logsResp pipelineRunLogsResponse

	input := pipelineRunLogsInput{
		Namespace: s.namespace,
		ResultUID: resultUID,
		RecordUID: recordUID,
	}
	if err := s.client.Query(ctx, procedurePipelineRunLogs, input, &logsResp); err != nil {
		return "", fmt.Errorf("fetching pipeline run logs: %w", err)
	}

	return stripANSI(logsResp.Logs), nil
}

// fetchReason fetches task tree and failed task logs for the given pipeline run.
func (s *PipelineRunService) fetchReason(
	ctx context.Context, r *tektonResult, out *PipelineRunListResult,
) error {
	if r.Summary == nil {
		return nil
	}

	resultUID, _ := parseRecordName(r.Summary.Record)
	if resultUID == "" {
		return nil
	}

	// Fetch all TaskRuns.
	var taskResp taskRunRecordsResponse

	input := taskRunRecordsInput{Namespace: s.namespace, ResultUID: resultUID}
	if err := s.client.Query(ctx, procedureTaskRunRecords, input, &taskResp); err != nil {
		return fmt.Errorf("fetching task runs: %w", err)
	}

	// Sort TaskRuns by startTime to approximate execution order.
	sort.Slice(taskResp.TaskRuns, func(i, j int) bool {
		return taskResp.TaskRuns[i].Status.StartTime < taskResp.TaskRuns[j].Status.StartTime
	})

	// Strip PipelineRun name prefix from task names for readability.
	runPrefix := out.PipelineRuns[0].Name + "-"

	out.Tasks = make([]TaskRunInfo, 0, len(taskResp.TaskRuns))

	var failedTaskRuns []decodedTaskRun

	for i := range taskResp.TaskRuns {
		t := &taskResp.TaskRuns[i]
		ti := mapTaskRunInfo(t)
		ti.Name = strings.TrimPrefix(ti.Name, runPrefix)
		out.Tasks = append(out.Tasks, ti)

		if t.isFailed() {
			failedTaskRuns = append(failedTaskRuns, *t)
		}
	}

	// Fetch logs for each failed task in parallel.
	if len(failedTaskRuns) > 0 {
		type taskLogs struct {
			name string
			logs string
		}

		g, gctx := errgroup.WithContext(ctx)
		results := make([]taskLogs, len(failedTaskRuns))

		for i, ft := range failedTaskRuns {
			g.Go(func() error {
				var resp taskRunLogsResponse

				logInput := taskRunLogsInput{
					Namespace:   s.namespace,
					ResultUID:   resultUID,
					TaskRunName: ft.Metadata.Name,
				}
				if err := s.client.Query(gctx, procedureTaskRunLogs, logInput, &resp); err != nil {
					return fmt.Errorf("fetching logs for task %q: %w", ft.Metadata.Name, err)
				}

				if resp.Error != "" {
					return fmt.Errorf("task %q logs: %s", ft.Metadata.Name, resp.Error)
				}

				results[i] = taskLogs{name: ft.Metadata.Name, logs: stripANSI(resp.Logs)}

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}

		// Build name→logs map, then attach to the corresponding TaskRunInfo entries.
		logsByName := make(map[string]string, len(results))
		for _, r := range results {
			logsByName[r.name] = r.logs
		}

		for i := range out.Tasks {
			// Task names were prefix-stripped; match against the original full name.
			fullName := runPrefix + out.Tasks[i].Name
			if logs, ok := logsByName[fullName]; ok {
				out.Tasks[i].Logs = logs
			}
		}
	}

	return nil
}

// mapTaskRunInfo converts a decoded TaskRun to the display model.
func mapTaskRunInfo(t *decodedTaskRun) TaskRunInfo {
	info := TaskRunInfo{
		Name:   t.Metadata.Name,
		Status: t.conditionStatus(),
	}

	info.Duration = computeTaskDuration(t)
	info.Message = t.conditionMessage()

	if step := t.failedStep(); step != nil {
		info.FailedStep = step.Name
		if step.Terminated != nil {
			info.ExitCode = step.Terminated.ExitCode
		}
	}

	return info
}

// computeTaskDuration calculates duration from TaskRun start/completion times.
func computeTaskDuration(t *decodedTaskRun) string {
	if t.Status.StartTime == "" || t.Status.CompletionTime == "" {
		return ""
	}

	start, err := time.Parse(time.RFC3339, t.Status.StartTime)
	if err != nil {
		return ""
	}

	end, err := time.Parse(time.RFC3339, t.Status.CompletionTime)
	if err != nil {
		return ""
	}

	d := end.Sub(start)
	if d < 0 {
		return ""
	}

	return formatDuration(d)
}

// buildCELFilter constructs the CEL filter expression from composable filters.
// Empty filters produce an empty string (no filtering, returns all pipeline runs).
func buildCELFilter(f PipelineRunFilter) string {
	var parts []string

	// Annotation filters in a fixed order.
	filters := []struct {
		value      string
		annotation string
	}{
		{f.Project, annotationCodebase},
		{f.Author, annotationGitAuthor},
		{f.Branch, annotationGitBranch},
		{f.Type, annotationPipelineType},
	}

	for _, m := range filters {
		if m.value != "" {
			parts = append(parts, fmt.Sprintf("annotations[%q] == %q", m.annotation, m.value))
		}
	}

	if f.PRNumber > 0 {
		parts = append(parts, fmt.Sprintf("annotations[%q] == %q", annotationGitChangeNumber, fmt.Sprintf("%d", f.PRNumber)))
	}

	if v, ok := statusCELValue[strings.ToLower(f.Status)]; ok {
		parts = append(parts, fmt.Sprintf("summary.status == %s", v))
	}

	return strings.Join(parts, " && ")
}

// mapPipelineRunInfo converts a Tekton Result to the display model.
func mapPipelineRunInfo(r *tektonResult) PipelineRunInfo {
	name := r.resultAnnotation(annotationObjectName)
	if name == "" {
		name = r.UID
	}

	status := ""
	if r.Summary != nil {
		status = displayStatus(r.Summary.Status)
	}

	startTime := r.CreateTime
	duration := computeDuration(r)

	return PipelineRunInfo{
		Name:         name,
		Status:       status,
		Pipeline:     r.resultAnnotation(annotationPipeline),
		Project:      r.resultAnnotation(annotationCodebase),
		Branch:       r.resultAnnotation(annotationGitBranch),
		PRNumber:     r.resultAnnotation(annotationGitChangeNumber),
		PRURL:        r.resultAnnotation(annotationGitChangeURL),
		Author:       r.resultAnnotation(annotationGitAuthor),
		Type:         r.resultAnnotation(annotationPipelineType),
		StartTime:    startTime,
		Duration:     duration,
		TargetBranch: r.resultAnnotation(annotationGitTargetBranch),
		CommitSHA:    r.resultAnnotation(annotationGitCommitSHA),
	}
}

// computeDuration calculates the duration string for a pipeline run.
// Matches portal logic: end_time fallback to update_time (Tekton Results watcher bug).
func computeDuration(r *tektonResult) string {
	if r.Summary == nil {
		return ""
	}

	if r.Summary.Status == resultStatusUnknown {
		return ""
	}

	endStr := r.Summary.End
	if endStr == "" {
		endStr = r.UpdateTime
	}

	if endStr == "" || r.CreateTime == "" {
		return ""
	}

	start, err := time.Parse(time.RFC3339, r.CreateTime)
	if err != nil {
		return ""
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return ""
	}

	d := end.Sub(start)
	if d < 0 {
		return ""
	}

	return formatDuration(d)
}

// formatDuration formats a duration as a human-readable string (e.g. "4m 12s").
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	switch {
	case h > 0:
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// parseRecordName extracts resultUID and recordUID from a Tekton Results record name.
// Format: "{namespace}/results/{resultUID}/records/{recordUID}"
func parseRecordName(name string) (resultUID, recordUID string) {
	parts := strings.Split(name, "/")
	if len(parts) >= 5 && parts[1] == "results" && parts[3] == "records" {
		return parts[2], parts[4]
	}

	return "", ""
}

// ansiPattern matches ANSI escape sequences for stripping from log output.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
