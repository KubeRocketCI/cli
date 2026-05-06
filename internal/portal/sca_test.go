package portal

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const (
	testCodebase  = "my-service"
	testSCABranch = "main"
	qTrue         = "true"
	qFalse        = "false"
)

func TestSCAService_List_Success(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/sca/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("pageNumber") != "2" || q.Get("pageSize") != "25" || q.Get("searchTerm") != "pay" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		// Defaults forwarded: onlyRoot=true, excludeInactive=true.
		if q.Get("onlyRoot") != qTrue || q.Get("excludeInactive") != qTrue {
			t.Fatalf("expected default onlyRoot=true, excludeInactive=true, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"items": [
				{"uuid":"u1","name":"pay-api","version":"main","classifier":"APPLICATION","active":true,"riskScore":1.5,
				 "metrics": {"critical": 1, "high": 0, "medium": 2, "low": 3, "unassigned": 0, "vulnerabilities": 6}}
			],
			"totalCount": 42
		}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSCAService(client).List(context.Background(), SCAListParams{
		Page: 2, PageSize: 25, Search: "pay",
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if got.TotalCount != 42 || len(got.Items) != 1 || got.Items[0].Name != "pay-api" {
		t.Errorf("unexpected: %+v", got)
	}
	if got.Items[0].Metrics == nil || got.Items[0].Metrics.Critical != 1 || got.Items[0].Metrics.Vulnerabilities != 6 {
		t.Errorf("unexpected metrics: %+v", got.Items[0].Metrics)
	}
}

func TestSCAService_List_IncludesInactiveAndChildrenFlipsFlags(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("onlyRoot") != qFalse || q.Get("excludeInactive") != qFalse {
			t.Fatalf("expected onlyRoot=false, excludeInactive=false, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items": [], "totalCount": 0}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).List(context.Background(), SCAListParams{
		IncludeInactive: true, IncludeChildren: true,
	})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
}

func TestSCAService_List_EmptyResult_InitialisesItemsSlice(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items": [], "totalCount": 0}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSCAService(client).List(context.Background(), SCAListParams{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if got.Items == nil {
		t.Fatalf("Items must be non-nil so JSON emits [] not null")
	}
}

func TestSCAService_List_401(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).List(context.Background(), SCAListParams{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestSCAService_List_502_MapsToUpstreamUnavailable(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":{"code":"BAD_GATEWAY","message":"DT down"}}`))
	}
	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).List(context.Background(), SCAListParams{})
	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("want ErrUpstreamUnavailable, got %v", err)
	}
}

func TestSCAService_List_503_MapsToUpstreamUnavailable(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":{"code":"SERVICE_UNAVAILABLE","message":"not configured"}}`))
	}
	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).List(context.Background(), SCAListParams{})
	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("want ErrUpstreamUnavailable, got %v", err)
	}
}

func TestSCAService_Get_Success_ExplicitBranch(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/sca/get" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("codebase") != testCodebase || q.Get("branch") != testSCABranch {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "OK",
			"project": {"uuid":"u1","name":"my-service","version":"main","classifier":"APPLICATION","active":true,"riskScore":12.5},
			"metrics": {"critical": 3, "high": 5, "medium": 0, "low": 0, "unassigned": 0, "vulnerabilities": 8, "components": 100}
		}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSCAService(client).Get(context.Background(), testCodebase, testSCABranch)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Status != SCAStatusOK || got.Project == nil || got.Metrics == nil {
		t.Fatalf("unexpected: %+v", got)
	}
	if got.Metrics.Critical != 3 || got.Metrics.Components != 100 {
		t.Errorf("unexpected metrics: %+v", got.Metrics)
	}
}

func TestSCAService_Get_OmitsBranchWhenEmpty(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("branch"); got != "" {
			t.Fatalf("branch should not be set, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"NONE"}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSCAService(client).Get(context.Background(), testCodebase, "")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Status != SCAStatusNone {
		t.Errorf("want NONE, got %s", got.Status)
	}
	if got.Project != nil || got.Metrics != nil {
		t.Errorf("NONE payload must leave project/metrics nil, got %+v", got)
	}
}

func TestSCAService_Get_404_CodebaseNotFound_WithExplicitBranch(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"reason":"codebase_not_found","message":"Codebase 'nope' not found"}`))
	}
	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).Get(context.Background(), "nope", "main")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want wrap of ErrNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "project nope not found") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestSCAService_Get_404_DefaultBranchMissing_DistinguishedInMessage(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"reason":"default_branch_missing","message":"Codebase 'foo' has no spec.defaultBranch configured"}`))
	}
	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).Get(context.Background(), "foo", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "has no spec.defaultBranch") {
		t.Errorf("expected default-branch-missing hint, got %v", err)
	}
}

