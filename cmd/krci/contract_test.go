package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/pkg/cmd/root"
)

var update = flag.Bool("update", false, "rewrite contract goldens")

// fixedNow is the Factory clock of every row.
var fixedNow = time.Date(2026, 1, 2, 10, 2, 3, 0, time.UTC)

const (
	contractDir = "testdata/contract"
	fixturesDir = contractDir + "/fixtures"

	formatJSON    = "json"
	formatUnknown = "xml"

	cmdPipelineRunList  = "pipelinerun list"
	cmdPipelineRunGet   = "pipelinerun get"
	cmdPipelineRunStart = "pipelinerun start"
	cmdProjectBuild     = "project build"
	cmdEnvGet           = "env get"
	cmdEnvPods          = "env pods"
	cmdEnvEvents        = "env events"
	cmdAuthStatus       = "auth status"
	cmdSonarList        = "sonar list"
	cmdSonarGet         = "sonar get"
	cmdSonarGate        = "sonar gate"
	cmdSonarIssues      = "sonar issues"
	cmdSCAList          = "sca list"
	cmdSCAGet           = "sca get"
	cmdSCAComponents    = "sca components"
	cmdSCAFindings      = "sca findings"

	cmdProjectList        = "project list"
	cmdProjectGet         = "project get"
	cmdProjectVersions    = "project versions"
	cmdProjectDeployments = "project deployments"
	cmdDeploymentList     = "deployment list"
	cmdDeploymentGet      = "deployment get"
	cmdEnvList            = "env list"

	nameUnauthorized  = "unauthorized"
	nameUnknownFormat = "unknown-format"
	nameNotFound      = "not-found"

	runSucceeded = "review-my-app-main-s1a2b"
	runFailed    = "build-my-app-main-f9x8y"
	failedResult = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

	startPipeline = "my-app-release"
	buildProject  = "my-app"
	scanProject   = "my-app"
	projectName   = "my-app"

	baseCluster   = "in-cluster"
	baseNamespace = "ns"

	envDeployment = "my-pipeline"
	envName       = "dev"
	envNamespace  = "my-pipeline-dev"

	emptyDeployment = "api-pipeline"

	pathResources  = "/rest/v1/resources/"
	pathList       = pathResources + "list"
	pathGet        = pathResources + "get"
	pathResults    = "/rest/v1/pipeline-runs"
	pathStart      = "/rest/v1/pipelineruns/start"
	pathBuild      = "/rest/v1/pipelineruns/build"
	pathConfig     = "/rest/v1/config"
	pathTaskRuns   = pathResults + "/" + failedResult + "/task-runs"
	pathTaskRunLog = "/rest/v1/task-runs/" + failedResult + "/logs"

	pathSonarList     = "/rest/v1/sonar/list"
	pathSonarGet      = "/rest/v1/sonar/get"
	pathSonarGate     = "/rest/v1/sonar/gate"
	pathSonarIssues   = "/rest/v1/sonar/issues"
	pathSCAList       = "/rest/v1/sca/list"
	pathSCAGet        = "/rest/v1/sca/get"
	pathSCAComponents = "/rest/v1/sca/components"
	pathSCAFindings   = "/rest/v1/sca/findings"
)

// envToken is the default KRCI_TOKEN: an unsigned JWT that expires in 2100.
var envToken = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." +
	base64.RawURLEncoding.EncodeToString([]byte(
		`{"email":"dev@example.com","name":"Dev User","groups":["developers"],"exp":4102444800}`)) + "."

// contractRow is one run of the CLI. format is the -o value, required; table is
// not golden-tested. env overrides the base environment; an empty value blanks
// the variable.
type contractRow struct {
	cmd      string
	args     []string
	format   string
	fixture  string
	env      map[string]string
	wantExit int
	name     string
}

func (r contractRow) argv() []string {
	return slices.Concat(strings.Fields(r.cmd), r.args, []string{"-o", r.format})
}

// id is the subtest name and the golden path relative to contractDir, without
// the stream extension.
func (r contractRow) id() string {
	return strings.ReplaceAll(r.cmd, " ", "_") + "/" + r.name
}

