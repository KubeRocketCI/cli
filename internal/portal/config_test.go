package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

func TestFetchOIDCConfig_RequiresHTTPS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		portalURL string
	}{
		{name: "http scheme", portalURL: "http://portal.example.com"},
		{name: "no scheme", portalURL: "portal.example.com"},
		{name: "empty host", portalURL: "https://"},
		{name: "empty string", portalURL: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := FetchOIDCConfig(t.Context(), tt.portalURL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "portal URL must use HTTPS")
		})
	}
}

func TestFetchOIDCConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantErr    bool
		wantErrMsg string
		wantURL    string
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/rest/v1/config/oidc", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"oidcIssuerUrl":"https://auth.example.com"}`))
			},
			wantURL: "https://auth.example.com",
		},
		{
			name: "HTTP error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`internal error`))
			},
			wantErr:    true,
			wantErrMsg: "HTTP 500",
		},
		{
			name: "invalid JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`not json`))
			},
			wantErr:    true,
			wantErrMsg: "parsing OIDC config",
		},
		{
			name: "empty issuer URL",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"oidcIssuerUrl":""}`))
			},
			wantErr:    true,
			wantErrMsg: "empty OIDC issuer URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			// Use http:// for test server (validatePortalURL requires https, so bypass it)
			issuerURL, err := fetchOIDCConfig(t.Context(), srv.URL)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, issuerURL)
		})
	}
}

func TestFetchClusterConfig_RequiresHTTPS(t *testing.T) {
	t.Parallel()

	_, err := FetchClusterConfig(t.Context(), "http://portal.example.com", "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "portal URL must use HTTPS")
}

func TestFetchClusterConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantErr     bool
		wantErrMsg  string
		wantCluster string
		wantNS      string
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/rest/v1/config", r.URL.Path)
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"clusterName":"in-cluster","defaultNamespace":"platform","sonarWebUrl":"","dependencyTrackWebUrl":""}`))
			},
			wantCluster: "in-cluster",
			wantNS:      "platform",
		},
		{
			name: "unauthorized",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr:    true,
			wantErrMsg: "unauthorized",
		},
		{
			name: "invalid JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`not json`))
			},
			wantErr:    true,
			wantErrMsg: "parsing cluster config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			cfg, err := fetchClusterConfig(t.Context(), srv.URL, "test-token")

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCluster, cfg.ClusterName)
			assert.Equal(t, tt.wantNS, cfg.DefaultNamespace)
		})
	}
}

// fetchers adapts both fetches to one signature. cluster uses a fixed token.
var fetchers = map[string]struct {
	fetch      func(ctx context.Context, portalURL string) error
	errPrefix  string
	urlPathEnd string
}{
	"OIDC": {
		fetch: func(ctx context.Context, portalURL string) error {
			_, err := fetchOIDCConfig(ctx, portalURL)
			return err
		},
		errPrefix:  "requesting OIDC config: Get ",
		urlPathEnd: "/rest/v1/config/oidc",
	},
	"cluster": {
		fetch: func(ctx context.Context, portalURL string) error {
			_, err := fetchClusterConfig(ctx, portalURL, "test-token")
			return err
		},
		errPrefix:  "requesting cluster config: Get ",
		urlPathEnd: "/rest/v1/config",
	},
}

func TestFetchConfig_RequestErrorText(t *testing.T) {
	t.Parallel()

	for name, tc := range fetchers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.NotFoundHandler())
			portalURL := srv.URL
			srv.Close()

			err := tc.fetch(t.Context(), portalURL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errPrefix+`"`+portalURL+tc.urlPathEnd+`": `)
		})
	}
}

func TestFetchConfig_CancelInFlight(t *testing.T) {
	t.Parallel()

	for name, tc := range fetchers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			started := make(chan struct{})
			release := make(chan struct{})

			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				close(started)

				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			// Cleanups run last-in first-out: the handler is released before the server closes.
			t.Cleanup(srv.Close)
			t.Cleanup(func() { close(release) })

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			errCh := make(chan error, 1)

			go func() { errCh <- tc.fetch(ctx, srv.URL) }()

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach the server")
			}

			cancel()

			// Client.Timeout surfaces as DeadlineExceeded; Canceled proves the
			// caller's ctx ended the request.
			select {
			case err := <-errCh:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("fetch did not return within 5s of cancellation")
			}
		})
	}
}

func TestRestURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://portal.example.com/rest/v1/config/oidc",
		restURL("https://portal.example.com", "/v1/config/oidc"))
	assert.Equal(t, "https://portal.example.com/rest/v1/config",
		restURL("https://portal.example.com", "/v1/config"))
}

func TestVerifySession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{name: "accepted", status: http.StatusOK},
		{name: "rejected", status: http.StatusUnauthorized, wantErr: ErrUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"clusterName":"in-cluster","defaultNamespace":"platform","sonarWebUrl":"","dependencyTrackWebUrl":""}`))
			}))
			defer srv.Close()

			// httptest serves plain HTTP: the check follows the configured
			// portal URL like every other portal call.
			client, err := restapi.NewClientWithResponses(srv.URL + "/rest")
			require.NoError(t, err)

			err = VerifySession(context.Background(), client)
			assert.Equal(t, "/rest/v1/config", gotPath)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}
