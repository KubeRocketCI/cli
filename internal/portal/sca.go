package portal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

// SCAStatus is the Dep-Track binding state for a (codebase, branch) pair.
// OK means a Dep-Track project exists; NONE means the codebase has no
// Dep-Track project for the requested branch — clients emit an empty payload
// and exit 0 in that case.
type SCAStatus string

const (
	SCAStatusOK   SCAStatus = "OK"
	SCAStatusNone SCAStatus = "NONE"
)

// SCAMetrics carries the vulnerability-count rollup for a project or component.
type SCAMetrics struct {
	Critical             int `json:"critical"`
	High                 int `json:"high"`
	Medium               int `json:"medium"`
	Low                  int `json:"low"`
	Unassigned           int `json:"unassigned"`
	Vulnerabilities      int `json:"vulnerabilities"`
	Components           int `json:"components,omitempty"`
	VulnerableComponents int `json:"vulnerableComponents,omitempty"`
}

// SCAProject is one row in `krci sca list` / the `project` member of `krci sca get`.
type SCAProject struct {
	UUID                      string      `json:"uuid"`
	Name                      string      `json:"name"`
	Version                   string      `json:"version"`
	Classifier                string      `json:"classifier,omitempty"`
	Active                    bool        `json:"active,omitempty"`
	IsLatest                  bool        `json:"isLatest,omitempty"`
	LastBomImport             int64       `json:"lastBomImport,omitempty"`
	LastBomImportFormat       string      `json:"lastBomImportFormat,omitempty"`
	LastVulnerabilityAnalysis int64       `json:"lastVulnerabilityAnalysis,omitempty"`
	RiskScore                 float32     `json:"riskScore,omitempty"`
	Metrics                   *SCAMetrics `json:"metrics,omitempty"`
}

// SCAProjectList is the response for `krci sca list`.
type SCAProjectList struct {
	Items      []SCAProject `json:"items"`
	TotalCount int          `json:"totalCount"`
}

// SCAProjectDetail is the response for `krci sca get <codebase>`.
// When Status is NONE, Project and Metrics are nil.
type SCAProjectDetail struct {
	Status  SCAStatus   `json:"status"`
	Project *SCAProject `json:"project,omitempty"`
	Metrics *SCAMetrics `json:"metrics,omitempty"`
}

// SCAComponent is one row in `krci sca components`.
type SCAComponent struct {
	UUID          string      `json:"uuid"`
	Name          string      `json:"name"`
	Version       string      `json:"version"`
	LatestVersion string      `json:"latestVersion,omitempty"`
	Outdated      bool        `json:"outdated,omitempty"`
	Group         string      `json:"group,omitempty"`
	License       string      `json:"license,omitempty"`
	IsInternal    bool        `json:"isInternal,omitempty"`
	RiskScore     float32     `json:"riskScore,omitempty"`
	Metrics       *SCAMetrics `json:"metrics,omitempty"`
}

// SCAComponentList is the response for `krci sca components <codebase>`.
// Truncated signals that items and totalCount reflect an incomplete view
// because the Portal could not examine every dependency before paginating.
type SCAComponentList struct {
	Status     SCAStatus      `json:"status"`
	Items      []SCAComponent `json:"items"`
	TotalCount int            `json:"totalCount"`
	Truncated  bool           `json:"truncated"`
}

// SCAFindingComponent is the component side of one finding row.
type SCAFindingComponent struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Group   string `json:"group,omitempty"`
}

// SCAFindingVulnerability is the vulnerability side of one finding row.
type SCAFindingVulnerability struct {
	VulnID          string  `json:"vulnId"`
	Source          string  `json:"source"`
	Severity        string  `json:"severity"`
	CvssV3BaseScore float32 `json:"cvssV3BaseScore,omitempty"`
	CvssV2BaseScore float32 `json:"cvssV2BaseScore,omitempty"`
}

// SCAFindingAnalysis is the analysis side of one finding row.
type SCAFindingAnalysis struct {
	State        string `json:"state"`
	IsSuppressed bool   `json:"isSuppressed"`
}

// SCAFindingAttribution is the attribution side of one finding row.
type SCAFindingAttribution struct {
	AnalyzerIdentity string `json:"analyzerIdentity,omitempty"`
	AttributedOn     int64  `json:"attributedOn,omitempty"`
}

// SCAFinding is one row in `krci sca findings`.
type SCAFinding struct {
	Component     SCAFindingComponent     `json:"component"`
	Vulnerability SCAFindingVulnerability `json:"vulnerability"`
	Analysis      SCAFindingAnalysis      `json:"analysis"`
	Attribution   SCAFindingAttribution   `json:"attribution,omitempty"`
}

