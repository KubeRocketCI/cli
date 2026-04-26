package portal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// envTestRecorder routes /rest/v1/resources/list calls to a list of typed
// responses keyed by Kind, plus optional /rest/v1/resources/get for the
// CDPipeline parent fetch in EnvService.Get.
//
//nolint:unused // false positives for fields populated through composite literals
type envTestRecorder struct {
	t            *testing.T
	listByKind   map[string]string // Kind → JSON response
	getByName    map[string]string // metadata.name → JSON response (for CDPipeline get)
	statusByKind map[string]int    // optional override per kind

	mu    sync.Mutex
	calls []envTestCall
}

type envTestCall struct {
	Path           string
	Kind           string
	LabelSelectors map[string]string
}

func (r *envTestRecorder) handler(w http.ResponseWriter, req *http.Request) {
	r.t.Helper()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.t.Fatalf("read body: %v", err)
	}

	var parsed struct {
		ResourceConfig struct {
			Kind string `json:"kind"`
		} `json:"resourceConfig"`
		Labels *map[string]string `json:"labels,omitempty"`
		Name   string             `json:"name,omitempty"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		r.t.Fatalf("unmarshal body: %v\nbody=%s", err, string(body))
	}

	call := envTestCall{Path: req.URL.Path, Kind: parsed.ResourceConfig.Kind}
	if parsed.Labels != nil {
		call.LabelSelectors = *parsed.Labels
	}
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()

	if status, ok := r.statusByKind[parsed.ResourceConfig.Kind]; ok && status != 0 {
		w.WriteHeader(status)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if strings.HasSuffix(req.URL.Path, "/v1/resources/get") {
		body, ok := r.getByName[parsed.Name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
		return
	}

	body2, ok := r.listByKind[parsed.ResourceConfig.Kind]
	if !ok {
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"List","items":[],"metadata":{}}`))
		return
	}
	_, _ = w.Write([]byte(body2))
}

func newEnvTestServer(t *testing.T, rec *envTestRecorder) (string, func()) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(rec.handler))

	return srv.URL, srv.Close
}

// newEnvServiceForTest wires up an EnvService against an httptest server.
func newEnvServiceForTest(t *testing.T, rec *envTestRecorder) (*EnvService, func()) {
	t.Helper()

	url, closer := newEnvTestServer(t, rec)

	client, err := restapi.NewClientWithResponses(url + "/rest")
	if err != nil {
		closer()
		t.Fatalf("new client: %v", err)
	}

	return NewEnvService(client, "in-cluster", "ns"), closer
}

// newDeploymentByProjectServiceForTest wires up a DeploymentByProjectService.
func newDeploymentByProjectServiceForTest(t *testing.T, rec *envTestRecorder) (*DeploymentByProjectService, func()) {
	t.Helper()

	url, closer := newEnvTestServer(t, rec)

	client, err := restapi.NewClientWithResponses(url + "/rest")
	if err != nil {
		closer()
		t.Fatalf("new client: %v", err)
	}

	return NewDeploymentByProjectService(client, "in-cluster", "ns"), closer
}

