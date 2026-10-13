package portal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

func TestNewServices_Clock(t *testing.T) {
	t.Parallel()

	before := time.Now()
	assert.False(t, NewServices(nil, Target{}).now().Before(before), "default clock is not time.Now")

	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s := NewServices(nil, Target{}, WithNow(func() time.Time { return fixed }))
	assert.Equal(t, fixed, s.now())
}

func TestServices_VerifySession(t *testing.T) {
	t.Parallel()

	const configJSON = `{"clusterName":"in-cluster","defaultNamespace":"platform","sonarWebUrl":"","dependencyTrackWebUrl":""}`

	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantErr     error
		wantErrMsg  string
	}{
		{name: "accepted", status: http.StatusOK, contentType: "application/json", body: configJSON},
		{name: "rejected", status: http.StatusUnauthorized, contentType: "application/json", body: configJSON,
			wantErr: ErrUnauthorized},
		{name: "rejected with malformed JSON", status: http.StatusUnauthorized, contentType: "application/json",
			body: `{"error":`, wantErr: ErrUnauthorized},
		{name: "forbidden with malformed JSON", status: http.StatusForbidden, contentType: "application/json",
			body: `{"error":`, wantErr: ErrPermissionDenied},
		{name: "200 page instead of config", status: http.StatusOK, contentType: "text/html; charset=utf-8",
			body: "<!doctype html><html></html>", wantErrMsg: "parsing cluster config: "},
		{name: "200 without body", status: http.StatusOK, wantErrMsg: "parsing cluster config: "},
		{name: "200 JSON null", status: http.StatusOK, contentType: "application/json", body: `null`,
			wantErrMsg: "parsing cluster config: response lacks"},
		{name: "200 empty JSON object", status: http.StatusOK, contentType: "application/json", body: `{}`,
			wantErrMsg: "parsing cluster config: response lacks"},
		{name: "200 unrelated JSON object", status: http.StatusOK, contentType: "application/json",
			body: `{"status":"ok"}`, wantErrMsg: "parsing cluster config: response lacks"},
		{name: "200 config with empty values", status: http.StatusOK, contentType: "application/json",
			body: `{"clusterName":"","defaultNamespace":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path

				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}

				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			client, err := restapi.NewClientWithResponses(srv.URL + "/rest")
			require.NoError(t, err)

			err = NewServices(client, Target{}).VerifySession(t.Context())
			assert.Equal(t, "/rest/v1/config", gotPath)

			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			case tt.wantErrMsg != "":
				require.ErrorContains(t, err, tt.wantErrMsg)
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestServices_VerifySession_TransportError(t *testing.T) {
	t.Parallel()

	cause := errors.New("dial refused")

	client, err := restapi.NewClientWithResponses("http://portal.invalid/rest",
		restapi.WithHTTPClient(failingDoer{err: cause}))
	require.NoError(t, err)

	err = NewServices(client, Target{}).VerifySession(t.Context())
	require.ErrorIs(t, err, cause)
	assert.ErrorContains(t, err, "requesting cluster config: ")
}

// failingDoer fails every request with err.
type failingDoer struct{ err error }

func (d failingDoer) Do(*http.Request) (*http.Response, error) { return nil, d.err }

// pathK8sResources prefixes the portal's Kubernetes resource endpoints.
const pathK8sResources = "/rest/v1/resources/"

// portalRequest is what TestServices_Accessors records of one request.
type portalRequest struct {
	path, cluster, namespace string
}

func TestServices_Accessors(t *testing.T) {
	t.Parallel()

	target := Target{Cluster: "core", Namespace: "krci"}

	tests := []struct {
		name string
		call func(context.Context, *Services) error
		// path is the exact request path, or pathK8sResources for any resource
		// endpoint scoped to the target.
		path string
	}{
		{
			name: "sonar",
			call: func(ctx context.Context, s *Services) error {
				_, err := s.Sonar().List(ctx, SonarListParams{Page: 1, PageSize: 50})
				return err
			},
			path: "/rest/v1/sonar/list",
		},
		{
			name: "sca",
			call: func(ctx context.Context, s *Services) error {
				_, err := s.SCA().List(ctx, SCAListParams{Page: 1, PageSize: 25})
				return err
			},
			path: "/rest/v1/sca/list",
		},
		{
			name: "project",
			call: func(ctx context.Context, s *Services) error {
				_, err := s.Project().List(ctx)
				return err
			},
			path: pathK8sResources,
		},
		{
			name: "project versions",
			call: func(ctx context.Context, s *Services) error {
				_, err := s.ProjectVersions().List(ctx, "my-app", "")
				return err
			},
			path: pathK8sResources,
		},
		{
			name: "deployment by project",
			call: func(ctx context.Context, s *Services) error {
				_, err := s.DeploymentByProject().List(ctx, "my-app")
				return err
			},
			path: pathK8sResources,
		},
		{
			name: "deployment",
			call: func(ctx context.Context, s *Services) error {
				_, err := s.Deployment().List(ctx)
				return err
			},
			path: pathK8sResources,
		},
		{
			name: "env",
			call: func(ctx context.Context, s *Services) error {
				_, err := s.Env().List(ctx, EnvListFilters{})
				return err
			},
			path: pathK8sResources,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				mu   sync.Mutex
				reqs []portalRequest
			)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					ClusterName string `json:"clusterName"`
					Namespace   string `json:"namespace"`
				}

				_ = json.NewDecoder(r.Body).Decode(&body)

				mu.Lock()
				reqs = append(reqs, portalRequest{path: r.URL.Path, cluster: body.ClusterName, namespace: body.Namespace})
				mu.Unlock()

				w.WriteHeader(http.StatusUnauthorized)
			}))
			t.Cleanup(srv.Close)

			client, err := restapi.NewClientWithResponses(srv.URL + "/rest")
			require.NoError(t, err)

			require.ErrorIs(t, tt.call(t.Context(), NewServices(client, target)), ErrUnauthorized)

			mu.Lock()
			defer mu.Unlock()

			require.NotEmpty(t, reqs)

			for _, req := range reqs {
				if tt.path != pathK8sResources {
					assert.Equal(t, tt.path, req.path)

					continue
				}

				assert.True(t, strings.HasPrefix(req.path, pathK8sResources), "path %q", req.path)
				assert.Equal(t, target.Cluster, req.cluster, req.path)
				assert.Equal(t, target.Namespace, req.namespace, req.path)
			}
		})
	}
}

// portalURL and the clock never reach the wire; the cmd/krci contract stub
// covers namespace and clusterName per route.
func TestServices_PipelineRunAccessors(t *testing.T) {
	t.Parallel()

	client, err := restapi.NewClientWithResponses("https://portal.example.com/rest")
	require.NoError(t, err)

	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	target := Target{PortalURL: "https://portal.example.com", Cluster: "core", Namespace: "krci"}
	s := NewServices(client, target, WithNow(func() time.Time { return fixed }))

	run := s.PipelineRun()
	assert.Same(t, client, run.client)
	assert.Equal(t, target.PortalURL, run.portalURL)
	assert.Equal(t, target.Cluster, run.clusterName)
	assert.Equal(t, target.Namespace, run.namespace)
	assert.Equal(t, fixed, run.now())

	start := s.PipelineRunStart()
	assert.Same(t, client, start.client)
	assert.Equal(t, target.Namespace, start.namespace)

	build := s.ProjectBuild()
	assert.Same(t, client, build.client)
	assert.Equal(t, target.Namespace, build.namespace)
}
