package get

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// newFactory returns a Factory whose portal serves the given resources/list
// body for every request; an empty body means no portal at all.
func newFactory(t *testing.T, listBody string) (*cmdutil.Factory, *bytes.Buffer) {
	t.Helper()

	out := &bytes.Buffer{}
	f := &cmdutil.Factory{
		IOStreams: &iostreams.IOStreams{Out: out, ErrOut: &bytes.Buffer{}},
		Config: func() (*config.Config, error) {
			return &config.Config{PortalURL: "https://portal.example", ClusterName: "c", Namespace: "ns"}, nil
		},
		RestClient: func() (*restapi.ClientWithResponses, error) {
			return nil, nil
		},
	}

	if listBody == "" {
		return f, out
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(listBody))
	}))
	t.Cleanup(srv.Close)

	f.RestClient = func() (*restapi.ClientWithResponses, error) {
		return restapi.NewClientWithResponses(srv.URL)
	}

	return f, out
}

func runJSON(condStatus, reason string) string {
	return `{"apiVersion":"v1","kind":"List","metadata":{},"items":[{
		"metadata":{"name":"run-x"},
		"status":{"startTime":"2024-01-01T10:00:00Z","conditions":[
			{"type":"Succeeded","status":"` + condStatus + `","reason":"` + reason + `"}
		],"results":[{"name":"VCS_TAG","value":"build/1.0.0"}]}
	}]}`
}

// execute runs `pipelinerun get` with args; the run polls every millisecond.
func execute(f *cmdutil.Factory, args ...string) error {
	cmd := NewCmdGet(f, func(opts *GetOptions) error {
		opts.pollInterval = time.Millisecond
		return getRun(context.Background(), opts)
	})
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	return cmd.Execute()
}

func TestGet_WaitFlagValidation(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		args []string
		want string
	}{
		"timeout without wait": {[]string{"run-x", "--timeout", "5m"}, "--timeout requires --wait"},
		"zero timeout":         {[]string{"run-x", "--wait", "--timeout", "0s"}, "--timeout must be greater than 0"},
		"negative timeout":     {[]string{"run-x", "--wait", "--timeout", "-1m"}, "--timeout must be greater than 0"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f, _ := newFactory(t, "")
			err := execute(f, tc.args...)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error %q, got %v", tc.want, err)
			}
		})
	}
}

func TestGet_WaitDefaults(t *testing.T) {
	t.Parallel()

	f, _ := newFactory(t, "")

	var got *GetOptions

	cmd := NewCmdGet(f, func(opts *GetOptions) error {
		got = opts
		return nil
	})
	cmd.SetArgs([]string{"run-x", "--wait"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !got.Wait || got.Timeout != time.Hour {
		t.Fatalf("want --wait with a 1h timeout, got wait=%v timeout=%s", got.Wait, got.Timeout)
	}
}

func TestGet_WaitSucceededExitsZero(t *testing.T) {
	t.Parallel()

	f, out := newFactory(t, runJSON("True", "Succeeded"))

	if err := execute(f, "run-x", "--wait", "-o", "json"); err != nil {
		t.Fatalf("a succeeded run must exit 0, got %v", err)
	}

	if !strings.Contains(out.String(), `"VCS_TAG": "build/1.0.0"`) {
		t.Fatalf("JSON must carry the pipeline results, got:\n%s", out.String())
	}
}

func TestGet_WaitFailedRunExitsNonZero(t *testing.T) {
	t.Parallel()

	f, out := newFactory(t, runJSON("False", "Failed"))

	err := execute(f, "run-x", "--wait")
	if err == nil {
		t.Fatal("a failed run must exit non-zero")
	}

	for _, want := range []string{`pipeline run "run-x" finished with status Failed`, "krci pipelinerun get run-x --reason"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}

	if !strings.Contains(out.String(), "Pipeline: run-x") {
		t.Errorf("the finished run must still be printed, got:\n%s", out.String())
	}
}

func TestGet_WaitTimesOut(t *testing.T) {
	t.Parallel()

	f, _ := newFactory(t, runJSON("Unknown", "Running"))

	err := execute(f, "run-x", "--wait", "--timeout", "50ms")
	if err == nil || !strings.Contains(err.Error(), `timed out after 50ms waiting for pipeline run "run-x" to finish`) {
		t.Fatalf("want a timeout error, got %v", err)
	}
}

func TestGet_WithoutWaitAFailedRunExitsZero(t *testing.T) {
	t.Parallel()

	f, _ := newFactory(t, runJSON("False", "Failed"))

	if err := execute(f, "run-x"); err != nil {
		t.Fatalf("plain get reports the run, it does not judge it: %v", err)
	}
}