// SCAFindingList is the response for `krci sca findings <codebase>`.
type SCAFindingList struct {
	Status    SCAStatus    `json:"status"`
	Items     []SCAFinding `json:"items"`
	Truncated bool         `json:"truncated"`
}

// SCAListParams carries the CLI-validated inputs for `krci sca list`.
type SCAListParams struct {
	Page            int
	PageSize        int
	Search          string
	IncludeInactive bool
	IncludeChildren bool
}

// SCAComponentsParams carries the CLI-validated inputs for `krci sca components`.
// Severity holds the canonical upper-case Dep-Track severity set to filter by;
// empty slice means no filter.
type SCAComponentsParams struct {
	Codebase     string
	Branch       string
	Page         int
	PageSize     int
	OnlyOutdated bool
	OnlyDirect   bool
	Severity     []string
}

// SCAFindingsParams carries the CLI-validated inputs for `krci sca findings`.
type SCAFindingsParams struct {
	Codebase          string
	Branch            string
	IncludeSuppressed bool
	Source            string
}

// SCAService wraps the generated restapi ScaList/ScaGet/ScaComponents/ScaFindings
// methods and returns decoupled domain structs.
type SCAService struct {
	client *restapi.ClientWithResponses
}

// NewSCAService creates an SCAService over the generated portal client.
func NewSCAService(client *restapi.ClientWithResponses) *SCAService {
	return &SCAService{client: client}
}

// checkSCAResponse extends checkResponse with 502/503 mapping used only by the
// sca endpoints — Portal translates Dep-Track-reachability failures and
// unconfigured integration into 502/503 so the CLI can distinguish those from
// a generic 500.
func checkSCAResponse(statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable:
		return fmt.Errorf("%w: %s", ErrUpstreamUnavailable, truncateBody(body))
	}

	return checkResponse(statusCode, body)
}

// scaNotFoundError wraps ErrNotFound with a user-facing message. Unlike a
// plain fmt.Errorf(...: %w, ErrNotFound), its Error() omits the sentinel
// "resource not found" suffix so the CLI emits only the rich disambiguation
// message. errors.Is(err, ErrNotFound) still matches via Unwrap.
type scaNotFoundError struct{ msg string }

func (e *scaNotFoundError) Error() string { return e.msg }
func (e *scaNotFoundError) Unwrap() error { return ErrNotFound }

// scaBranchNotFoundErr decodes the 404 body returned by the resolveBranch
// helper on the Portal side. The body distinguishes `codebase_not_found` vs
// `default_branch_missing`; the CLI surfaces a matching human-readable message
// wrapping ErrNotFound so callers can still match with errors.Is.
func scaBranchNotFoundErr(err error, body []byte, codebase, branch string) error {
	if !errors.Is(err, ErrNotFound) {
		return err
	}

	// With an explicit --branch the 404 can only mean the Codebase CR itself
	// is missing (Portal doesn't lookup default branch in that path). Without
	// --branch, the Portal may respond 404 for either the missing CR or
	// missing spec.defaultBranch. Inspect the raw body rather than the wrapped
	// error text so disambiguation is robust when body is absent.
	bodyLower := strings.ToLower(string(body))
	switch {
	case branch != "":
		return &scaNotFoundError{msg: fmt.Sprintf("codebase %s not found", codebase)}
	case strings.Contains(bodyLower, "default_branch_missing"):
		return &scaNotFoundError{msg: fmt.Sprintf(
			"codebase %s has no spec.defaultBranch configured — pass --branch explicitly", codebase)}
	default:
		return &scaNotFoundError{msg: fmt.Sprintf(
			"codebase %s not found — use 'krci sca list --search=%s' to find projects known to Dep-Track",
			codebase, codebase)}
	}
}

func scaListParamsOnlyRoot(b bool) *restapi.ScaListParamsOnlyRoot {
	v := restapi.ScaListParamsOnlyRootFalse
	if b {
		v = restapi.ScaListParamsOnlyRootTrue
	}
	return &v
}

func scaListParamsExcludeInactive(b bool) *restapi.ScaListParamsExcludeInactive {
	v := restapi.ScaListParamsExcludeInactiveFalse
	if b {
		v = restapi.ScaListParamsExcludeInactiveTrue
	}
	return &v
}

func scaComponentsOnlyOutdated(b bool) *restapi.ScaComponentsParamsOnlyOutdated {
	if !b {
		return nil
	}
	v := restapi.ScaComponentsParamsOnlyOutdatedTrue
	return &v
}

func scaComponentsOnlyDirect(b bool) *restapi.ScaComponentsParamsOnlyDirect {
	if !b {
		return nil
	}
	v := restapi.ScaComponentsParamsOnlyDirectTrue
	return &v
}