func TestEnvService_List_Default(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Stage": stageListJSON([]stageStub{
				{name: "my-pipeline-prod", deployment: "my-pipeline", env: "prod", cluster: "in-cluster", namespace: "my-pipeline-prod", trigger: "Manual", order: 2, status: "created"},
				{name: "my-pipeline-dev", deployment: "my-pipeline", env: "dev", cluster: "in-cluster", namespace: "my-pipeline-dev", trigger: "Auto", order: 0, status: "created"},
				{name: "other-pipe-dev", deployment: "other-pipe", env: "dev", cluster: "in-cluster", namespace: "other-pipe-dev", trigger: "Auto", order: 0, status: "in_progress"},
				{name: "my-pipeline-stage", deployment: "my-pipeline", env: "stage", cluster: "in-cluster", namespace: "my-pipeline-stage", trigger: "Manual", order: 1, status: "created"},
			}),
		},
	}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	rows, err := svc.List(context.Background(), EnvListFilters{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	want := []EnvSummary{
		{Deployment: "my-pipeline", Env: "dev", Cluster: "in-cluster", Namespace: "my-pipeline-dev", TriggerType: "Auto", Status: "created", Order: 0},
		{Deployment: "my-pipeline", Env: "stage", Cluster: "in-cluster", Namespace: "my-pipeline-stage", TriggerType: "Manual", Status: "created", Order: 1},
		{Deployment: "my-pipeline", Env: "prod", Cluster: "in-cluster", Namespace: "my-pipeline-prod", TriggerType: "Manual", Status: "created", Order: 2},
		{Deployment: "other-pipe", Env: "dev", Cluster: "in-cluster", Namespace: "other-pipe-dev", TriggerType: "Auto", Status: "in_progress", Order: 0},
	}

	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}

	for i, row := range rows {
		if row != want[i] {
			t.Errorf("row[%d] = %+v, want %+v", i, row, want[i])
		}
	}
}

func TestEnvService_List_FilteredByDeployment(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Stage": stageListJSON([]stageStub{
				{name: "my-pipeline-dev", deployment: "my-pipeline", env: "dev", cluster: "in-cluster", namespace: "my-pipeline-dev", trigger: "Auto", order: 0, status: "created"},
			}),
		},
	}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	rows, err := svc.List(context.Background(), EnvListFilters{Deployment: "my-pipeline"})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(rec.calls))
	}

	if got := rec.calls[0].LabelSelectors[labelCDPipelineName]; got != "my-pipeline" {
		t.Errorf("label %s = %q, want %q", labelCDPipelineName, got, "my-pipeline")
	}
}

func TestEnvService_List_ClusterFilter(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Stage": stageListJSON([]stageStub{
				{name: "a", deployment: "p", env: "dev", cluster: "in-cluster", order: 0, status: "created"},
				{name: "b", deployment: "p", env: "stage", cluster: "remote", order: 1, status: "created"},
			}),
		},
	}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	rows, err := svc.List(context.Background(), EnvListFilters{Cluster: "remote"})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(rows) != 1 || rows[0].Env != "stage" {
		t.Fatalf("expected 1 remote row, got %+v", rows)
	}
}

func TestEnvService_List_Empty(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{t: t, listByKind: map[string]string{}}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	rows, err := svc.List(context.Background(), EnvListFilters{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(rows) != 0 {
		t.Errorf("expected empty rows, got %+v", rows)
	}
}

func TestEnvService_List_Unauthorized(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{t: t, statusByKind: map[string]int{"Stage": http.StatusUnauthorized}}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	_, err := svc.List(context.Background(), EnvListFilters{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestEnvService_Get_DeploymentNotFound(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t:          t,
		listByKind: map[string]string{"Stage": stageListJSON(nil)},
		getByName:  map[string]string{}, // empty → 404
	}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	_, err := svc.Get(context.Background(), "nope", "dev")
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Errorf("expected ErrDeploymentNotFound, got %v", err)
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ErrDeploymentNotFound must wrap ErrNotFound, got %v", err)
	}
}

func TestEnvService_Get_EnvNotFound(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Stage": stageListJSON([]stageStub{
				{name: "my-pipeline-dev", deployment: "my-pipeline", env: "dev", order: 0},
			}),
		},
		getByName: map[string]string{
			"my-pipeline": cdPipelineGetJSON("my-pipeline", []string{"foo"}),
		},
	}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	_, err := svc.Get(context.Background(), "my-pipeline", "wat")
	if !errors.Is(err, ErrEnvNotFound) {
		t.Errorf("expected ErrEnvNotFound, got %v", err)
	}

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ErrEnvNotFound must wrap ErrNotFound, got %v", err)
	}
}

