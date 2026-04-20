package portal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

const testProject = "my-proj"

func newTestClient(t *testing.T, handler http.HandlerFunc) (*restapi.ClientWithResponses, func()) {
	t.Helper()

	srv := httptest.NewServer(handler)

	client, err := restapi.NewClientWithResponses(srv.URL + "/rest")
	if err != nil {
		srv.Close()
		t.Fatalf("NewClientWithResponses: %v", err)
	}

	return client, srv.Close
}

func TestSonarService_List_Success(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/sonar/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("pageSize") != "25" ||
			r.URL.Query().Get("searchTerm") != "pay" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"projects": [
				{"key":"pay-api","name":"pay-api","qualityGateStatus":"OK"}
			],
			"paging": {"pageIndex":2,"pageSize":25,"total":1}
		}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	svc := NewSonarService(client)

	got, err := svc.List(context.Background(), SonarListParams{Page: 2, PageSize: 25, SearchTerm: "pay"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(got.Projects) != 1 || got.Projects[0].Key != "pay-api" {
		t.Errorf("unexpected projects: %+v", got.Projects)
	}

	if got.Projects[0].QualityGateStatus != "OK" {
		t.Errorf("qualityGateStatus = %q", got.Projects[0].QualityGateStatus)
	}

	if got.Paging.Total != 1 {
		t.Errorf("paging.total = %d", got.Paging.Total)
	}
}

func TestSonarService_List_401(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).List(context.Background(), SonarListParams{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestSonarService_Get_Success_PassesPullRequest(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("projectKey"); got != testProject {
			t.Fatalf("projectKey = %q", got)
		}
		if got := r.URL.Query().Get("pullRequest"); got != "123" {
			t.Fatalf("pullRequest = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"key":"my-proj","name":"my-proj",
			"visibility":"private","lastAnalysisDate":"2026-04-18T00:00:00Z",
			"qualityGateStatus":"OK",
			"measures":{"bugs":"2","coverage":"84.2","reliability_rating":"1.0"}
		}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSonarService(client).Get(context.Background(), testProject, "123")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if got.Key != testProject || got.QualityGateStatus != "OK" {
		t.Errorf("unexpected detail: %+v", got)
	}

	if got.Measures["bugs"] != "2" || got.Measures["coverage"] != "84.2" {
		t.Errorf("unexpected measures: %+v", got.Measures)
	}
}

func TestSonarService_Get_Success_OmitsPullRequestWhenEmpty(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("pullRequest"); got != "" {
			t.Fatalf("pullRequest should not be set, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"key":"my-proj","name":"my-proj"}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Get(context.Background(), testProject, "")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
}

func TestSonarService_Get_404_ProjectNotFoundMessage(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Get(context.Background(), "nope", "")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "project nope not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSonarService_Get_404_PullRequestNotFoundMessage(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Get(context.Background(), testProject, "999")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "pull request 999 not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSonarService_Get_500(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"code":"INTERNAL_SERVER_ERROR","message":"oops"}}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Get(context.Background(), testProject, "")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSonarService_Gate_Success_NONE(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projectStatus":{"status":"NONE","conditions":[]}}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	g, err := NewSonarService(client).Gate(context.Background(), testProject, "")
	if err != nil {
		t.Fatalf("Gate error: %v", err)
	}

	if g.ProjectStatus.Status != "NONE" {
		t.Errorf("status = %q", g.ProjectStatus.Status)
	}
}

func TestSonarService_Gate_Success_ERROR_WithConditions(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pullRequest") != "123" {
			t.Fatalf("pullRequest missing")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projectStatus":{
			"status":"ERROR",
			"conditions":[
				{"metricKey":"new_coverage","comparator":"LT","errorThreshold":"80.0","actualValue":"61.4","status":"ERROR"}
			]
		}}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	g, err := NewSonarService(client).Gate(context.Background(), testProject, "123")
	if err != nil {
		t.Fatalf("Gate error: %v", err)
	}

	if g.ProjectStatus.Status != "ERROR" {
		t.Errorf("status = %q", g.ProjectStatus.Status)
	}
	if len(g.ProjectStatus.Conditions) != 1 || g.ProjectStatus.Conditions[0].MetricKey != "new_coverage" {
		t.Errorf("unexpected conditions: %+v", g.ProjectStatus.Conditions)
	}
}

func TestSonarService_Issues_Success_ForwardsFilters(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("projectKey") != testProject ||
			q.Get("severities") != "BLOCKER,CRITICAL" ||
			q.Get("types") != "BUG" ||
			q.Get("resolved") != "true" ||
			q.Get("p") != "2" || q.Get("ps") != "10" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"total":1,"p":2,"ps":10,
			"paging":{"pageIndex":2,"pageSize":10,"total":1},
			"issues":[{
				"key":"I1","rule":"ts:S1","severity":"BLOCKER","type":"BUG","status":"OPEN",
				"component":"c","project":"my-proj","message":"m","creationDate":"2026-04-18T00:00:00Z"
			}]
		}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	list, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Severities: []string{"BLOCKER", "CRITICAL"},
		Types:      []string{"BUG"},
		Resolved:   ptr.To(true),
		Page:       2,
		PageSize:   10,
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}

	if list.Paging.Total != 1 || list.Issues[0].Key != "I1" {
		t.Errorf("unexpected result: %+v", list)
	}
}

func TestSonarService_Issues_404(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{ProjectKey: "nope"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "project nope not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// List — edge cases
// ---------------------------------------------------------------------------

func TestSonarService_List_NoPaginationParams(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") != "" || q.Get("pageSize") != "" || q.Get("searchTerm") != "" {
			t.Errorf("no pagination params expected, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projects":[{"key":"a","name":"A"}],"paging":{"pageIndex":1,"pageSize":10,"total":1}}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSonarService(client).List(context.Background(), SonarListParams{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(got.Projects) != 1 || got.Projects[0].Key != "a" {
		t.Errorf("unexpected projects: %+v", got.Projects)
	}
}

func TestSonarService_List_SearchTermForwarded(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("searchTerm") != "frontend" {
			t.Fatalf("expected searchTerm=frontend, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projects":[],"paging":{"pageIndex":1,"pageSize":10,"total":0}}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).List(context.Background(), SonarListParams{SearchTerm: "frontend"})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
}

func TestSonarService_List_500_MessagePassthrough(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`upstream database connection failed`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).List(context.Background(), SonarListParams{})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "upstream database connection failed") {
		t.Errorf("expected 500 body in error message, got: %v", err)
	}
}

func TestSonarService_List_EmptyJSON200(t *testing.T) {
	t.Parallel()

	// The generated client sets JSON200 to nil when Content-Type is not application/json.
	// Simulate by returning no Content-Type so the generated client leaves JSON200 nil.
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No Content-Type → JSON200 stays nil in the generated client.
		w.Write([]byte(`{}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).List(context.Background(), SonarListParams{})
	if err == nil {
		t.Fatal("expected error when JSON200 is nil")
	}

	if !strings.Contains(err.Error(), "portal returned empty sonar list response") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Get / Gate — 404 ErrNotFound wrapping (errors.Is must hold)
// ---------------------------------------------------------------------------

func TestSonarService_Get_404_ProjectNotFound_ErrorIs(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Get(context.Background(), "my-proj", "")
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) must be true; got: %v", err)
	}

	if !strings.Contains(err.Error(), "project my-proj not found") {
		t.Errorf("message must contain project name; got: %v", err)
	}
}

func TestSonarService_Get_404_PullRequest_ErrorIs(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Get(context.Background(), testProject, "123")
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) must be true; got: %v", err)
	}

	if !strings.Contains(err.Error(), "pull request 123 not found") {
		t.Errorf("message must contain pull request id; got: %v", err)
	}
}

func TestSonarService_Gate_404_ProjectNotFound(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Gate(context.Background(), "ghost-proj", "")
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) must be true; got: %v", err)
	}

	if !strings.Contains(err.Error(), "project ghost-proj not found") {
		t.Errorf("message must contain project name; got: %v", err)
	}
}

func TestSonarService_Gate_404_PullRequest_ErrorIs(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Gate(context.Background(), testProject, "123")
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) must be true; got: %v", err)
	}

	if !strings.Contains(err.Error(), "pull request 123 not found") {
		t.Errorf("message must contain pull request id; got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Issues — optional *bool and CSV-join forwarding
// ---------------------------------------------------------------------------

func TestSonarService_Issues_Resolved_False(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("resolved") != "false" {
			t.Fatalf("expected resolved=false, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Resolved:   ptr.To(false),
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

func TestSonarService_Issues_Resolved_Nil_Omitted(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.URL.Query()["resolved"]; present {
			t.Fatalf("resolved param must be absent when nil, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Resolved:   nil,
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

func TestSonarService_Issues_Asc_True(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("asc") != "true" {
			t.Fatalf("expected asc=true, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Asc:        ptr.To(true),
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

func TestSonarService_Issues_Asc_False(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("asc") != "false" {
			t.Fatalf("expected asc=false, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Asc:        ptr.To(false),
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

func TestSonarService_Issues_Asc_Nil_Omitted(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.URL.Query()["asc"]; present {
			t.Fatalf("asc param must be absent when nil, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Asc:        nil,
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

func TestSonarService_Issues_Types_CSVJoin(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("types") != "BUG,VULNERABILITY" {
			t.Fatalf("expected types=BUG,VULNERABILITY, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Types:      []string{"BUG", "VULNERABILITY"},
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

func TestSonarService_Issues_EmptyTypes_Omitted(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.URL.Query()["types"]; present {
			t.Fatalf("types param must be absent for empty slice, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Types:      []string{},
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

func TestSonarService_Issues_Statuses_CSVJoin(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("statuses") != "OPEN,CONFIRMED" {
			t.Fatalf("expected statuses=OPEN,CONFIRMED, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Statuses:   []string{"OPEN", "CONFIRMED"},
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

func TestSonarService_Issues_Sort_Forwarded(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("s") != "SEVERITY" {
			t.Fatalf("expected s=SEVERITY, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey: testProject,
		Sort:       "SEVERITY",
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

func TestSonarService_Issues_PullRequest_Forwarded(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pullRequest") != "42" {
			t.Fatalf("expected pullRequest=42, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"p":1,"ps":10,"paging":{"pageIndex":1,"pageSize":10,"total":0},"issues":[]}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{
		ProjectKey:  testProject,
		PullRequest: "42",
	})
	if err != nil {
		t.Fatalf("Issues error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mapper unit tests — called directly for high coverage-per-test ratio
// ---------------------------------------------------------------------------

func TestMapSonarProjectDetail_NilMeasures(t *testing.T) {
	t.Parallel()

	src := &restapi.SonarProjectDetail{
		Key:      "proj-1",
		Name:     "Project 1",
		Measures: nil,
	}

	got := mapSonarProjectDetail(src)

	if got.Measures == nil {
		t.Error("Measures must be non-nil (empty map) when source Measures is nil")
	}

	if len(got.Measures) != 0 {
		t.Errorf("expected empty map, got: %v", got.Measures)
	}
}

func TestMapSonarProjectDetail_NonNilMeasures(t *testing.T) {
	t.Parallel()

	m := map[string]string{"coverage": "80.5", "bugs": "3"}
	src := &restapi.SonarProjectDetail{
		Key:      "proj-2",
		Name:     "Project 2",
		Measures: &m,
	}

	got := mapSonarProjectDetail(src)

	if got.Measures == nil {
		t.Error("Measures must not be nil")
	}

	if got.Measures["coverage"] != "80.5" || got.Measures["bugs"] != "3" {
		t.Errorf("unexpected measures: %v", got.Measures)
	}
}

func TestMapSonarGate_NoConditions(t *testing.T) {
	t.Parallel()

	src := &restapi.SonarGate{
		ProjectStatus: struct {
			Conditions        *[]restapi.SonarGateCondition        `json:"conditions,omitempty"`
			IgnoredConditions *bool                                `json:"ignoredConditions,omitempty"`
			Status            restapi.SonarGateProjectStatusStatus `json:"status"`
		}{
			Status:     restapi.SonarGateProjectStatusStatusNONE,
			Conditions: nil,
		},
	}

	got := mapSonarGate(src)

	if got.ProjectStatus.Conditions == nil {
		t.Error("Conditions must be non-nil empty slice (never null) when source has no conditions")
	}

	if len(got.ProjectStatus.Conditions) != 0 {
		t.Errorf("expected empty slice, got %d conditions", len(got.ProjectStatus.Conditions))
	}
}

func TestMapSonarGate_IgnoredConditions(t *testing.T) {
	t.Parallel()

	ignored := true
	src := &restapi.SonarGate{
		ProjectStatus: struct {
			Conditions        *[]restapi.SonarGateCondition        `json:"conditions,omitempty"`
			IgnoredConditions *bool                                `json:"ignoredConditions,omitempty"`
			Status            restapi.SonarGateProjectStatusStatus `json:"status"`
		}{
			Status:            restapi.SonarGateProjectStatusStatusOK,
			IgnoredConditions: &ignored,
		},
	}

	got := mapSonarGate(src)

	if !got.ProjectStatus.IgnoredConditions {
		t.Error("IgnoredConditions must be true")
	}
}

func TestMapSonarIssueList_TagsNil(t *testing.T) {
	t.Parallel()

	src := &restapi.SonarIssueList{
		Paging: restapi.SonarPaging{PageIndex: 1, PageSize: 10, Total: 1},
		Issues: []restapi.SonarIssue{
			{
				Key:          "I1",
				Rule:         "rule1",
				Severity:     restapi.BLOCKER,
				Type:         restapi.BUG,
				Status:       restapi.OPEN,
				Component:    "comp",
				Project:      "proj",
				Message:      "msg",
				CreationDate: "2026-04-18T00:00:00Z",
				Tags:         nil,
			},
		},
	}

	got := mapSonarIssueList(src)

	if got.Issues[0].Tags != nil {
		t.Errorf("Tags must remain nil when source Tags is nil, got: %v", got.Issues[0].Tags)
	}
}

func TestMapSonarIssueList_TagsNonNil(t *testing.T) {
	t.Parallel()

	tags := []string{"security", "cwe"}
	src := &restapi.SonarIssueList{
		Paging: restapi.SonarPaging{PageIndex: 1, PageSize: 10, Total: 1},
		Issues: []restapi.SonarIssue{
			{
				Key:          "I2",
				Rule:         "rule2",
				Severity:     restapi.CRITICAL,
				Type:         restapi.VULNERABILITY,
				Status:       restapi.OPEN,
				Component:    "comp",
				Project:      "proj",
				Message:      "msg",
				CreationDate: "2026-04-18T00:00:00Z",
				Tags:         &tags,
			},
		},
	}

	got := mapSonarIssueList(src)

	if len(got.Issues[0].Tags) != 2 || got.Issues[0].Tags[0] != "security" || got.Issues[0].Tags[1] != "cwe" {
		t.Errorf("unexpected tags: %v", got.Issues[0].Tags)
	}
}

func TestQualityGateStatus_Nil(t *testing.T) {
	t.Parallel()

	var p *restapi.SonarProjectQualityGateStatus

	got := qualityGateStatus(p)

	if got != "" {
		t.Errorf("expected empty string for nil input, got %q", got)
	}
}

func TestQualityGateStatus_NonNil(t *testing.T) {
	t.Parallel()

	v := restapi.SonarProjectQualityGateStatusERROR

	got := qualityGateStatus(&v)

	if got != QualityGateError {
		t.Errorf("expected QualityGateError, got %q", got)
	}
}

func TestSonarNotFoundErr_NonNotFound_PassThrough(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("some other error")

	got := sonarNotFoundErr(sentinel, "proj", "")

	if got != sentinel {
		t.Errorf("expected same error instance to pass through, got: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Get / Gate / Issues — JSON200 == nil (200 response without application/json
// Content-Type leaves JSON200 nil in the generated client)
// ---------------------------------------------------------------------------

func TestSonarService_Get_EmptyJSON200(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		// No Content-Type → generated client leaves JSON200 nil.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Get(context.Background(), testProject, "")
	if err == nil {
		t.Fatal("expected error when JSON200 is nil")
	}

	if !strings.Contains(err.Error(), "portal returned empty sonar get response") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSonarService_Gate_EmptyJSON200(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		// No Content-Type → generated client leaves JSON200 nil.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Gate(context.Background(), testProject, "")
	if err == nil {
		t.Fatal("expected error when JSON200 is nil")
	}

	if !strings.Contains(err.Error(), "portal returned empty sonar gate response") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSonarService_Issues_EmptyJSON200(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		// No Content-Type → generated client leaves JSON200 nil.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSonarService(client).Issues(context.Background(), SonarIssuesParams{ProjectKey: testProject})
	if err == nil {
		t.Fatal("expected error when JSON200 is nil")
	}

	if !strings.Contains(err.Error(), "portal returned empty sonar issues response") {
		t.Errorf("unexpected error message: %v", err)
	}
}
