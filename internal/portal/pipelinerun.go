package portal

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

var pipelineRunResourceConfig = restapi.K8sListJSONBody{
	ResourceConfig: struct {
		ApiVersion    string             `json:"apiVersion"`
		ClusterScoped *bool              `json:"clusterScoped,omitempty"`
		Group         string             `json:"group"`
		Kind          string             `json:"kind"`
		Labels        *map[string]string `json:"labels,omitempty"`
		PluralName    string             `json:"pluralName"`
		SingularName  string             `json:"singularName"`
		Version       string             `json:"version"`
	}{
		ApiVersion:   "tekton.dev/v1",
		Group:        "tekton.dev",
		Kind:         "PipelineRun",
		PluralName:   "pipelineruns",
		SingularName: "pipelinerun",
		Version:      "v1",
	},
}

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

// PipelineRunGetOptions controls expansion for Get.
type PipelineRunGetOptions struct {
	IncludeLogs   bool
	IncludeReason bool
}

// getFromResults returns a single pipeline run by name from Tekton Results.
func (s *PipelineRunService) getFromResults(
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

// Get returns a single pipeline run by name, checking K8s first (the only source for
// in-flight runs) and falling back to Tekton Results for completed pipelines.
func (s *PipelineRunService) Get(
	ctx context.Context, name string, opts PipelineRunGetOptions,
) (*PipelineRunListResult, error) {
	// K8s is the only source that includes running pipelines.
	k8sResult, err := s.getLiveRun(ctx, name)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			// Hard K8s error (auth, network, etc.): do not mask with a Tekton Results fallback.
			return nil, err
		}
		// Run not in K8s; fall back to Tekton Results (has completed pipelines with full history).
		return s.getFromResults(ctx, name, opts)
	}

	if k8sResult.PipelineRuns[0].Status == StatusRunning {
		// Pipeline is still running: logs/reason are not available from Tekton Results yet.
		return k8sResult, nil
	}

	// Pipeline completed but still visible in K8s. Try Tekton Results for full expansion.
	if opts.IncludeLogs || opts.IncludeReason {
		tektonResult, tektonErr := s.getFromResults(ctx, name, opts)
		if tektonErr == nil {
			return tektonResult, nil
		}

		if !errors.Is(tektonErr, ErrNotFound) {
			return nil, tektonErr
		}
		// ErrNotFound: run not yet indexed in Tekton Results; fall through to K8s result.
	}

	return k8sResult, nil
}

// getLiveRun fetches a single pipeline run from K8s by name.
// It lists all live runs and scans for the matching name since the K8s list
// API does not support filtering by resource name directly.
func (s *PipelineRunService) getLiveRun(ctx context.Context, name string) (*PipelineRunListResult, error) {
	runs, err := s.fetchLiveRuns(ctx, PipelineRunFilter{})
	if err != nil {
		return nil, err
	}

	for _, r := range runs {
		if r.Name == name {
			return &PipelineRunListResult{PipelineRuns: []PipelineRunInfo{r}}, nil
		}
	}

	return nil, ErrNotFound
}

// PipelineRunListOptions controls filtering and expansion for List.
type PipelineRunListOptions struct {
	Filter        PipelineRunFilter
	IncludeLogs   bool
	IncludeReason bool
}

