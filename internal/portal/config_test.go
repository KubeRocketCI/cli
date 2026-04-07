package portal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

			_, err := FetchOIDCConfig(tt.portalURL)
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
			issuerURL, err := fetchOIDCConfig(srv.URL)

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

	_, err := FetchClusterConfig("http://portal.example.com", "token")
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

			cfg, err := fetchClusterConfig(srv.URL, "test-token")

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

func TestRestURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://portal.example.com/rest/v1/config/oidc",
		restURL("https://portal.example.com", "/v1/config/oidc"))
	assert.Equal(t, "https://portal.example.com/rest/v1/config",
		restURL("https://portal.example.com", "/v1/config"))
}
