// Package envtestutil holds what the tests of the `krci env` verbs that take
// <deployment> <env> share: a mock portal and the checks every such verb
// passes.
package envtestutil

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
)

// The environment Portal serves.
const (
	Deployment   = "my-pipeline"
	Env          = "dev"
	Namespace    = "my-pipeline-dev"
	LocalCluster = "in-cluster"
)

// newCmd is the constructor of a verb, as cmdtest takes it.
type newCmd[T any] func(*cmdutil.Factory, func(*T) error) *cobra.Command

// Portal returns a mock portal with one deployment and its one environment on
// the given cluster. A list of kind answers with items (comma-separated JSON
// objects), a list of any other kind is empty, and any other deployment is not
// found.
func Portal(cluster, kind, items string) http.Handler {
	lists := map[string]string{
		"Stage": `{"metadata":{"name":"` + Namespace + `"},"spec":{"name":"` + Env + `","cdPipeline":"` + Deployment +
			`","clusterName":"` + cluster + `","namespace":"` + Namespace + `"}}`,
		kind: items,
	}
	pipeline := `{"apiVersion":"v2.edp.epam.com/v1","kind":"CDPipeline","metadata":{"name":"` + Deployment +
		`"},"spec":{"applications":["foo"]}}`

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name           string `json:"name"`
			ResourceConfig struct {
				Kind string `json:"kind"`
			} `json:"resourceConfig"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/v1/resources/list":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"List","metadata":{},"items":[` +
				lists[req.ResourceConfig.Kind] + `]}`))
		case r.URL.Path == "/v1/resources/get" && req.Name == Deployment:
			_, _ = w.Write([]byte(pipeline))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// CheckInvalidTarget checks that the verb rejects a wrong number of
// positionals, an invalid deployment or env and an unknown output format
// before its run function.
func CheckInvalidTarget[T any](t *testing.T, cmd newCmd[T]) {
	t.Helper()

	cases := map[string]struct {
		args []string
		want string
	}{
		"no positionals":     {args: []string{}, want: "requires a deployment and an env"},
		"one positional":     {args: []string{"only-one"}, want: "requires a deployment and an env"},
		"three positionals":  {args: []string{"a", "b", "c"}, want: "requires a deployment and an env"},
		"invalid deployment": {args: []string{"BAD_NAME", Env}, want: "<deployment> must be a valid DNS-1123 name"},
		"invalid env":        {args: []string{Deployment, "Bad_Env"}, want: "<env> must be a valid DNS-1123 name"},
		"unknown format":     {args: []string{Deployment, Env, "-o", "yaml"}, want: "unknown output format"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts, err := cmdtest.RunCmd(t, cmd, tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}

			if opts != nil {
				t.Error("the run function must not be reached")
			}
		})
	}
}

// CheckNotFound checks the message of the verb for a missing deployment and
// for a missing environment, on stderr and in the JSON error envelope. kind
// and items are what Portal serves in the environment namespace.
func CheckNotFound[T any](t *testing.T, cmd newCmd[T], kind, items string) {
	t.Helper()

	cases := map[string]struct {
		args []string
		want string
	}{
		"unknown deployment": {
			args: []string{"nope", Env},
			want: `deployment "nope" not found`,
		},
		"unknown environment": {
			args: []string{Deployment, "wat"},
			want: `environment "wat" not found in deployment "` + Deployment + `"`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmdtest.CheckErrorOutput(t, cmd, Portal(LocalCluster, kind, items), tc.args, discovery.SchemaVersion, tc.want)
		})
	}
}

// CheckRemoteCluster checks that the verb refuses an environment on another
// cluster with a message that names the cluster.
func CheckRemoteCluster[T any](t *testing.T, cmd newCmd[T], kind, items string) {
	t.Helper()

	want := `environment "` + Env + `" of deployment "` + Deployment + `" runs on cluster "prod-cluster"; ` +
		`the Portal reads pods and events only on its own cluster`

	cmdtest.CheckErrorOutput(t, cmd, Portal("prod-cluster", kind, items),
		[]string{Deployment, Env}, discovery.SchemaVersion, want)
}