// goldenPath returns the path of the golden file of one stream: stdout or
// stderr.
func (r contractRow) goldenPath(stream string) string {
	return filepath.Join(contractDir, filepath.FromSlash(r.id())+"."+stream)
}

var contractRows = []contractRow{
	{cmd: cmdPipelineRunList, format: formatJSON, fixture: "runs", name: "merged"},
	{cmd: cmdPipelineRunList, args: []string{"--status", "running"}, format: formatJSON, fixture: "runs",
		name: "status-running"},
	{cmd: cmdPipelineRunList, format: formatJSON, fixture: "runs-live-401", wantExit: 1, name: "live-unauthorized"},
	{cmd: cmdPipelineRunList, format: formatUnknown, fixture: "runs", wantExit: 1, name: nameUnknownFormat},

	{cmd: cmdPipelineRunGet, args: []string{runSucceeded}, format: formatJSON, fixture: "runs", name: "succeeded"},
	{cmd: cmdPipelineRunGet, args: []string{runFailed, "--wait"}, format: formatJSON, fixture: "failed-run",
		wantExit: 1, name: "wait-failed"},
	{cmd: cmdPipelineRunGet, args: []string{runFailed, "--reason"}, format: formatJSON, fixture: "failed-run",
		name: "reason-failed"},
	{cmd: cmdPipelineRunGet, args: []string{"ghost"}, format: formatJSON, fixture: "no-runs", wantExit: 1,
		name: nameNotFound},
	{cmd: cmdPipelineRunGet, args: []string{runSucceeded}, format: formatJSON, fixture: "runs",
		env: map[string]string{"KRCI_TOKEN": ""}, wantExit: 1, name: "no-token"},

	{cmd: cmdPipelineRunStart, args: []string{startPipeline, "--param", "git-revision=main"}, format: formatJSON,
		fixture: "start", name: "created"},
	{cmd: cmdPipelineRunStart, args: []string{startPipeline}, format: formatJSON, fixture: "start-401",
		wantExit: 1, name: nameUnauthorized},
	{cmd: cmdPipelineRunStart, args: []string{startPipeline}, format: formatUnknown, fixture: "none",
		wantExit: 1, name: nameUnknownFormat},

	{cmd: cmdProjectBuild, args: []string{buildProject, "--branch", "main"}, format: formatJSON, fixture: "build",
		name: "created"},
	{cmd: cmdProjectBuild, args: []string{buildProject}, format: formatJSON, fixture: "build-401", wantExit: 1,
		name: nameUnauthorized},

	{cmd: cmdEnvGet, args: []string{envDeployment, envName}, format: formatJSON, fixture: "env", name: "detail"},
	{cmd: cmdEnvGet, args: []string{envDeployment, envName}, format: formatJSON, fixture: "env-applications-401",
		wantExit: 1, name: nameUnauthorized},
	{cmd: cmdEnvGet, args: []string{envDeployment, envName}, format: formatUnknown, fixture: "none",
		wantExit: 1, name: nameUnknownFormat},
	{cmd: cmdEnvPods, args: []string{envDeployment, envName}, format: formatJSON, fixture: "env", name: "pods"},
	{cmd: cmdEnvPods, args: []string{envDeployment, envName}, format: formatJSON, fixture: "env-pods-401",
		wantExit: 1, name: nameUnauthorized},
	{cmd: cmdEnvEvents, args: []string{envDeployment, envName}, format: formatJSON, fixture: "env", name: "events"},
	{cmd: cmdEnvEvents, args: []string{envDeployment, envName}, format: formatJSON, fixture: "env-events-401",
		wantExit: 1, name: nameUnauthorized},

	{cmd: cmdAuthStatus, format: formatJSON, fixture: "config", name: "env-token"},
	{cmd: cmdAuthStatus, format: formatJSON, fixture: "config-401", wantExit: 1, name: nameUnauthorized},
	{cmd: cmdAuthStatus, format: formatUnknown, fixture: "none", wantExit: 1, name: nameUnknownFormat},

	{cmd: cmdSonarList, format: formatJSON, fixture: "sonar", name: "projects"},
	{cmd: cmdSonarList, format: formatJSON, fixture: "sonar-401", wantExit: 1, name: nameUnauthorized},
	{cmd: cmdSonarList, format: formatUnknown, fixture: "none", wantExit: 1, name: nameUnknownFormat},
	{cmd: cmdSonarGet, args: []string{scanProject}, format: formatJSON, fixture: "sonar", name: "detail"},
	{cmd: cmdSonarGet, args: []string{scanProject}, format: formatJSON, fixture: "sonar-401", wantExit: 1,
		name: nameUnauthorized},
	{cmd: cmdSonarGet, args: []string{scanProject}, format: formatUnknown, fixture: "none", wantExit: 1,
		name: nameUnknownFormat},
	{cmd: cmdSonarGate, args: []string{scanProject, "--branch", "main"}, format: formatJSON, fixture: "sonar",
		name: "failed-gate"},
	{cmd: cmdSonarGate, args: []string{scanProject, "--branch", "main"}, format: formatJSON,
		fixture: "sonar-401", wantExit: 1, name: nameUnauthorized},
	{cmd: cmdSonarIssues, args: []string{scanProject, "--pr", "42"}, format: formatJSON, fixture: "sonar",
		name: "issues"},
	{cmd: cmdSonarIssues, args: []string{scanProject, "--pr", "42"}, format: formatJSON, fixture: "sonar-401",
		wantExit: 1, name: nameUnauthorized},

	{cmd: cmdSCAList, format: formatJSON, fixture: "sca", name: "projects"},
	{cmd: cmdSCAList, format: formatJSON, fixture: "sca-401", wantExit: 1, name: nameUnauthorized},
	{cmd: cmdSCAList, format: formatJSON, fixture: "sca-503", wantExit: 1, name: "upstream-unavailable"},
	{cmd: cmdSCAList, format: formatUnknown, fixture: "none", wantExit: 1, name: nameUnknownFormat},
	{cmd: cmdSCAGet, args: []string{scanProject}, format: formatJSON, fixture: "sca", name: "detail"},
	{cmd: cmdSCAGet, args: []string{scanProject}, format: formatJSON, fixture: "sca-401", wantExit: 1,
		name: nameUnauthorized},
	{cmd: cmdSCAGet, args: []string{scanProject}, format: formatUnknown, fixture: "none", wantExit: 1,
		name: nameUnknownFormat},
	{cmd: cmdSCAComponents, args: []string{scanProject, "--severity", "high"}, format: formatJSON, fixture: "sca",
		name: "components"},
	{cmd: cmdSCAComponents, args: []string{scanProject, "--severity", "high"}, format: formatJSON,
		fixture: "sca-401", wantExit: 1, name: nameUnauthorized},
	{cmd: cmdSCAFindings, args: []string{scanProject}, format: formatJSON, fixture: "sca", name: "findings"},
	{cmd: cmdSCAFindings, args: []string{scanProject}, format: formatJSON, fixture: "sca-401", wantExit: 1,
		name: nameUnauthorized},

	{cmd: cmdProjectList, format: formatJSON, fixture: "project", name: "projects"},
	{cmd: cmdProjectList, format: formatJSON, fixture: "codebase-401", wantExit: 1, name: nameUnauthorized},
	{cmd: cmdProjectGet, args: []string{projectName}, format: formatJSON, fixture: "project", name: "detail"},
	{cmd: cmdProjectGet, args: []string{projectName}, format: formatJSON, fixture: "codebase-404", wantExit: 1,
		name: nameNotFound},
	{cmd: cmdProjectGet, args: []string{projectName}, format: formatJSON, fixture: "codebase-401", wantExit: 1,
		name: nameUnauthorized},
	{cmd: cmdProjectVersions, args: []string{projectName}, format: formatJSON, fixture: "project", name: "versions"},
	{cmd: cmdProjectVersions, args: []string{projectName}, format: formatJSON, fixture: "codebase-404",
		wantExit: 1, name: nameNotFound},
	{cmd: cmdProjectVersions, args: []string{projectName}, format: formatJSON, fixture: "image-streams-401",
		wantExit: 1, name: nameUnauthorized},
	{cmd: cmdProjectDeployments, args: []string{projectName}, format: formatJSON, fixture: "project",
		name: "deployments"},
	{cmd: cmdProjectDeployments, args: []string{projectName}, format: formatJSON, fixture: "cdpipelines-401",
		wantExit: 1, name: nameUnauthorized},

	{cmd: cmdDeploymentList, format: formatJSON, fixture: "project", name: "deployments"},
	{cmd: cmdDeploymentList, format: formatJSON, fixture: "cdpipelines-401", wantExit: 1, name: nameUnauthorized},
	{cmd: cmdDeploymentGet, args: []string{envDeployment}, format: formatJSON, fixture: "env", name: "detail"},
	{cmd: cmdDeploymentGet, args: []string{emptyDeployment}, format: formatJSON, fixture: "project",
		name: "no-stages"},
	{cmd: cmdDeploymentGet, args: []string{envDeployment}, format: formatJSON, fixture: "cdpipeline-404",
		wantExit: 1, name: nameNotFound},
	{cmd: cmdDeploymentGet, args: []string{envDeployment}, format: formatJSON, fixture: "cdpipeline-401",
		wantExit: 1, name: nameUnauthorized},
	{cmd: cmdDeploymentGet, args: []string{envDeployment}, format: formatUnknown, fixture: "env", wantExit: 1,
		name: nameUnknownFormat},

	{cmd: cmdEnvList, format: formatJSON, fixture: "env", name: "stages"},
	{cmd: cmdEnvList, format: formatJSON, fixture: "stages-401", wantExit: 1, name: nameUnauthorized},
}