func TestEnvService_Get_ProjectsRegisteredButNotDeployed(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Stage": stageListJSON([]stageStub{
				{name: "my-pipeline-prod", deployment: "my-pipeline", env: "prod", cluster: "in-cluster", namespace: "my-pipeline-prod", trigger: "Manual", order: 2, status: "created"},
			}),
			"Application": applicationListJSON([]applicationStub{
				{appName: "foo", pipeline: "my-pipeline", stage: "prod", health: "Healthy", sync: "Synced", imageRepo: "registry/foo", imageTag: "1.2.0", imageDigest: "sha256:abc12345abc12345"},
				{appName: "bar", pipeline: "my-pipeline", stage: "prod", health: "Healthy", sync: "Synced", imageRepo: "registry/bar", imageTag: "2.0.1", externalURLs: []string{"https://bar.prod.example.com"}},
			}),
		},
		getByName: map[string]string{
			"my-pipeline": cdPipelineGetJSON("my-pipeline", []string{"foo", "bar", "baz"}),
		},
	}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	d, err := svc.Get(context.Background(), "my-pipeline", "prod")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if got := d.Env; got != "prod" {
		t.Errorf("env = %q, want %q", got, "prod")
	}

	if len(d.Projects) != 3 {
		t.Fatalf("got %d projects, want 3: %+v", len(d.Projects), d.Projects)
	}

	wantNames := []string{"bar", "baz", "foo"}
	for i, p := range d.Projects {
		if p.Name != wantNames[i] {
			t.Errorf("projects[%d].name = %q, want %q (sort by name asc)", i, p.Name, wantNames[i])
		}
	}

	// baz is registered but not deployed
	for _, p := range d.Projects {
		if p.Name == "baz" {
			if p.Status != nil || p.Sync != nil || p.Version != nil || p.ImageDigest != nil {
				t.Errorf("baz dynamic fields should be nil: %+v", p)
			}
			if len(p.IngressURLs) != 0 {
				t.Errorf("baz ingressUrls should be empty, got %v", p.IngressURLs)
			}
		}
	}

	// foo deployed: check fields
	for _, p := range d.Projects {
		if p.Name == "foo" {
			if p.Status == nil || *p.Status != "healthy" {
				t.Errorf("foo.status = %v, want pointer to 'healthy'", p.Status)
			}
			if p.Sync == nil || *p.Sync != "synced" {
				t.Errorf("foo.sync = %v, want pointer to 'synced'", p.Sync)
			}
			if p.Version == nil || *p.Version != "1.2.0" {
				t.Errorf("foo.version = %v, want '1.2.0'", p.Version)
			}
			if p.ImageDigest == nil || !strings.HasPrefix(*p.ImageDigest, "sha256:") {
				t.Errorf("foo.imageDigest = %v", p.ImageDigest)
			}
		}
	}
}