func scaFindingsSuppressed(b bool) *restapi.ScaFindingsParamsSuppressed {
	v := restapi.ScaFindingsParamsSuppressedFalse
	if b {
		v = restapi.ScaFindingsParamsSuppressedTrue
	}
	return &v
}

// List returns the paginated Dep-Track project listing.
func (s *SCAService) List(ctx context.Context, params SCAListParams) (*SCAProjectList, error) {
	p := &restapi.ScaListParams{}
	if params.Page > 0 {
		p.PageNumber = ptr.To(params.Page)
	}
	if params.PageSize > 0 {
		p.PageSize = ptr.To(params.PageSize)
	}
	if params.Search != "" {
		p.SearchTerm = ptr.To(params.Search)
	}
	// Defaults (match Portal UI): exclude inactive, only root.
	p.ExcludeInactive = scaListParamsExcludeInactive(!params.IncludeInactive)
	p.OnlyRoot = scaListParamsOnlyRoot(!params.IncludeChildren)

	resp, err := s.client.ScaListWithResponse(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calling sca list: %w", err)
	}

	if err := checkSCAResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, err
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("portal returned empty sca list response")
	}

	return mapSCAProjectList(resp.JSON200), nil
}

// Get returns the Dep-Track project overview for a codebase/branch pair.
// When no project is bound for the pair, Status is SCAStatusNone and Project
// / Metrics are nil.
func (s *SCAService) Get(ctx context.Context, codebase, branch string) (*SCAProjectDetail, error) {
	p := &restapi.ScaGetParams{Codebase: codebase}
	if branch != "" {
		p.Branch = ptr.To(branch)
	}

	resp, err := s.client.ScaGetWithResponse(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calling sca get: %w", err)
	}

	if err := checkSCAResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, scaBranchNotFoundErr(err, resp.Body, codebase, branch)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("portal returned empty sca get response")
	}

	return mapSCAProjectDetail(resp.JSON200), nil
}

// Components returns the paginated dependency list for a codebase/branch pair.
func (s *SCAService) Components(ctx context.Context, params SCAComponentsParams) (*SCAComponentList, error) {
	p := &restapi.ScaComponentsParams{Codebase: params.Codebase}
	if params.Branch != "" {
		p.Branch = ptr.To(params.Branch)
	}
	if params.Page > 0 {
		p.PageNumber = ptr.To(params.Page)
	}
	if params.PageSize > 0 {
		p.PageSize = ptr.To(params.PageSize)
	}
	p.OnlyOutdated = scaComponentsOnlyOutdated(params.OnlyOutdated)
	p.OnlyDirect = scaComponentsOnlyDirect(params.OnlyDirect)
	if len(params.Severity) > 0 {
		p.Severity = ptr.To(strings.Join(params.Severity, ","))
	}

	resp, err := s.client.ScaComponentsWithResponse(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calling sca components: %w", err)
	}

	if err := checkSCAResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, scaBranchNotFoundErr(err, resp.Body, params.Codebase, params.Branch)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("portal returned empty sca components response")
	}

	return mapSCAComponentList(resp.JSON200), nil
}

// Findings returns the vulnerability findings for a codebase/branch pair.
// The Portal caps the server-side result at 1000 rows; when exceeded,
// Truncated is true.
func (s *SCAService) Findings(ctx context.Context, params SCAFindingsParams) (*SCAFindingList, error) {
	p := &restapi.ScaFindingsParams{Codebase: params.Codebase}
	if params.Branch != "" {
		p.Branch = ptr.To(params.Branch)
	}
	p.Suppressed = scaFindingsSuppressed(params.IncludeSuppressed)
	if params.Source != "" {
		p.Source = ptr.To(params.Source)
	}

	resp, err := s.client.ScaFindingsWithResponse(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calling sca findings: %w", err)
	}

	if err := checkSCAResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, scaBranchNotFoundErr(err, resp.Body, params.Codebase, params.Branch)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("portal returned empty sca findings response")
	}

	return mapSCAFindingList(resp.JSON200), nil
}

// ---------------------------------------------------------------------------
// Mappers — isolate generated types behind the service boundary.
// ---------------------------------------------------------------------------

func mapSCAProjectList(src *restapi.SCAListResponse) *SCAProjectList {
	out := &SCAProjectList{
		Items:      make([]SCAProject, 0, len(src.Items)),
		TotalCount: src.TotalCount,
	}
	for i := range src.Items {
		out.Items = append(out.Items, mapSCAProject(&src.Items[i]))
	}
	return out
}

