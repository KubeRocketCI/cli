package list

import (
	"bytes"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

// liveRunsHandler serves items as the live runs and an empty Tekton Results
// history; the history filter of the request lands in filter.
func liveRunsHandler(items string, filter *atomic.Value) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/resources/list":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"List","metadata":{},"items":[` + items + `]}`))
		case "/v1/pipeline-runs":
			filter.Store(r.URL.Query().Get("filter"))
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func TestList_FlagValidation(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		args []string
		want string
	}{
		"unknown status": {
			[]string{"--status", "bogus"},
			"invalid --status=bogus; must be one of succeeded, failed, running, timeout, cancelled",
		},
		"empty status":           {[]string{"--status", ""}, "invalid --status=; must be one of succeeded"},
		"env without deployment": {[]string{"--env", "dev"}, "--env requires --deployment"},
		"empty deployment":       {[]string{"--deployment", ""}, "--deployment must not be empty"},
		"empty env":              {[]string{"--deployment", "demo", "--env", ""}, "--env must not be empty"},
		"invalid deployment":     {[]string{"--deployment", "Demo_1"}, "--deployment must be a valid DNS-1123 name"},
		"invalid env":            {[]string{"--deployment", "demo", "--env", "dev.eu"}, "--env must be a valid DNS-1123 name"},
		"positional arguments":   {[]string{"demo", "dev"}, `unknown command "demo"`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts, err := cmdtest.RunCmd(t, NewCmdList, tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error %q, got %v", tc.want, err)
			}

			if opts != nil {
				t.Error("runF must not be reached on invalid input")
			}
		})
	}
}

// TestList_EnvironmentFilterEndToEnd drives listRun against a mock portal: the
// flags must reach both the Tekton Results filter and the live-run filter.
func TestList_EnvironmentFilterEndToEnd(t *testing.T) {
	t.Parallel()

	var filter atomic.Value

	f, out := cmdtest.NewPortalFactory(t, liveRunsHandler(`
		{"metadata":{"name":"deploy-demo-dev-live1","labels":{"app.edp.epam.com/pipelinetype":"deploy",
			"app.edp.epam.com/cdpipeline":"demo","app.edp.epam.com/cdstage":"demo-dev"}},
		 "status":{"startTime":"2024-01-01T10:10:00Z","conditions":[{"type":"Succeeded","status":"Unknown"}]}},
		{"metadata":{"name":"deploy-demo-qa-live2","labels":{"app.edp.epam.com/pipelinetype":"deploy",
			"app.edp.epam.com/cdpipeline":"demo","app.edp.epam.com/cdstage":"demo-qa"}},
		 "status":{"startTime":"2024-01-01T10:11:00Z","conditions":[{"type":"Succeeded","status":"Unknown"}]}}`,
		&filter))

	err := cmdtest.Execute(NewCmdList, f,
		[]string{"--deployment", "demo", "--env", "dev", "--type", "deploy", "-o", "json"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got, _ := filter.Load().(string); !strings.Contains(got, `annotations["app.edp.epam.com/cdstage"] == "demo-dev"`) {
		t.Errorf("Tekton Results filter misses the stage: %q", got)
	}

	if !strings.Contains(out.String(), "deploy-demo-dev-live1") || strings.Contains(out.String(), "deploy-demo-qa-live2") {
		t.Errorf("want only the demo/dev run, got:\n%s", out.String())
	}

	if !strings.Contains(out.String(), `"env": "dev"`) {
		t.Errorf("rows must carry the env, got:\n%s", out.String())
	}
}

// runningLiveRun is one live run in progress that started at 10:10:00Z.
const runningLiveRun = `
	{"metadata":{"name":"build-my-app-live1"},
	 "status":{"startTime":"2024-01-01T10:10:00Z","conditions":[{"type":"Succeeded","status":"Unknown"}]}}`

// TestList_RunningDurationUsesTheFactoryClock: the duration of a running run is
// measured to f.Now.
func TestList_RunningDurationUsesTheFactoryClock(t *testing.T) {
	t.Parallel()

	f, out := cmdtest.NewPortalFactory(t, liveRunsHandler(runningLiveRun, &atomic.Value{}))
	f.Now = func() time.Time { return time.Date(2024, time.January, 1, 10, 12, 3, 0, time.UTC) }

	if err := cmdtest.Execute(NewCmdList, f, []string{"--status", "running", "-o", "json"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if want := `"duration": "2m 3s"`; !strings.Contains(out.String(), want) {
		t.Errorf("want %s, got:\n%s", want, out.String())
	}
}

// TestList_ReasonOnARunningRun: the newest match is still running, so it has no tasks, and the
// result says why in both views.
func TestList_ReasonOnARunningRun(t *testing.T) {
	t.Parallel()

	var note bytes.Buffer
	if err := output.RenderNoTaskData(&note, portal.TasksRunNotFinished); err != nil {
		t.Fatal(err)
	}

	reason := `"tasksUnavailable": "` + portal.TasksRunNotFinished + `"`

	cases := map[string]struct {
		args         []string
		want, absent string
	}{
		"json":  {[]string{"--reason", "-o", "json"}, reason, `"tasks"`},
		"table": {[]string{"--reason"}, note.String(), "Tasks:"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f, out := cmdtest.NewPortalFactory(t, liveRunsHandler(runningLiveRun, &atomic.Value{}))

			if err := cmdtest.Execute(NewCmdList, f, tc.args); err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if !strings.Contains(out.String(), tc.want) || strings.Contains(out.String(), tc.absent) {
				t.Errorf("want %q and no %q, got:\n%s", tc.want, tc.absent, out.String())
			}
		})
	}
}
