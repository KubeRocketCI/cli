package portal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchConfig_RequiresHTTPS(t *testing.T) {
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

			_, err := FetchConfig(tt.portalURL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "portal URL must use HTTPS")
		})
	}
}

func TestFetchConfig_Integration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		handler         http.HandlerFunc
		wantErr         bool
		wantClusterName string
		wantDefaultNS   string
	}{
		{
			name: "SuperJSON format",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"result":{"data":{"json":{"clusterName":"eks","defaultNamespace":"edp"}}}}`))
			},
			wantClusterName: "eks",
			wantDefaultNS:   "edp",
		},
		{
			name: "plain format",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"result":{"data":{"clusterName":"in-cluster","defaultNamespace":"platform"}}}`))
			},
			wantClusterName: "in-cluster",
			wantDefaultNS:   "platform",
		},
		{
			name: "HTTP error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`not json`))
			},
			wantErr: true,
		},
		{
			name: "empty data",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`{"result":{"data":{}}}`))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			cfg, err := fetchConfig(srv.URL)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantClusterName, cfg.ClusterName)
			assert.Equal(t, tt.wantDefaultNS, cfg.DefaultNamespace)
		})
	}
}

func TestFetchConfig_RequestPath(t *testing.T) {
	t.Parallel()

	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"result":{"data":{"clusterName":"c","defaultNamespace":"n"}}}`))
	}))
	defer srv.Close()

	_, err := fetchConfig(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "/api/config.get", gotPath)
}

func TestParseConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    []byte
		wantErr bool
		wantNS  string
	}{
		{
			name:   "SuperJSON",
			body:   []byte(`{"result":{"data":{"json":{"clusterName":"eks","defaultNamespace":"edp"}}}}`),
			wantNS: "edp",
		},
		{
			name:   "plain",
			body:   []byte(`{"result":{"data":{"clusterName":"c","defaultNamespace":"ns"}}}`),
			wantNS: "ns",
		},
		{
			name:    "malformed JSON",
			body:    []byte(`{broken`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := parseConfig(tt.body)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantNS, cfg.DefaultNamespace)
		})
	}
}
