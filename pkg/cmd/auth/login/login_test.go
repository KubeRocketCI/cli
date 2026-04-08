package login

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// mockTokenProvider implements auth.TokenProvider for testing.
type mockTokenProvider struct {
	token    string
	tokenErr error
}

func (m *mockTokenProvider) GetToken(_ context.Context) (string, error) {
	return m.token, m.tokenErr
}

func (m *mockTokenProvider) Login(_ context.Context) error     { return nil }
func (m *mockTokenProvider) Logout() error                     { return nil }
func (m *mockTokenProvider) UserInfo() (*auth.UserInfo, error) { return nil, nil }

func TestPopulateClusterConfig(t *testing.T) {
	t.Parallel()

	okFetcher := func(cluster, ns string) clusterFetchFunc {
		return func(_, _ string) (*portal.ClusterConfig, error) {
			return &portal.ClusterConfig{ClusterName: cluster, DefaultNamespace: ns}, nil
		}
	}

	errFetcher := func(_, _ string) (*portal.ClusterConfig, error) {
		return nil, errors.New("connection refused")
	}

	tests := []struct {
		name         string
		cfg          config.Config
		tp           auth.TokenProvider
		fetcher      clusterFetchFunc
		wantCluster  string
		wantNS       string
		wantOutput   []string
		wantNoOutput []string
	}{
		{
			name: "both already set — skips fetch",
			cfg: config.Config{
				ClusterName: "existing",
				Namespace:   "existing-ns",
			},
			tp:           &mockTokenProvider{token: "tok"},
			fetcher:      errFetcher,
			wantCluster:  "existing",
			wantNS:       "existing-ns",
			wantNoOutput: []string{"Warning"},
		},
		{
			name:        "fetch succeeds — populates both",
			cfg:         config.Config{PortalURL: "https://portal.test"},
			tp:          &mockTokenProvider{token: "tok"},
			fetcher:     okFetcher("discovered", "discovered-ns"),
			wantCluster: "discovered",
			wantNS:      "discovered-ns",
			wantOutput:  []string{"Namespace: discovered-ns (from portal)"},
		},
		{
			name:        "fetch succeeds — does not overwrite existing cluster name",
			cfg:         config.Config{PortalURL: "https://portal.test", ClusterName: "existing"},
			tp:          &mockTokenProvider{token: "tok"},
			fetcher:     okFetcher("discovered", "discovered-ns"),
			wantCluster: "existing",
			wantNS:      "discovered-ns",
		},
		{
			name:        "fetch succeeds — does not overwrite existing namespace",
			cfg:         config.Config{PortalURL: "https://portal.test", Namespace: "existing-ns"},
			tp:          &mockTokenProvider{token: "tok"},
			fetcher:     okFetcher("discovered", "other-ns"),
			wantCluster: "discovered",
			wantNS:      "existing-ns",
		},
		{
			name:        "token error — warns and emits namespace warning",
			cfg:         config.Config{PortalURL: "https://portal.test"},
			tp:          &mockTokenProvider{tokenErr: errors.New("token expired")},
			fetcher:     errFetcher,
			wantCluster: "",
			wantNS:      "",
			wantOutput: []string{
				"Warning: could not get token",
				"Warning: namespace not configured",
			},
		},
		{
			name:        "fetch error — warns and emits namespace warning",
			cfg:         config.Config{PortalURL: "https://portal.test"},
			tp:          &mockTokenProvider{token: "tok"},
			fetcher:     errFetcher,
			wantCluster: "",
			wantNS:      "",
			wantOutput: []string{
				"Warning: could not fetch cluster config",
				"Warning: namespace not configured",
			},
		},
		{
			name:        "invalid namespace from portal — warns",
			cfg:         config.Config{PortalURL: "https://portal.test"},
			tp:          &mockTokenProvider{token: "tok"},
			fetcher:     okFetcher("c", "INVALID NS!"),
			wantCluster: "c",
			wantNS:      "",
			wantOutput: []string{
				"Warning: portal returned invalid namespace",
				"Warning: namespace not configured",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := tt.cfg
			var buf bytes.Buffer
			populateClusterConfig(context.Background(), tt.tp, &cfg, &buf, tt.fetcher)

			assert.Equal(t, tt.wantCluster, cfg.ClusterName, "ClusterName")
			assert.Equal(t, tt.wantNS, cfg.Namespace, "Namespace")

			output := buf.String()
			for _, want := range tt.wantOutput {
				assert.Contains(t, output, want)
			}
			for _, notWant := range tt.wantNoOutput {
				assert.NotContains(t, output, notWant)
			}
		})
	}
}