// route answers one request. kind and name match resourceConfig.kind and name
// of the POST body, empty matching anything. namespace matches the namespace
// of the POST body or the namespace query, empty for an endpoint that names
// none. query is a substring of the raw query. params, when set, is the exact
// set of query parameters. bodyContains are substrings of the raw request
// body. status 0 is 200. body is the response, served as JSON.
type route struct {
	method, path, kind, name, namespace, query string
	params                                     url.Values
	bodyContains                               []string
	status                                     int
	body                                       string
}

// request is what a route matches besides the method, path and raw query.
type request struct {
	kind, name, namespace, body string
}

func (rt route) matches(r *http.Request, req request) bool {
	return rt.method == r.Method && rt.path == r.URL.Path &&
		(rt.kind == "" || rt.kind == req.kind) &&
		(rt.name == "" || rt.name == req.name) &&
		rt.namespace == req.namespace &&
		strings.Contains(r.URL.RawQuery, rt.query) &&
		(rt.params == nil || maps.EqualFunc(rt.params, r.URL.Query(), slices.Equal[[]string])) &&
		!slices.ContainsFunc(rt.bodyContains, func(s string) bool { return !strings.Contains(req.body, s) })
}

func (rt route) in(ns string) route {
	rt.namespace = ns

	return rt
}

