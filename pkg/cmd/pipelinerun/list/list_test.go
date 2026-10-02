package list

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

func TestList_EnvironmentFlagValidation(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		args []string
		want string
	}{
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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/resources/list":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"List","metadata":{},"items":[
				{"metadata":{"name":"deploy-demo-dev-live1","labels":{"app.edp.epam.com/pipelinetype":"deploy",
					"app.edp.epam.com/cdpipeline":"demo","app.edp.epam.com/cdstage":"demo-dev"}},
				 "status":{"startTime":"2024-01-01T10:10:00Z","conditions":[{"type":"Succeeded","status":"Unknown"}]}},
				{"metadata":{"name":"deploy-demo-qa-live2","labels":{"app.edp.epam.com/pipelinetype":"deploy",
					"app.edp.epam.com/cdpipeline":"demo","app.edp.epam.com/cdstage":"demo-qa"}},
				 "status":{"startTime":"2024-01-01T10:11:00Z","conditions":[{"type":"Succeeded","status":"Unknown"}]}}]}`))
		case "/v1/pipeline-runs":
			filter.Store(r.URL.Query().Get("filter"))
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	out := &bytes.Buffer{}
	f := &cmdutil.Factory{
		IOStreams: &iostreams.IOStreams{Out: out, ErrOut: &bytes.Buffer{}},
		Config: func() (*config.Config, error) {
			return &config.Config{PortalURL: "https://portal.example", ClusterName: "c", Namespace: "ns"}, nil
		},
		RestClient: func() (*restapi.ClientWithResponses, error) {
			return restapi.NewClientWithResponses(srv.URL)
		},
	}

	cmd := NewCmdList(f, func(opts *ListOptions) error {
		return listRun(context.Background(), opts)
	})
	cmd.SetArgs([]string{"--deployment", "demo", "--env", "dev", "--type", "deploy", "-o", "json"})

	if err := cmd.Execute(); err != nil {
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
