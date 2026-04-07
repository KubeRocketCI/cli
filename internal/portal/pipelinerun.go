package portal

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

// PipelineRunService provides access to pipeline run data via the portal's REST API.
type PipelineRunService struct {
	client      *restapi.ClientWithResponses
	portalURL   string
	clusterName string
	namespace   string
}

// NewPipelineRunService creates a PipelineRunService.
func NewPipelineRunService(
	client *restapi.ClientWithResponses, portalURL, clusterName, namespace string,
) *PipelineRunService {
	return &PipelineRunService{
		client:      client,
		portalURL:   portalURL,
		clusterName: clusterName,
		namespace:   namespace,
	}
}

func (s *PipelineRunService) pipelineRunPortalURL(name string) string {
	if s.portalURL == "" || s.clusterName == "" || name == "" {
		return ""
	}

	u, err := url.JoinPath(s.portalURL, "c", s.clusterName, "cicd", "pipelineruns", s.namespace, name)
	if err != nil {
		return ""
	}

	return u
}

// PipelineRunFilter holds composable filter criteria for listing pipeline runs.
type PipelineRunFilter struct {
	Project  string
	PRNumber int
	Author   string
	Branch   string
	Type     string
	Status   string
}

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
	pageSize := float32(5)

	resp, err := s.client.TektonResultsGetPipelineRunResultsWithResponse(ctx,
		&restapi.TektonResultsGetPipelineRunResultsParams{
			Namespace: s.namespace,
			Filter:    &filter,
			PageSize:  &pageSize,
		})
	if err != nil {
		return nil, fmt.Errorf("getting pipeline run: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, fmt.Errorf("getting pipeline run: %w", err)
	}

	if resp.JSON200 == nil || len(resp.JSON200.Results) == 0 {
		return nil, fmt.Errorf("pipeline run %q: %w", name, ErrNotFound)
	}

	r := &resp.JSON200.Results[0]
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
func (s *PipelineRunService) List(ctx context.Context, opts PipelineRunListOptions) (*PipelineRunListResult, error) {
	cel := buildCELFilter(opts.Filter)
	pageSize := float32(10)

	resp, err := s.client.TektonResultsGetPipelineRunResultsWithResponse(ctx,
		&restapi.TektonResultsGetPipelineRunResultsParams{
			Namespace: s.namespace,
			Filter:    ptr.To(cel),
			PageSize:  &pageSize,
		})
	if err != nil {
		return nil, fmt.Errorf("listing pipeline runs: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, fmt.Errorf("listing pipeline runs: %w", err)
	}

	if resp.JSON200 == nil {
		return &PipelineRunListResult{}, nil
	}

	result := &PipelineRunListResult{
		PipelineRuns: make([]PipelineRunInfo, 0, len(resp.JSON200.Results)),
	}

	for i := range resp.JSON200.Results {
		info := mapPipelineRunInfo(&resp.JSON200.Results[i])
		info.PortalURL = s.pipelineRunPortalURL(info.Name)
		result.PipelineRuns = append(result.PipelineRuns, info)
	}

	if len(resp.JSON200.Results) > 0 {
		if err := s.fetchExpansion(ctx, &resp.JSON200.Results[0], result, opts.IncludeLogs, opts.IncludeReason); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// fetchExpansion fetches optional task tree or logs for the given pipeline run result.
func (s *PipelineRunService) fetchExpansion(
	ctx context.Context, r *restapi.TektonResult, out *PipelineRunListResult, includeLogs, includeReason bool,
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

func (s *PipelineRunService) fetchLogs(ctx context.Context, r *restapi.TektonResult) (string, error) {
	if r.Summary == nil {
		return "", nil
	}

	resultUID, recordUID := parseRecordName(r.Summary.Record)
	if resultUID == "" || recordUID == "" {
		return "", nil
	}

	resultUUID, err := uuid.Parse(resultUID)
	if err != nil {
		return "", fmt.Errorf("invalid result UID: %w", err)
	}

	recordUUID, err := uuid.Parse(recordUID)
	if err != nil {
		return "", fmt.Errorf("invalid record UID: %w", err)
	}

	resp, err := s.client.TektonResultsGetPipelineRunLogsWithResponse(ctx,
		resultUUID,
		&restapi.TektonResultsGetPipelineRunLogsParams{
			Namespace: s.namespace,
			RecordUid: recordUUID,
		})
	if err != nil {
		return "", fmt.Errorf("fetching pipeline run logs: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return "", fmt.Errorf("fetching pipeline run logs: %w", err)
	}

	if resp.JSON200 == nil {
		return "", nil
	}

	return stripANSI(resp.JSON200.Logs), nil
}

func (s *PipelineRunService) fetchReason(
	ctx context.Context, r *restapi.TektonResult, out *PipelineRunListResult,
) error {
	if r.Summary == nil {
		return nil
	}

	resultUID, _ := parseRecordName(r.Summary.Record)
	if resultUID == "" {
		return nil
	}

	resultUUID, err := uuid.Parse(resultUID)
	if err != nil {
		return fmt.Errorf("invalid result UID: %w", err)
	}

	resp, err := s.client.TektonResultsGetTaskRunRecordsWithResponse(ctx,
		resultUUID,
		&restapi.TektonResultsGetTaskRunRecordsParams{
			Namespace: s.namespace,
		})
	if err != nil {
		return fmt.Errorf("fetching task runs: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return fmt.Errorf("fetching task runs: %w", err)
	}

	if resp.JSON200 == nil {
		return nil
	}

	taskRuns := resp.JSON200.TaskRuns

	sort.Slice(taskRuns, func(i, j int) bool {
		return ptr.Deref(taskRuns[i].Status.StartTime, "") < ptr.Deref(taskRuns[j].Status.StartTime, "")
	})

	runPrefix := out.PipelineRuns[0].Name + "-"

	out.Tasks = make([]TaskRunInfo, 0, len(taskRuns))

	var failedTaskRuns []restapi.TaskRun

	for i := range taskRuns {
		t := &taskRuns[i]
		ti := mapTaskRunInfo(t)
		ti.Name = strings.TrimPrefix(ti.Name, runPrefix)
		out.Tasks = append(out.Tasks, ti)

		if isFailed(t) {
			failedTaskRuns = append(failedTaskRuns, *t)
		}
	}

	if len(failedTaskRuns) > 0 {
		type taskLogs struct {
			name string
			logs string
		}

		g, gctx := errgroup.WithContext(ctx)
		results := make([]taskLogs, len(failedTaskRuns))

		for i, ft := range failedTaskRuns {
			g.Go(func() error {
				logResp, err := s.client.TektonResultsGetTaskRunLogsWithResponse(gctx,
					resultUUID,
					&restapi.TektonResultsGetTaskRunLogsParams{
						Namespace:   s.namespace,
						TaskRunName: ft.Metadata.Name,
					})
				if err != nil {
					return fmt.Errorf("fetching logs for task %q: %w", ft.Metadata.Name, err)
				}

				if err := checkResponse(logResp.StatusCode(), logResp.Body); err != nil {
					return fmt.Errorf("fetching logs for task %q: %w", ft.Metadata.Name, err)
				}

				if logResp.JSON200 != nil {
					if errStr, err := logResp.JSON200.Error.Get(); err == nil && errStr != "" {
						return fmt.Errorf("task %q logs: %s", ft.Metadata.Name, errStr)
					}

					results[i] = taskLogs{name: ft.Metadata.Name, logs: stripANSI(logResp.JSON200.Logs)}
				}

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}

		logsByName := make(map[string]string, len(results))
		for _, r := range results {
			logsByName[r.name] = r.logs
		}

		for i := range out.Tasks {
			fullName := runPrefix + out.Tasks[i].Name
			if logs, ok := logsByName[fullName]; ok {
				out.Tasks[i].Logs = logs
			}
		}
	}

	return nil
}

// mapTaskRunInfo converts a TaskRun to the display model.
func mapTaskRunInfo(t *restapi.TaskRun) TaskRunInfo {
	info := TaskRunInfo{
		Name:   t.Metadata.Name,
		Status: conditionStatus(t),
	}

	info.Duration = computeTaskDuration(t)
	info.Message = conditionMessage(t)

	if step := failedStep(t); step != nil {
		info.FailedStep = ptr.Deref(step.Name, "")
		if step.Terminated != nil {
			info.ExitCode = step.Terminated.ExitCode
		}
	}

	return info
}

func computeTaskDuration(t *restapi.TaskRun) string {
	startStr := ptr.Deref(t.Status.StartTime, "")
	completionStr := ptr.Deref(t.Status.CompletionTime, "")

	if startStr == "" || completionStr == "" {
		return ""
	}

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return ""
	}

	end, err := time.Parse(time.RFC3339, completionStr)
	if err != nil {
		return ""
	}

	d := end.Sub(start)
	if d < 0 {
		return ""
	}

	return formatDuration(d)
}

// mapPipelineRunInfo converts a Tekton Result to the display model.
func mapPipelineRunInfo(r *restapi.TektonResult) PipelineRunInfo {
	name := resultAnnotation(r, annotationObjectName)
	if name == "" {
		name = r.Uid
	}

	status := ""
	if r.Summary != nil {
		status = displayStatus(string(r.Summary.Status))
	}

	return PipelineRunInfo{
		Name:         name,
		Status:       status,
		Pipeline:     resultAnnotation(r, annotationPipeline),
		Project:      resultAnnotation(r, annotationCodebase),
		Branch:       resultAnnotation(r, annotationGitBranch),
		PRNumber:     resultAnnotation(r, annotationGitChangeNumber),
		PRURL:        resultAnnotation(r, annotationGitChangeURL),
		Author:       resultAnnotation(r, annotationGitAuthor),
		Type:         resultAnnotation(r, annotationPipelineType),
		StartTime:    r.CreateTime,
		Duration:     computeDuration(r),
		TargetBranch: resultAnnotation(r, annotationGitTargetBranch),
		CommitSHA:    resultAnnotation(r, annotationGitCommitSHA),
	}
}

func computeDuration(r *restapi.TektonResult) string {
	if r.Summary == nil || string(r.Summary.Status) == resultStatusUnknown {
		return ""
	}

	endStr := ptr.Deref(r.Summary.EndTime, "")
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

func buildCELFilter(f PipelineRunFilter) string {
	var parts []string

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

func parseRecordName(name string) (resultUID, recordUID string) {
	parts := strings.Split(name, "/")
	if len(parts) >= 5 && parts[1] == "results" && parts[3] == "records" {
		return parts[2], parts[4]
	}

	return "", ""
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

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