// fixtures returns the route tables by name and the set of fixture file paths
// they read. Bodies are files under fixturesDir. Routes are in baseNamespace
// unless set otherwise. The first matching route answers. "none" serves no
// route: its rows must fail before any request.
func fixtures(t *testing.T) (map[string][]route, map[string]bool) {
	t.Helper()

	files := map[string]bool{}
	file := func(name string) string {
		path := filepath.Join(fixturesDir, name+".json")
		files[path] = true

		b, err := os.ReadFile(path)
		require.NoError(t, err)

		return string(b)
	}

	unauthorized := file("unauthorized")
	notFound := file("not-found")
	fail := func(rt route, status int, body string) route {
		rt.status, rt.body = status, body

		return rt
	}
	deny := func(rt route) route { return fail(rt, http.StatusUnauthorized, unauthorized) }
	missing := func(rt route) route { return fail(rt, http.StatusNotFound, notFound) }
	denyAll := func(rts []route) []route {
		out := make([]route, 0, len(rts))
		for _, rt := range rts {
			out = append(out, deny(rt))
		}

		return out
	}

	at := func(method, path, body string) route {
		return route{method: method, path: path, namespace: baseNamespace, body: file(body)}
	}

	list := func(kind, body string) route {
		rt := at(http.MethodPost, pathList, body)
		rt.kind = kind

		return rt
	}

	// Sonar and sca endpoints name no namespace.
	getQuery := func(path string, params url.Values, body string) route {
		rt := at(http.MethodGet, path, body).in("")
		rt.params = params

		return rt
	}

	liveRuns := list("PipelineRun", "runs-live")
	noResults := at(http.MethodGet, pathResults, "runs-results-empty")
	start := at(http.MethodPost, pathStart, "start-created")
	start.bodyContains = []string{`"pipeline":"` + startPipeline + `"`}
	build := at(http.MethodPost, pathBuild, "build-created")
	build.bodyContains = []string{`"codebase":"` + buildProject + `"`}
	cfgRoute := at(http.MethodGet, pathConfig, "config").in("")

	failedResults := at(http.MethodGet, pathResults, "runs-results-failed")
	failedResults.query = runFailed
	failedLogs := at(http.MethodGet, pathTaskRunLog, "taskrun-logs-compile")
	failedLogs.query = "taskRunName=" + runFailed + "-compile"

	pipeline := at(http.MethodPost, pathGet, "env-cdpipeline")
	pipeline.kind, pipeline.name = "CDPipeline", envDeployment

	stages := list("Stage", "env-stages")
	apps := list("Application", "env-applications")
	pods := list("Pod", "env-pods").in(envNamespace)
	events := list("Event", "env-events").in(envNamespace)
	env := []route{stages, pipeline, apps, pods, events}

	codebases := list("Codebase", "project-codebases")
	codebase := at(http.MethodPost, pathGet, "project-codebase")
	codebase.kind, codebase.name = "Codebase", projectName
	byProject := []string{`"labels":{"app.edp.epam.com/codebase":"` + projectName + `"}`}
	streams := list("CodebaseImageStream", "project-image-streams")
	streams.bodyContains = byProject
	branches := list("CodebaseBranch", "project-branches")
	branches.bodyContains = byProject
	cdpipelines := list("CDPipeline", "deployment-cdpipelines")
	apiPipeline := at(http.MethodPost, pathGet, "deployment-api-pipeline")
	apiPipeline.kind, apiPipeline.name = "CDPipeline", emptyDeployment
	projectApps := list("Application", "project-applications")
	projectApps.bodyContains = []string{`"labels":{"app.edp.epam.com/app-name":"` + projectName + `"}`}
	project := []route{codebases, codebase, streams, branches, cdpipelines, apiPipeline, stages, projectApps}

	sonar := []route{
		getQuery(pathSonarList, url.Values{"page": {"1"}, "pageSize": {"50"}}, "sonar-projects"),
		getQuery(pathSonarGet, url.Values{"projectKey": {scanProject}}, "sonar-project"),
		getQuery(pathSonarGate, url.Values{"projectKey": {scanProject}, "branch": {"main"}}, "sonar-gate"),
		getQuery(pathSonarIssues,
			url.Values{"projectKey": {scanProject}, "pullRequest": {"42"}, "p": {"1"}, "ps": {"25"}},
			"sonar-issues"),
	}
	scaList := getQuery(pathSCAList, url.Values{
		"excludeInactive": {"true"}, "onlyRoot": {"true"}, "pageNumber": {"1"}, "pageSize": {"25"},
	}, "sca-projects")
	sca := []route{
		scaList,
		getQuery(pathSCAGet, url.Values{"codebase": {scanProject}}, "sca-project"),
		getQuery(pathSCAComponents, url.Values{
			"codebase": {scanProject}, "pageNumber": {"1"}, "pageSize": {"50"}, "severity": {"CRITICAL,HIGH"},
		}, "sca-components"),
		getQuery(pathSCAFindings, url.Values{"codebase": {scanProject}, "suppressed": {"false"}}, "sca-findings"),
	}

	startParam := start
	startParam.bodyContains = slices.Concat(start.bodyContains, []string{`"git-revision":"main"`})
	buildBranch := build
	buildBranch.bodyContains = slices.Concat(build.bodyContains, []string{`"branch":"main"`})

	return map[string][]route{
		"runs":          {liveRuns, at(http.MethodGet, pathResults, "runs-results")},
		"runs-live-401": {deny(liveRuns), noResults},
		"failed-run": {
			list("PipelineRun", "runs-live-failed"),
			failedResults,
			at(http.MethodGet, pathTaskRuns, "taskruns-failed"),
			failedLogs,
		},
		"no-runs":              {list("PipelineRun", "runs-live-empty"), noResults},
		"start":                {startParam},
		"start-401":            {deny(start)},
		"build":                {buildBranch},
		"build-401":            {deny(build)},
		"env":                  env,
		"env-applications-401": append([]route{deny(apps)}, env...),
		"env-pods-401":         append([]route{deny(pods)}, env...),
		"env-events-401":       append([]route{deny(events)}, env...),
		"stages-401":           append([]route{deny(stages)}, env...),
		"cdpipeline-404":       append([]route{missing(pipeline)}, env...),
		"cdpipeline-401":       append([]route{deny(pipeline)}, env...),
		"project":              project,
		"codebase-404":         append([]route{missing(codebase)}, project...),
		"codebase-401":         append(denyAll([]route{codebases, codebase}), project...),
		"image-streams-401":    append([]route{deny(streams)}, project...),
		"cdpipelines-401":      append([]route{deny(cdpipelines)}, project...),
		"config":               {cfgRoute},
		"none":                 {},
		"config-401":           {deny(cfgRoute)},
		"sonar":                sonar,
		"sonar-401":            denyAll(sonar),
		"sca":                  sca,
		"sca-401":              denyAll(sca),
		// sca-unavailable.json has no trailing newline: the 503 message embeds
		// the body verbatim.
		"sca-503": {fail(scaList, http.StatusServiceUnavailable, file("sca-unavailable"))},
	}, files
}

