package portal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

// QualityGateStatus is the SonarQube quality-gate outcome. An unset gate
// (project never analyzed) maps to QualityGateNone.
type QualityGateStatus string

const (
	QualityGateOK    QualityGateStatus = "OK"
	QualityGateWarn  QualityGateStatus = "WARN"
	QualityGateError QualityGateStatus = "ERROR"
	QualityGateNone  QualityGateStatus = "NONE"
)

// SonarProject is one entry in `krci sonar list`.
// `Qualifier` is an internal SonarQube discriminator (`TRK` for projects); we
// don't surface it. `Visibility` / `LastAnalysisDate` / `Revision` are best-effort
// optional fields — SonarQube's `/api/components/search` (the default non-admin
// endpoint) omits them, so they are typically absent.
type SonarProject struct {
	Key               string            `json:"key"`
	Name              string            `json:"name"`
	Visibility        string            `json:"visibility,omitempty"`
	LastAnalysisDate  string            `json:"lastAnalysisDate,omitempty"`
	Revision          string            `json:"revision,omitempty"`
	QualityGateStatus QualityGateStatus `json:"qualityGateStatus,omitempty"`
}

// SonarPaging is shared across paginated responses.
type SonarPaging struct {
	PageIndex int `json:"pageIndex"`
	PageSize  int `json:"pageSize"`
	Total     int `json:"total"`
}

// SonarProjectList is the response for `krci sonar list`.
type SonarProjectList struct {
	Projects []SonarProject `json:"projects"`
	Paging   SonarPaging    `json:"paging"`
}

// SonarProjectDetail is the response for `krci sonar get <project>`.
// All user-facing fields are always present (empty string when unknown) so
// downstream scripts can safely `jq .data.revision` without a `has` guard.
// `Qualifier` (SonarQube's internal `TRK` discriminator) is intentionally hidden.
type SonarProjectDetail struct {
	Key               string            `json:"key"`
	Name              string            `json:"name"`
	Visibility        string            `json:"visibility"`
	LastAnalysisDate  string            `json:"lastAnalysisDate"`
	Revision          string            `json:"revision"`
	QualityGateStatus QualityGateStatus `json:"qualityGateStatus"`
	Measures          map[string]string `json:"measures"`
}

// SonarGateCondition is one row in the gate conditions table.
type SonarGateCondition struct {
	MetricKey      string            `json:"metricKey"`
	Comparator     string            `json:"comparator,omitempty"`
	ErrorThreshold string            `json:"errorThreshold,omitempty"`
	ActualValue    string            `json:"actualValue,omitempty"`
	Status         QualityGateStatus `json:"status"`
}

// SonarGate is the response for `krci sonar gate <project>`.
type SonarGate struct {
	ProjectStatus SonarGateProjectStatus `json:"projectStatus"`
}

// SonarGateProjectStatus carries the overall + per-condition gate state.
// `Conditions` is always serialized as `[]` (never null) so scripting
// consumers can always range over it.
type SonarGateProjectStatus struct {
	Status            QualityGateStatus    `json:"status"`
	Conditions        []SonarGateCondition `json:"conditions"`
	IgnoredConditions bool                 `json:"ignoredConditions,omitempty"`
}