func mapSCAProject(src *restapi.SCAProject) SCAProject {
	return SCAProject{
		UUID:                      src.Uuid,
		Name:                      src.Name,
		Version:                   src.Version,
		Classifier:                ptr.Deref(src.Classifier, ""),
		Active:                    ptr.Deref(src.Active, false),
		IsLatest:                  ptr.Deref(src.IsLatest, false),
		LastBomImport:             ptr.Deref(src.LastBomImport, 0),
		LastBomImportFormat:       ptr.Deref(src.LastBomImportFormat, ""),
		LastVulnerabilityAnalysis: ptr.Deref(src.LastVulnerabilityAnalysis, 0),
		RiskScore:                 ptr.Deref(src.RiskScore, 0),
		Metrics:                   mapSCAMetrics(src.Metrics),
	}
}

func mapSCAProjectDetail(src *restapi.SCAGetResponse) *SCAProjectDetail {
	out := &SCAProjectDetail{
		Status: SCAStatus(src.Status),
	}
	if src.Project != nil {
		p := mapSCAProject(src.Project)
		out.Project = &p
	}
	if src.Metrics != nil {
		out.Metrics = mapSCAMetrics(src.Metrics)
	}
	return out
}

func mapSCAMetrics(src *restapi.SCAMetrics) *SCAMetrics {
	if src == nil {
		return nil
	}
	return &SCAMetrics{
		Critical:             ptr.Deref(src.Critical, 0),
		High:                 ptr.Deref(src.High, 0),
		Medium:               ptr.Deref(src.Medium, 0),
		Low:                  ptr.Deref(src.Low, 0),
		Unassigned:           ptr.Deref(src.Unassigned, 0),
		Vulnerabilities:      ptr.Deref(src.Vulnerabilities, 0),
		Components:           ptr.Deref(src.Components, 0),
		VulnerableComponents: ptr.Deref(src.VulnerableComponents, 0),
	}
}

func mapSCAComponentList(src *restapi.SCAComponentsResponse) *SCAComponentList {
	out := &SCAComponentList{
		Status:     SCAStatus(src.Status),
		Items:      make([]SCAComponent, 0, len(src.Items)),
		TotalCount: src.TotalCount,
		Truncated:  src.Truncated,
	}
	for i := range src.Items {
		out.Items = append(out.Items, mapSCAComponent(&src.Items[i]))
	}
	return out
}

func mapSCAComponent(src *restapi.SCAComponent) SCAComponent {
	return SCAComponent{
		UUID:          src.Uuid,
		Name:          src.Name,
		Version:       src.Version,
		LatestVersion: ptr.Deref(src.LatestVersion, ""),
		Outdated:      ptr.Deref(src.Outdated, false),
		Group:         ptr.Deref(src.Group, ""),
		License:       ptr.Deref(src.License, ""),
		IsInternal:    ptr.Deref(src.IsInternal, false),
		RiskScore:     ptr.Deref(src.RiskScore, 0),
		Metrics:       mapSCAMetrics(src.Metrics),
	}
}

func mapSCAFindingList(src *restapi.SCAFindingsResponse) *SCAFindingList {
	out := &SCAFindingList{
		Status:    SCAStatus(src.Status),
		Items:     make([]SCAFinding, 0, len(src.Items)),
		Truncated: src.Truncated,
	}
	for i := range src.Items {
		out.Items = append(out.Items, mapSCAFinding(&src.Items[i]))
	}
	return out
}

func mapSCAFinding(src *restapi.SCAFinding) SCAFinding {
	f := SCAFinding{
		Component: SCAFindingComponent{
			UUID:    src.Component.Uuid,
			Name:    src.Component.Name,
			Version: ptr.Deref(src.Component.Version, ""),
			Group:   ptr.Deref(src.Component.Group, ""),
		},
		Vulnerability: SCAFindingVulnerability{
			VulnID:          src.Vulnerability.VulnId,
			Source:          src.Vulnerability.Source,
			Severity:        string(src.Vulnerability.Severity),
			CvssV3BaseScore: ptr.Deref(src.Vulnerability.CvssV3BaseScore, 0),
			CvssV2BaseScore: ptr.Deref(src.Vulnerability.CvssV2BaseScore, 0),
		},
		Analysis: SCAFindingAnalysis{
			State:        src.Analysis.State,
			IsSuppressed: src.Analysis.IsSuppressed,
		},
	}
	if src.Attribution != nil {
		f.Attribution = SCAFindingAttribution{
			AnalyzerIdentity: ptr.Deref(src.Attribution.AnalyzerIdentity, ""),
			AttributedOn:     ptr.Deref(src.Attribution.AttributedOn, 0),
		}
	}
	return f
}