func TestDeploymentByProjectService_List_Mockup6(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Application": applicationListJSON([]applicationStub{
				{appName: "foo", pipeline: "my-pipeline", stage: "dev", health: "Healthy", sync: "Synced", imageRepo: "registry/foo", imageTag: "1.2.3", imageDigest: "sha256:abc12345xx", externalURLs: []string{"https://foo.dev.example.com"}},
				{appName: "foo", pipeline: "my-pipeline", stage: "stage", health: "Healthy", sync: "Synced", imageRepo: "registry/foo", imageTag: "1.2.3", imageDigest: "sha256:abc12345xx", externalURLs: []string{"https://foo.stage.example.com"}},
				{appName: "foo", pipeline: "my-pipeline", stage: "prod", health: "Degraded", sync: "OutOfSync", imageRepo: "registry/foo", imageTag: "1.2.0", imageDigest: "sha256:def34567xx", externalURLs: []string{"https://foo.prod.example.com", "https://foo-admin.prod.example.com"}},
			}),
			"Stage": stageListJSON([]stageStub{
				{name: "my-pipeline-dev", deployment: "my-pipeline", env: "dev", cluster: "in-cluster", namespace: "my-pipeline-dev", trigger: "Auto", order: 0, status: "created"},
				{name: "my-pipeline-stage", deployment: "my-pipeline", env: "stage", cluster: "in-cluster", namespace: "my-pipeline-stage", trigger: "Manual", order: 1, status: "created"},
				{name: "my-pipeline-prod", deployment: "my-pipeline", env: "prod", cluster: "in-cluster", namespace: "my-pipeline-prod", trigger: "Manual", order: 2, status: "created"},
				{name: "other-pipe-dev", deployment: "other-pipe", env: "dev", cluster: "in-cluster", namespace: "other-pipe-dev", trigger: "Auto", order: 0, status: "created"},
			}),
			"CDPipeline": cdPipelineListJSON([]cdPipelineStub{
				{name: "my-pipeline", applications: []string{"foo", "bar"}},
				{name: "other-pipe", applications: []string{"foo"}},
			}),
		},
	}

	svc, closer := newDeploymentByProjectServiceForTest(t, rec)
	defer closer()

	rows, err := svc.List(context.Background(), "foo")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4: %+v", len(rows), rows)
	}

	// Sort: my-pipeline (dev,stage,prod) then other-pipe (dev)
	want := []struct {
		deployment string
		env        string
		deployed   bool
	}{
		{"my-pipeline", "dev", true},
		{"my-pipeline", "stage", true},
		{"my-pipeline", "prod", true},
		{"other-pipe", "dev", false},
	}

	for i, row := range rows {
		if row.Deployment != want[i].deployment {
			t.Errorf("rows[%d].deployment = %q, want %q", i, row.Deployment, want[i].deployment)
		}
		if row.Env != want[i].env {
			t.Errorf("rows[%d].env = %q, want %q", i, row.Env, want[i].env)
		}
		if row.Deployed != want[i].deployed {
			t.Errorf("rows[%d].deployed = %v, want %v", i, row.Deployed, want[i].deployed)
		}
	}

	// other-pipe/dev row carries Stage statics even when not deployed
	last := rows[3]
	if last.Cluster == "" || last.Namespace == "" || last.TriggerType == "" {
		t.Errorf("not-deployed row must still carry cluster/namespace/triggerType: %+v", last)
	}
	if last.Status != nil || last.Sync != nil || last.Version != nil {
		t.Errorf("not-deployed row dynamic fields must be nil: %+v", last)
	}
	if len(last.IngressURLs) != 0 {
		t.Errorf("not-deployed row ingressUrls must be empty, got %v", last.IngressURLs)
	}

	// JSON round-trip — preserves the schema shape.
	payload := ProjectDeploymentsPayload{Project: "foo", Rows: rows}
	if _, err := json.Marshal(payload); err != nil {
		t.Errorf("JSON marshal failed: %v", err)
	}
}

func TestDeploymentByProjectService_List_NoPipeline(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Application": applicationListJSON(nil),
			"Stage":       stageListJSON(nil),
			"CDPipeline":  cdPipelineListJSON(nil),
		},
	}

	svc, closer := newDeploymentByProjectServiceForTest(t, rec)
	defer closer()

	rows, err := svc.List(context.Background(), "foo")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(rows) != 0 {
		t.Errorf("expected zero rows, got %+v", rows)
	}
}

func TestDeployedVersion_HelmSingleSource_ImageTagParam(t *testing.T) {
	t.Parallel()

	spec := map[string]any{
		"source": map[string]any{
			"helm": map[string]any{
				"parameters": []any{
					map[string]any{"name": "image.tag", "value": "1.2.3"},
				},
			},
			"targetRevision": "v9.9.9",
		},
	}

	tag, _ := helmImageFields(spec)
	got, ok := deployedVersion(spec, tag)
	if !ok || got != "1.2.3" {
		t.Errorf("deployedVersion = (%q,%v), want ('1.2.3',true)", got, ok)
	}
}