func TestSCAService_Get_404_CodebaseNotFound_NoBranch_GenericHint(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"reason":"codebase_not_found","message":"Codebase 'foo' not found"}`))
	}
	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).Get(context.Background(), "foo", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "use 'krci sca list --search=foo'") {
		t.Errorf("expected discover hint, got %v", err)
	}
}

func TestSCAService_Components_Success_ForwardsFilters(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("codebase") != testCodebase || q.Get("branch") != testSCABranch {
			t.Fatalf("unexpected scope: %s", r.URL.RawQuery)
		}
		if q.Get("onlyOutdated") != qTrue || q.Get("onlyDirect") != qTrue {
			t.Fatalf("expected passthrough of filters: %s", r.URL.RawQuery)
		}
		if q.Get("pageNumber") != "1" || q.Get("pageSize") != "50" {
			t.Fatalf("expected pagination: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "OK",
			"items": [
				{"uuid":"c1","name":"log4j-core","version":"2.11.2","latestVersion":"2.24.0","outdated":true,
				 "metrics": {"critical": 1, "high": 0, "medium": 0, "low": 0, "unassigned": 0, "vulnerabilities": 1}}
			],
			"totalCount": 120
		}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSCAService(client).Components(context.Background(), SCAComponentsParams{
		Codebase: testCodebase, Branch: testSCABranch, Page: 1, PageSize: 50,
		OnlyOutdated: true, OnlyDirect: true,
	})
	if err != nil {
		t.Fatalf("Components error: %v", err)
	}
	if got.Status != SCAStatusOK || got.TotalCount != 120 || !got.Items[0].Outdated {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestSCAService_Components_OmitsOptionalFiltersWhenFalse(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("onlyOutdated") != "" || q.Get("onlyDirect") != "" {
			t.Fatalf("filters should be omitted when false, got %s", r.URL.RawQuery)
		}
		if _, ok := q["severity"]; ok {
			t.Fatalf("severity should be omitted when not set, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"OK","items":[],"totalCount":0}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).Components(context.Background(), SCAComponentsParams{
		Codebase: testCodebase,
	})
	if err != nil {
		t.Fatalf("Components error: %v", err)
	}
}

func TestSCAService_Components_SendsSeverityCSVWhenProvided(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("severity"); got != "CRITICAL,HIGH" {
			t.Fatalf("expected severity=CRITICAL,HIGH; got %q (raw=%s)", got, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"OK","items":[],"totalCount":0}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).Components(context.Background(), SCAComponentsParams{
		Codebase: testCodebase,
		Severity: []string{"CRITICAL", "HIGH"},
	})
	if err != nil {
		t.Fatalf("Components error: %v", err)
	}
}

func TestSCAService_Components_TruncatedFieldPropagates(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"OK","items":[],"totalCount":0,"truncated":true}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSCAService(client).Components(context.Background(), SCAComponentsParams{
		Codebase: testCodebase,
		Severity: []string{"HIGH"},
	})
	if err != nil {
		t.Fatalf("Components error: %v", err)
	}
	if !got.Truncated {
		t.Errorf("expected Truncated=true; got %+v", got)
	}
}

func TestSCAService_Components_NONE_InitialisesItemsSlice(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"NONE","items":[],"totalCount":0}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSCAService(client).Components(context.Background(), SCAComponentsParams{
		Codebase: testCodebase,
	})
	if err != nil {
		t.Fatalf("Components error: %v", err)
	}
	if got.Status != SCAStatusNone || got.Items == nil {
		t.Errorf("want NONE with non-nil Items, got %+v", got)
	}
}

func TestSCAService_Findings_Success_PassesSourceAndSuppressed(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("suppressed") != qTrue || q.Get("source") != "NVD" {
			t.Fatalf("expected suppressed=true, source=NVD, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "OK",
			"items": [{
				"component": {"uuid":"c1","name":"log4j-core","version":"2.11.2"},
				"vulnerability": {"vulnId":"CVE-2021-44228","source":"NVD","severity":"CRITICAL","cvssV3BaseScore":10.0},
				"analysis": {"state":"NOT_SET","isSuppressed":false},
				"attribution": {"analyzerIdentity":"OSSINDEX_ANALYZER","attributedOn":1713456000000}
			}],
			"truncated": false
		}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSCAService(client).Findings(context.Background(), SCAFindingsParams{
		Codebase: testCodebase, IncludeSuppressed: true, Source: "NVD",
	})
	if err != nil {
		t.Fatalf("Findings error: %v", err)
	}
	if got.Status != SCAStatusOK || len(got.Items) != 1 || got.Items[0].Vulnerability.VulnID != "CVE-2021-44228" {
		t.Errorf("unexpected: %+v", got)
	}
	if got.Truncated {
		t.Error("Truncated must be false")
	}
}

func TestSCAService_Findings_DefaultSuppressedIsFalse(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("suppressed") != qFalse {
			t.Fatalf("expected suppressed=false by default, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"OK","items":[],"truncated":false}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).Findings(context.Background(), SCAFindingsParams{Codebase: testCodebase})
	if err != nil {
		t.Fatalf("Findings error: %v", err)
	}
}

func TestSCAService_Findings_TruncatedFlagPropagates(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"OK","items":[],"truncated":true}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	got, err := NewSCAService(client).Findings(context.Background(), SCAFindingsParams{Codebase: testCodebase})
	if err != nil {
		t.Fatalf("Findings error: %v", err)
	}
	if !got.Truncated {
		t.Error("Truncated must be true")
	}
}

func TestSCAService_Findings_MalformedBody_Returns5xx(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"code":"INTERNAL_SERVER_ERROR","message":"oops"}}`))
	}

	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).Findings(context.Background(), SCAFindingsParams{Codebase: testCodebase})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected HTTP 500 error, got %v", err)
	}
}

func TestSCAService_Get_404_DefaultBranchMissing_CaseInsensitive(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"reason":"DEFAULT_BRANCH_MISSING","message":"upper-case sentinel"}`))
	}
	client, closer := newTestClient(t, handler)
	defer closer()

	_, err := NewSCAService(client).Get(context.Background(), "foo", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "has no spec.defaultBranch") {
		t.Errorf("expected default-branch hint regardless of case, got %v", err)
	}
}
