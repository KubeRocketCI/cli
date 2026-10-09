package login

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
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
		return func(_ context.Context, _, _ string) (*portal.ClusterConfig, error) {
			return &portal.ClusterConfig{ClusterName: cluster, DefaultNamespace: ns}, nil
		}
	}

	errFetcher := func(_ context.Context, _, _ string) (*portal.ClusterConfig, error) {
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

type ctxKey struct{}

func TestPopulateClusterConfig_PassesContextToFetch(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")

	var got context.Context

	fetch := func(fetchCtx context.Context, _, _ string) (*portal.ClusterConfig, error) {
		got = fetchCtx

		return &portal.ClusterConfig{ClusterName: "c", DefaultNamespace: "ns"}, nil
	}

	cfg := config.Config{PortalURL: "https://portal.test"}
	populateClusterConfig(ctx, &mockTokenProvider{token: "tok"}, &cfg, io.Discard, fetch)

	if assert.NotNil(t, got, "fetch was not called") {
		assert.Equal(t, "marker", got.Value(ctxKey{}))
	}
}

func TestLoginRun_CancelledAfterLoginSavesConfigAndReturnsCtxErr(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		PortalURL:   "https://portal.test",
		IssuerURL:   "https://issuer.test/realms/krci",
		ClusterName: "c",
		Namespace:   "ns",
		ConfigDir:   t.TempDir(),
	}

	var errOut bytes.Buffer

	opts := &LoginOptions{
		IO:            iostreams.FromWriters(nil, io.Discard, &errOut),
		Config:        func() (*config.Config, error) { return cfg, nil },
		TokenProvider: func() (auth.TokenProvider, error) { return &mockTokenProvider{token: "tok"}, nil },
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	err := loginRun(cmd, opts)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, errOut.String())

	saved, readErr := os.ReadFile(filepath.Join(cfg.ConfigDir, "config.yaml"))
	if assert.NoError(t, readErr) {
		assert.Contains(t, string(saved), "portal-url: https://portal.test")
	}
}

func TestPopulateClusterConfig_CancelledPrintsNoWarnings(t *testing.T) {
	t.Parallel()

	cancelledFetch := func(ctx context.Context, _, _ string) (*portal.ClusterConfig, error) {
		return nil, ctx.Err()
	}

	tests := map[string]auth.TokenProvider{
		"token error": &mockTokenProvider{tokenErr: context.Canceled},
		"fetch error": &mockTokenProvider{token: "tok"},
	}

	for name, tp := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			cfg := config.Config{PortalURL: "https://portal.test"}

			var buf bytes.Buffer
			populateClusterConfig(ctx, tp, &cfg, &buf, cancelledFetch)

			assert.Empty(t, buf.String())
			assert.Empty(t, cfg.Namespace)
		})
	}
}