// SonarIssue is one row in `krci sonar issues <project>`.
type SonarIssue struct {
	Key          string   `json:"key"`
	Rule         string   `json:"rule"`
	Severity     string   `json:"severity"`
	Type         string   `json:"type"`
	Status       string   `json:"status"`
	Component    string   `json:"component"`
	Project      string   `json:"project"`
	Line         int      `json:"line,omitempty"`
	Message      string   `json:"message"`
	Effort       string   `json:"effort,omitempty"`
	Debt         string   `json:"debt,omitempty"`
	Author       string   `json:"author,omitempty"`
	CreationDate string   `json:"creationDate"`
	UpdateDate   string   `json:"updateDate,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// SonarIssueList is the response for `krci sonar issues <project>`.
// SonarQube's raw response duplicates pagination via both `total`/`p`/`ps`
// and a `paging` object; we only surface the structured `Paging` form.
type SonarIssueList struct {
	Paging SonarPaging  `json:"paging"`
	Issues []SonarIssue `json:"issues"`
}

// SonarListParams carries the CLI-validated inputs for `krci sonar list`.
type SonarListParams struct {
	Page       int
	PageSize   int
	SearchTerm string
}

// SonarIssuesParams carries the CLI-validated inputs for `krci sonar issues`.
// Types / Severities / Statuses are the already-validated enum values; the
// service joins them with commas when building the restapi query string.
// Resolved and Asc are `*bool`: nil means "let SonarQube apply its default",
// non-nil forwards the explicit value. PullRequest / Branch follow the same
// mutex rule as SonarScope.
type SonarIssuesParams struct {
	ProjectKey  string
	PullRequest string
	Branch      string
	Types       []string
	Severities  []string
	Statuses    []string
	Resolved    *bool
	Sort        string
	Asc         *bool
	Page        int
	PageSize    int
}

// SonarService wraps the generated restapi SonarList/SonarGet/SonarGate/SonarIssues
// methods and returns decoupled domain structs.
type SonarService struct {
	client *restapi.ClientWithResponses
}

// NewSonarService creates a SonarService over the generated portal client.
func NewSonarService(client *restapi.ClientWithResponses) *SonarService {
	return &SonarService{client: client}
}

// List calls `GET /rest/v1/sonar/list`.
func (s *SonarService) List(ctx context.Context, params SonarListParams) (*SonarProjectList, error) {
	p := &restapi.SonarListParams{}
	if params.Page > 0 {
		p.Page = ptr.To(params.Page)
	}
	if params.PageSize > 0 {
		p.PageSize = ptr.To(params.PageSize)
	}
	if params.SearchTerm != "" {
		p.SearchTerm = ptr.To(params.SearchTerm)
	}

	resp, err := s.client.SonarListWithResponse(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calling sonar list: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, err
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("portal returned empty sonar list response")
	}

	return mapSonarProjectList(resp.JSON200), nil
}

// SonarScope narrows a Sonar query to a pull request OR a branch. The two
// fields are mutually exclusive at the SonarQube API layer; callers validate
// that invariant before handing the struct to the service.
type SonarScope struct {
	PullRequest string
	Branch      string
}

// scopeQueryParams maps SonarScope to the (pullRequest, branch) pointer pair
// that every generated sonar params struct exposes. Empty scope fields are
// forwarded as nil so the portal omits the query parameter entirely.
func scopeQueryParams(s SonarScope) (pullRequest, branch *string) {
	if s.PullRequest != "" {
		pullRequest = ptr.To(s.PullRequest)
	}

	if s.Branch != "" {
		branch = ptr.To(s.Branch)
	}

	return pullRequest, branch
}

// Get calls `GET /rest/v1/sonar/get`. When a scope is supplied and upstream
// returns 404, the returned error message distinguishes between
// "pull request <id> not found" and "branch <name> not found".
func (s *SonarService) Get(ctx context.Context, project string, scope SonarScope) (*SonarProjectDetail, error) {
	p := &restapi.SonarGetParams{ProjectKey: project}
	p.PullRequest, p.Branch = scopeQueryParams(scope)

	resp, err := s.client.SonarGetWithResponse(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calling sonar get: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, sonarNotFoundErr(err, project, scope)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("portal returned empty sonar get response")
	}

	return mapSonarProjectDetail(resp.JSON200), nil
}

// Gate calls `GET /rest/v1/sonar/gate`.
func (s *SonarService) Gate(ctx context.Context, project string, scope SonarScope) (*SonarGate, error) {
	p := &restapi.SonarGateParams{ProjectKey: project}
	p.PullRequest, p.Branch = scopeQueryParams(scope)

	resp, err := s.client.SonarGateWithResponse(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calling sonar gate: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, sonarNotFoundErr(err, project, scope)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("portal returned empty sonar gate response")
	}

	return mapSonarGate(resp.JSON200), nil
}

// Issues calls `GET /rest/v1/sonar/issues`.
func (s *SonarService) Issues(ctx context.Context, params SonarIssuesParams) (*SonarIssueList, error) {
	scope := SonarScope{PullRequest: params.PullRequest, Branch: params.Branch}
	p := &restapi.SonarIssuesParams{ProjectKey: params.ProjectKey}
	p.PullRequest, p.Branch = scopeQueryParams(scope)

	if len(params.Types) > 0 {
		p.Types = ptr.To(strings.Join(params.Types, ","))
	}
	if len(params.Severities) > 0 {
		p.Severities = ptr.To(strings.Join(params.Severities, ","))
	}
	if len(params.Statuses) > 0 {
		p.Statuses = ptr.To(strings.Join(params.Statuses, ","))
	}
	if params.Resolved != nil {
		v := restapi.SonarIssuesParamsResolvedFalse
		if *params.Resolved {
			v = restapi.SonarIssuesParamsResolvedTrue
		}
		p.Resolved = &v
	}
	if params.Sort != "" {
		p.S = ptr.To(params.Sort)
	}
	if params.Asc != nil {
		v := restapi.False
		if *params.Asc {
			v = restapi.True
		}
		p.Asc = &v
	}
	if params.Page > 0 {
		p.P = ptr.To(params.Page)
	}
	if params.PageSize > 0 {
		p.Ps = ptr.To(params.PageSize)
	}

	resp, err := s.client.SonarIssuesWithResponse(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calling sonar issues: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, sonarNotFoundErr(err, params.ProjectKey, scope)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("portal returned empty sonar issues response")
	}

	return mapSonarIssueList(resp.JSON200), nil
}

// sonarNotFoundErr disambiguates "project not found" from scope-specific
// 404s (pull request vs. branch) when upstream returns 404. The returned
// error wraps ErrNotFound so callers can still match with errors.Is.
func sonarNotFoundErr(err error, project string, scope SonarScope) error {
	if !errors.Is(err, ErrNotFound) {
		return err
	}

	if scope.PullRequest != "" {
		return fmt.Errorf("pull request %s not found: %w", scope.PullRequest, ErrNotFound)
	}

	if scope.Branch != "" {
		return fmt.Errorf("branch %s not found: %w", scope.Branch, ErrNotFound)
	}

	return fmt.Errorf("project %s not found: %w", project, ErrNotFound)
}

// ---------------------------------------------------------------------------
// Mappers — isolate generated types behind the service boundary.
// ---------------------------------------------------------------------------

func mapSonarProjectList(src *restapi.SonarProjectList) *SonarProjectList {
	out := &SonarProjectList{
		Projects: make([]SonarProject, 0, len(src.Projects)),
		Paging:   SonarPaging(src.Paging),
	}

	for i := range src.Projects {
		out.Projects = append(out.Projects, mapSonarProject(&src.Projects[i]))
	}

	return out
}

func mapSonarProject(src *restapi.SonarProject) SonarProject {
	return SonarProject{
		Key:               src.Key,
		Name:              src.Name,
		LastAnalysisDate:  ptr.Deref(src.LastAnalysisDate, ""),
		Revision:          ptr.Deref(src.Revision, ""),
		Visibility:        ptr.Deref(src.Visibility, ""),
		QualityGateStatus: qualityGateStatus(src.QualityGateStatus),
	}
}

func mapSonarProjectDetail(src *restapi.SonarProjectDetail) *SonarProjectDetail {
	// Take ownership of the generated map directly — the response is dropped
	// after mapping, so aliasing is safe. Always surface a non-nil map so
	// `get -o json` renders `"measures": {}` instead of `null`.
	measures := map[string]string{}
	if src.Measures != nil {
		measures = *src.Measures
	}

	return &SonarProjectDetail{
		Key:               src.Key,
		Name:              src.Name,
		LastAnalysisDate:  ptr.Deref(src.LastAnalysisDate, ""),
		Revision:          ptr.Deref(src.Revision, ""),
		Visibility:        ptr.Deref(src.Visibility, ""),
		QualityGateStatus: qualityGateStatus(src.QualityGateStatus),
		Measures:          measures,
	}
}

// qualityGateStatus normalizes the two generated QualityGateStatus enum types
// (SonarProject and SonarProjectDetail carry different named types with
// identical values) into the domain QualityGateStatus, treating nil as "".
func qualityGateStatus[T ~string](p *T) QualityGateStatus {
	if p == nil {
		return ""
	}

	return QualityGateStatus(*p)
}

func mapSonarGate(src *restapi.SonarGate) *SonarGate {
	g := &SonarGate{
		ProjectStatus: SonarGateProjectStatus{
			Status: QualityGateStatus(src.ProjectStatus.Status),
			// Render `"conditions": []` instead of `null` for projects with no
			// analyses so scripting consumers can always range over it.
			Conditions: []SonarGateCondition{},
		},
	}

	if src.ProjectStatus.IgnoredConditions != nil {
		g.ProjectStatus.IgnoredConditions = *src.ProjectStatus.IgnoredConditions
	}

	if src.ProjectStatus.Conditions != nil {
		for _, c := range *src.ProjectStatus.Conditions {
			g.ProjectStatus.Conditions = append(g.ProjectStatus.Conditions, SonarGateCondition{
				MetricKey:      c.MetricKey,
				Comparator:     ptr.Deref(c.Comparator, ""),
				ErrorThreshold: ptr.Deref(c.ErrorThreshold, ""),
				ActualValue:    ptr.Deref(c.ActualValue, ""),
				Status:         QualityGateStatus(c.Status),
			})
		}
	}

	return g
}

func mapSonarIssueList(src *restapi.SonarIssueList) *SonarIssueList {
	out := &SonarIssueList{
		Paging: SonarPaging(src.Paging),
		Issues: make([]SonarIssue, 0, len(src.Issues)),
	}

	for _, i := range src.Issues {
		issue := SonarIssue{
			Key:          i.Key,
			Rule:         i.Rule,
			Severity:     string(i.Severity),
			Type:         string(i.Type),
			Status:       string(i.Status),
			Component:    i.Component,
			Project:      i.Project,
			Line:         ptr.Deref(i.Line, 0),
			Message:      i.Message,
			Effort:       ptr.Deref(i.Effort, ""),
			Debt:         ptr.Deref(i.Debt, ""),
			Author:       ptr.Deref(i.Author, ""),
			CreationDate: i.CreationDate,
			UpdateDate:   ptr.Deref(i.UpdateDate, ""),
		}
		if i.Tags != nil {
			issue.Tags = *i.Tags
		}
		out.Issues = append(out.Issues, issue)
	}

	return out
}