// portalStub serves routes. A request whose Authorization is not "Bearer " +
// token, or a request to /rest/v1/resources/* whose body clusterName is not
// cluster, fails the test. An unmatched request fails the test and gets 500.
func portalStub(t *testing.T, routes []route, cluster, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("%s %s: Authorization = %q, want bearer of the row token", r.Method, r.URL.Path, got)
		}

		var body struct {
			ClusterName    string `json:"clusterName"`
			Namespace      string `json:"namespace"`
			Name           string `json:"name"`
			ResourceConfig struct {
				Kind string `json:"kind"`
			} `json:"resourceConfig"`
		}

		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)

		if strings.HasPrefix(r.URL.Path, pathResources) && body.ClusterName != cluster {
			t.Errorf("%s %s: clusterName = %q, want %q", r.Method, r.URL.Path, body.ClusterName, cluster)
		}

		req := request{
			kind:      body.ResourceConfig.Kind,
			name:      body.Name,
			namespace: cmp.Or(body.Namespace, r.URL.Query().Get("namespace")),
			body:      string(raw),
		}

		for _, rt := range routes {
			if rt.matches(r, req) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(cmp.Or(rt.status, http.StatusOK))
				_, _ = io.WriteString(w, rt.body)

				return
			}
		}

		t.Errorf("unmatched request %s %s %+v query=%q", r.Method, r.URL.Path, req, r.URL.RawQuery)
		w.WriteHeader(http.StatusInternalServerError)
	})
}