func TestDeployedVersion_HelmSingleSource_TargetRevision(t *testing.T) {
	t.Parallel()

	spec := map[string]any{
		"source": map[string]any{
			"helm":           map[string]any{"parameters": []any{}},
			"targetRevision": "refs/tags/v1.0.0",
		},
	}

	tag, _ := helmImageFields(spec)
	got, ok := deployedVersion(spec, tag)
	if !ok || got != "v1.0.0" {
		t.Errorf("deployedVersion = (%q,%v), want ('v1.0.0',true)", got, ok)
	}
}

func TestDeployedVersion_MultiSource_HelmParam(t *testing.T) {
	t.Parallel()

	spec := map[string]any{
		"sources": []any{
			map[string]any{"targetRevision": "main"},
			map[string]any{
				"helm": map[string]any{
					"parameters": []any{
						map[string]any{"name": "image.tag", "value": "v2.0.0"},
					},
				},
			},
		},
	}

	tag, _ := helmImageFields(spec)
	got, ok := deployedVersion(spec, tag)
	if !ok || got != "v2.0.0" {
		t.Errorf("deployedVersion = (%q,%v), want ('v2.0.0',true)", got, ok)
	}
}

func TestDeployedVersion_NoMatch(t *testing.T) {
	t.Parallel()

	spec := map[string]any{}

	tag, _ := helmImageFields(spec)
	if v, ok := deployedVersion(spec, tag); ok {
		t.Errorf("deployedVersion = %q, want empty", v)
	}
}

func TestImageDigest_AnchoredPrefix(t *testing.T) {
	t.Parallel()

	// summary.images contains a same-prefix repo first; the matcher must
	// skip it and pick the exact-repo entry.
	summary := map[string]any{
		"images": []any{
			"registry/foo-bar:1.0@sha256:wrong",
			"registry/foo:1.2.3@sha256:right",
		},
	}

	got, ok := imageDigest("registry/foo", summary)
	if !ok || got != "sha256:right" {
		t.Errorf("imageDigest = (%q,%v), want ('sha256:right',true)", got, ok)
	}

	// No exact match → not found, even though a similarly-named repo exists.
	summaryOnlyOther := map[string]any{
		"images": []any{"registry/foo-bar:1.0@sha256:wrong"},
	}

	if got, ok := imageDigest("registry/foo", summaryOnlyOther); ok {
		t.Errorf("imageDigest = %q, want empty (no exact-repo match)", got)
	}
}

func TestFindSliceItem(t *testing.T) {
	t.Parallel()

	slice := []any{
		map[string]any{"name": "image.repository", "value": "registry/foo"},
		map[string]any{"name": "image.tag", "value": "1.2.3"},
	}

	if got := findSliceItem(slice, "name", "image.tag"); got == nil || got["value"] != "1.2.3" {
		t.Errorf("findSliceItem(image.tag) = %v", got)
	}

	if got := findSliceItem(slice, "name", "missing"); got != nil {
		t.Errorf("findSliceItem(missing) = %v, want nil", got)
	}
}

// --- helpers ---

// mustJSON marshals v or panics on failure. The test fixtures only build
// deterministic primitive trees so a marshal error is a programmer mistake,
// not test data; surface it loudly instead of returning an empty string.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic("test fixture marshal: " + err.Error())
	}

	return string(b)
}

// stageStub describes the minimal Stage spec/status used by the test stubs.
type stageStub struct {
	name       string
	deployment string
	env        string
	cluster    string
	namespace  string
	trigger    string
	order      int
	status     string
}

func stageListJSON(stages []stageStub) string {
	type metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels,omitempty"`
	}

	type stage struct {
		Metadata metadata       `json:"metadata"`
		Spec     map[string]any `json:"spec"`
		Status   map[string]any `json:"status"`
	}

	items := make([]stage, 0, len(stages))
	for _, s := range stages {
		items = append(items, stage{
			Metadata: metadata{
				Name:      s.name,
				Namespace: "ns",
				Labels: map[string]string{
					"app.edp.epam.com/cdPipelineName": s.deployment,
				},
			},
			Spec: map[string]any{
				"name":            s.env,
				"cdPipeline":      s.deployment,
				"clusterName":     s.cluster,
				"namespace":       s.namespace,
				"triggerType":     s.trigger,
				"triggerTemplate": "deploy",
				"order":           s.order,
				"qualityGates":    []any{},
			},
			Status: map[string]any{"status": s.status},
		})
	}

	envelope := map[string]any{
		"apiVersion": "v1",
		"kind":       "List",
		"metadata":   map[string]any{},
		"items":      items,
	}

	return mustJSON(envelope)
}