// listFromResults returns pipeline runs from Tekton Results matching the given filters.
func (s *PipelineRunService) listFromResults(
	ctx context.Context, opts PipelineRunListOptions,
) (*PipelineRunListResult, error) {
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

// List returns both running (from K8s) and completed (from Tekton Results)
// pipeline runs, merged and deduplicated. Same unified view as the portal UI.
func (s *PipelineRunService) List(
	ctx context.Context, opts PipelineRunListOptions,
) (*PipelineRunListResult, error) {
	g, ctx := errgroup.WithContext(ctx)

	var liveRuns []PipelineRunInfo
	var historyResult *PipelineRunListResult

	g.Go(func() error {
		runs, err := s.fetchLiveRuns(ctx, opts.Filter)
		if err != nil {
			return err
		}
		liveRuns = runs
		return nil
	})

	// Tekton Results only stores completed pipeline runs, so querying it while
	// filtering for "running" causes API errors — skip that round trip entirely.
	if !strings.EqualFold(opts.Filter.Status, StatusRunning) {
		g.Go(func() error {
			result, err := s.listFromResults(ctx, opts)
			if err != nil {
				return err
			}
			historyResult = result
			return nil
		})
	} else {
		historyResult = &PipelineRunListResult{}
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	allRuns := mergePipelineRuns(liveRuns, historyResult.PipelineRuns)

	result := &PipelineRunListResult{
		PipelineRuns: allRuns,
	}

	// Only carry over expansion data (logs/tasks) if the run it was fetched for
	// (historyResult.PipelineRuns[0]) is still the top result after merging. A live
	// run may have sorted ahead of it, or a different history run may now be first.
	if len(allRuns) > 0 && len(historyResult.PipelineRuns) > 0 &&
		allRuns[0].Name == historyResult.PipelineRuns[0].Name {
		result.Logs = historyResult.Logs
		result.Tasks = historyResult.Tasks
	}

	return result, nil
}

// mergePipelineRuns combines live (K8s) and history (Tekton Results) runs into a single
// deduplicated slice sorted by StartTime descending. Live runs take precedence: any
// history entry whose name is already covered by a live run is dropped.
func mergePipelineRuns(live, history []PipelineRunInfo) []PipelineRunInfo {
	liveNameSet := make(map[string]bool, len(live))
	for _, pr := range live {
		liveNameSet[pr.Name] = true
	}

	all := make([]PipelineRunInfo, 0, len(live)+len(history))
	all = append(all, live...)

	for _, pr := range history {
		if !liveNameSet[pr.Name] {
			all = append(all, pr)
		}
	}

	// Parse once per entry: comparing as strings would mis-order sources with
	// different precisions (Results emits nanoseconds, K8s emits seconds).
	starts := make(map[string]time.Time, len(all))
	for _, pr := range all {
		// Parse errors leave a zero time, sinking malformed/empty entries to the bottom of the sort.
		starts[pr.Name], _ = time.Parse(time.RFC3339Nano, pr.StartTime)
	}

	sort.Slice(all, func(i, j int) bool {
		return starts[all[i].Name].After(starts[all[j].Name])
	})

	return all
}

// fetchLiveRuns fetches pipeline runs from the K8s API and applies client-side filtering.
func (s *PipelineRunService) fetchLiveRuns(ctx context.Context, filter PipelineRunFilter) ([]PipelineRunInfo, error) {
	resp, err := s.client.K8sListWithResponse(ctx, restapi.K8sListJSONRequestBody{
		ClusterName:    s.clusterName,
		Namespace:      ptr.To(s.namespace),
		ResourceConfig: pipelineRunResourceConfig.ResourceConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("fetching live pipeline runs: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, fmt.Errorf("fetching live pipeline runs: %w", err)
	}

	if resp.JSON200 == nil || len(resp.JSON200.Items) == 0 {
		return []PipelineRunInfo{}, nil
	}

	runs := make([]PipelineRunInfo, 0, len(resp.JSON200.Items))
	for i := range resp.JSON200.Items {
		info := mapK8sPipelineRunInfo(&resp.JSON200.Items[i])

		// Apply all filters client-side (K8s API doesn't support label selectors in this endpoint)
		if !matchesFilter(info, filter) {
			continue
		}

		info.PortalURL = s.pipelineRunPortalURL(info.Name)
		runs = append(runs, info)
	}

	return runs, nil
}

// matchesFilter checks if a pipeline run matches all non-empty filter criteria.
func matchesFilter(info PipelineRunInfo, filter PipelineRunFilter) bool {
	if filter.Project != "" && info.Project != filter.Project {
		return false
	}

	if filter.Type != "" && info.Type != filter.Type {
		return false
	}

	if filter.Branch != "" && info.Branch != filter.Branch {
		return false
	}

	if filter.Author != "" && info.Author != filter.Author {
		return false
	}

	if filter.PRNumber > 0 && info.PRNumber != strconv.Itoa(filter.PRNumber) {
		return false
	}

	if filter.Status != "" && !matchesStatus(info.Status, filter.Status) {
		return false
	}

	return true
}

// matchesStatus checks if the actual status matches the filter status (case-insensitive).
func matchesStatus(actualStatus, filterStatus string) bool {
	m, ok := statusMappings[strings.ToLower(filterStatus)]
	if !ok {
		return false
	}

	return actualStatus == m.display
}

// mapK8sPipelineRunInfo converts a K8s PipelineRun item to the display model.
func mapK8sPipelineRunInfo(item *restapi.K8sList_200_Items_Item) PipelineRunInfo {
	info := PipelineRunInfo{
		Name: item.Metadata.Name,
	}

	if labels := ptr.Deref(item.Metadata.Labels, nil); labels != nil {
		info.Project = labels[annotationCodebase]
		info.Type = labels[annotationPipelineType]
		info.Branch = labels[annotationGitBranch]
		info.Author = labels[annotationGitAuthor]
		info.PRNumber = labels[annotationGitChangeNumber]
		info.TargetBranch = labels[annotationGitTargetBranch]
	}

	if annotations := ptr.Deref(item.Metadata.Annotations, nil); annotations != nil {
		info.PRURL = annotations[annotationGitChangeURL]
		info.CommitSHA = annotations[annotationGitCommitSHA]
	}

	// pipelineRef lives under spec, not status, so it is available even before the reconciler runs.
	if item.Spec != nil {
		if pipelineRef, ok := (*item.Spec)["pipelineRef"].(map[string]any); ok {
			info.Pipeline = stringVal(pipelineRef, "name")
		}
	}

	if item.Status != nil {
		status := *item.Status
		info.StartTime = stringVal(status, "startTime")

		if conditions, ok := status["conditions"].([]any); ok && len(conditions) > 0 {
			if cond, ok := conditions[0].(map[string]any); ok {
				switch stringVal(cond, "status") {
				case conditionStatusTrue:
					info.Status = StatusSucceeded
				case conditionStatusFalse:
					switch stringVal(cond, "reason") {
					case "PipelineRunTimeout":
						info.Status = StatusTimeout
					case "PipelineRunCancelled", "Cancelled":
						info.Status = StatusCancelled
					default:
						info.Status = StatusFailed
					}
				default:
					info.Status = StatusRunning
				}
			}
		}

		completionTimeStr := stringVal(status, "completionTime")
		if info.StartTime != "" {
			if completionTimeStr != "" {
				start, startErr := time.Parse(time.RFC3339, info.StartTime)
				end, endErr := time.Parse(time.RFC3339, completionTimeStr)
				if startErr == nil && endErr == nil {
					if d := end.Sub(start); d > 0 {
						info.Duration = formatDuration(d)
					}
				}
			} else if info.Status == StatusRunning {
				if start, err := time.Parse(time.RFC3339, info.StartTime); err == nil {
					if d := time.Since(start); d > 0 {
						info.Duration = formatDuration(d)
					}
				}
			}
		}
	}

	// Fall back to creationTimestamp when status.startTime is absent: covers both
	// "no status yet" and "status present but no startTime".
	if info.StartTime == "" {
		info.StartTime = ptr.Deref(item.Metadata.CreationTimestamp, "")
	}

	return info
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

	// Not all task names carry runPrefix (Tekton truncates at 63 chars),
	// so re-adding it after TrimPrefix is lossy — keep originals for log lookup.
	originalNames := make([]string, 0, len(taskRuns))

	var failedTaskRuns []restapi.TaskRun

	for i := range taskRuns {
		t := &taskRuns[i]
		ti := mapTaskRunInfo(t)
		originalNames = append(originalNames, ti.Name)
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

		g, ctx := errgroup.WithContext(ctx)
		results := make([]taskLogs, len(failedTaskRuns))

		for i, ft := range failedTaskRuns {
			g.Go(func() error {
				logResp, err := s.client.TektonResultsGetTaskRunLogsWithResponse(ctx,
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
			if logs, ok := logsByName[originalNames[i]]; ok {
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

	// Prefer summary.start_time (actual pipeline start) over create_time (when the
	// record was stored in Tekton Results, which is near completion). Using
	// create_time here inverts the order of mixed-source lists, since live K8s runs
	// use status.startTime and would sort alongside Results entries tagged with
	// completion timestamps.
	startTime := r.CreateTime
	if r.Summary != nil {
		if s := ptr.Deref(r.Summary.StartTime, ""); s != "" {
			startTime = s
		}
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
		StartTime:    startTime,
		Duration:     computeDuration(r, startTime),
		TargetBranch: resultAnnotation(r, annotationGitTargetBranch),
		CommitSHA:    resultAnnotation(r, annotationGitCommitSHA),
	}
}

func computeDuration(r *restapi.TektonResult, startStr string) string {
	if r.Summary == nil || string(r.Summary.Status) == resultStatusUnknown {
		return ""
	}

	endStr := ptr.Deref(r.Summary.EndTime, "")
	if endStr == "" {
		endStr = r.UpdateTime
	}

	if endStr == "" || startStr == "" {
		return ""
	}

	start, err := time.Parse(time.RFC3339, startStr)
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
		parts = append(parts, fmt.Sprintf("annotations[%q] == %q", annotationGitChangeNumber, strconv.Itoa(f.PRNumber)))
	}

	if m, ok := statusMappings[strings.ToLower(f.Status)]; ok {
		parts = append(parts, fmt.Sprintf("summary.status == %s", m.cel))
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