// golden compares got with the file at path, or writes it under -update.
func golden(t *testing.T, path string, got []byte) {
	t.Helper()

	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))

		return
	}

	want, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("golden %s is missing; run: go test ./cmd/krci -run TestContract -update", path)

		return
	}

	require.NoError(t, err)
	assert.Equal(t, string(want), string(got), path)
}

func TestContract(t *testing.T) {
	for _, row := range contractRows {
		t.Run(row.id(), func(t *testing.T) {
			runContractRow(t, row)
		})
	}
}

func runContractRow(t *testing.T, row contractRow) {
	isolatedConfigDir(t)

	tables, _ := fixtures(t)
	routes, ok := tables[row.fixture]
	require.True(t, ok, "unknown fixture %q", row.fixture)

	env := map[string]string{
		"KRCI_CLUSTER_NAME":    baseCluster,
		"KRCI_NAMESPACE":       baseNamespace,
		"KRCI_KEYRING_BACKEND": "file",
		"KRCI_TOKEN":           envToken,
	}
	maps.Copy(env, row.env)

	srv := httptest.NewServer(portalStub(t, routes, env["KRCI_CLUSTER_NAME"], env["KRCI_TOKEN"]))
	t.Cleanup(srv.Close)

	if _, ok := row.env["KRCI_PORTAL_URL"]; !ok {
		env["KRCI_PORTAL_URL"] = srv.URL
	}

	for k, v := range env {
		t.Setenv(k, v)
	}

	var out, errb bytes.Buffer

	f := newFactory(&out, &errb)
	f.Now = func() time.Time { return fixedNow }

	code := runWith(context.Background(), row.argv(), f)

	norm := strings.NewReplacer(
		srv.URL, "{{PORTAL}}",
	)

	assert.Equal(t, row.wantExit, code, "exit code; stderr = %q", errb.String())
	golden(t, row.goldenPath("stdout"), []byte(norm.Replace(out.String())))
	golden(t, row.goldenPath("stderr"), []byte(norm.Replace(errb.String())))
}