type applicationStub struct {
	appName      string
	pipeline     string
	stage        string
	health       string
	sync         string
	imageRepo    string
	imageTag     string
	imageDigest  string // already in "sha256:..." format; appended to image entry
	externalURLs []string
}

func applicationListJSON(apps []applicationStub) string {
	type metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels,omitempty"`
	}

	type app struct {
		Metadata metadata       `json:"metadata"`
		Spec     map[string]any `json:"spec"`
		Status   map[string]any `json:"status"`
	}

	items := make([]app, 0, len(apps))
	for _, a := range apps {
		params := []any{}
		if a.imageRepo != "" {
			params = append(params, map[string]any{"name": "image.repository", "value": a.imageRepo})
		}
		if a.imageTag != "" {
			params = append(params, map[string]any{"name": "image.tag", "value": a.imageTag})
		}

		summary := map[string]any{}
		if len(a.externalURLs) > 0 {
			summary["externalURLs"] = stringsToAny(a.externalURLs)
		}
		if a.imageDigest != "" {
			summary["images"] = stringsToAny([]string{a.imageRepo + ":" + a.imageTag + "@" + a.imageDigest})
		}

		items = append(items, app{
			Metadata: metadata{
				Name:      a.pipeline + "-" + a.stage + "-" + a.appName,
				Namespace: "ns",
				Labels: map[string]string{
					"app.edp.epam.com/app-name":       a.appName,
					"app.edp.epam.com/cdPipelineName": a.pipeline,
					"app.edp.epam.com/pipeline":       a.pipeline,
					"app.edp.epam.com/stage":          a.stage,
				},
			},
			Spec: map[string]any{
				"source": map[string]any{
					"helm":           map[string]any{"parameters": params},
					"targetRevision": "main",
				},
			},
			Status: map[string]any{
				"health":  map[string]any{"status": a.health},
				"sync":    map[string]any{"status": a.sync},
				"summary": summary,
			},
		})
	}

	envelope := map[string]any{
		"apiVersion": "v1",
		"kind":       "List",
		"metadata":   map[string]any{},
		"items":      items,
	}

	return mustJSON(envelope)
}

func stringsToAny(s []string) []any {
	out := make([]any, 0, len(s))
	for _, v := range s {
		out = append(out, v)
	}
	return out
}

type cdPipelineStub struct {
	name         string
	applications []string
}

func cdPipelineListJSON(items []cdPipelineStub) string {
	type metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}

	type pipe struct {
		Metadata metadata       `json:"metadata"`
		Spec     map[string]any `json:"spec"`
		Status   map[string]any `json:"status"`
	}

	out := make([]pipe, 0, len(items))
	for _, it := range items {
		out = append(out, pipe{
			Metadata: metadata{Name: it.name, Namespace: "ns"},
			Spec: map[string]any{
				"applications": stringsToAny(it.applications),
			},
			Status: map[string]any{"status": "created"},
		})
	}

	envelope := map[string]any{
		"apiVersion": "v1",
		"kind":       "List",
		"metadata":   map[string]any{},
		"items":      out,
	}

	return mustJSON(envelope)
}

func cdPipelineGetJSON(name string, applications []string) string {
	envelope := map[string]any{
		"apiVersion": "v1",
		"kind":       "CDPipeline",
		"metadata":   map[string]any{"name": name, "namespace": "ns"},
		"spec":       map[string]any{"applications": stringsToAny(applications)},
		"status":     map[string]any{"status": "created"},
	}

	return mustJSON(envelope)
}