// Row goldens are unique. Every file under contractDir outside fixturesDir is
// the golden of a row. Every route table is used by a row. Every file under
// fixturesDir is read while building the route tables.
func TestContract_InventoryMatchesRows(t *testing.T) {
	goldens := map[string]bool{}
	used := map[string]bool{}

	for _, row := range contractRows {
		path := row.goldenPath("stdout")
		assert.False(t, goldens[path], "duplicate golden name %q for %q", row.name, row.cmd)
		goldens[path] = true
		goldens[row.goldenPath("stderr")] = true
		used[row.fixture] = true
	}

	tables, files := fixtures(t)
	fixturesRoot := filepath.Clean(fixturesDir) + string(filepath.Separator)

	err := filepath.WalkDir(contractDir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
		case strings.HasPrefix(path, fixturesRoot):
			assert.True(t, files[path], "%s is read by no route table", path)
		default:
			assert.True(t, goldens[path], "%s is not the golden of any row", path)
		}

		return nil
	})
	require.NoError(t, err)

	for name := range tables {
		assert.True(t, used[name], "fixture %q is used by no row", name)
	}
}

// verbPath is the command path of c without the root name.
func verbPath(c *cobra.Command) string {
	return strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" ")
}

// outputVerbs returns the commands that register -o, keyed by command path
// without the root name.
func outputVerbs(rootCmd *cobra.Command) map[string]bool {
	verbs := map[string]bool{}

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Flags().Lookup("output") != nil {
			verbs[verbPath(c)] = true
		}

		for _, sub := range c.Commands() {
			walk(sub)
		}
	}

	walk(rootCmd)

	return verbs
}

func TestContract_EveryOutputVerbHasRows(t *testing.T) {
	rootCmd := root.NewCmdRoot(newFactory(&bytes.Buffer{}, &bytes.Buffer{}), "test", "none", "unknown")
	verbs := outputVerbs(rootCmd)

	success, failure := map[string]int{}, map[string]int{}

	for _, row := range contractRows {
		assert.NotEmpty(t, row.format, "%s sets no format", row.id())
		assert.NotEqual(t, "table", row.format, "%s: table output is not golden-tested", row.id())

		c, rest, err := rootCmd.Find(strings.Fields(row.cmd))
		if assert.NoError(t, err, row.cmd) && assert.Empty(t, rest, row.cmd) {
			assert.Equal(t, verbPath(c), row.cmd, "row cmd is not the command path")
			assert.NotNil(t, c.Flags().Lookup("output"), "%q has no -o flag", row.cmd)
		}

		if row.wantExit == 0 {
			success[row.cmd]++
		} else {
			failure[row.cmd]++
		}
	}

	for verb := range verbs {
		assert.Positive(t, success[verb], "%q has no success row", verb)
		assert.Positive(t, failure[verb], "%q has no error row", verb)
	}
}
