package cmdutil

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/token"
)

func newTestFactory() *Factory {
	return New(iostreams.New(nil, io.Discard, io.Discard, false))
}

func TestFactoryConfig_DefaultResolverFails(t *testing.T) {
	t.Parallel()

	_, err := newTestFactory().Config()

	require.Error(t, err)
	assert.EqualError(t, err, "loading config: config resolver not set")
}

func TestFactoryConfig_CopiedFuncSeesResolver(t *testing.T) {
	t.Parallel()

	f := newTestFactory()
	captured := f.Config

	f.SetConfigResolver(func() (*config.Config, error) {
		return &config.Config{PortalURL: "https://p.example"}, nil
	})

	cfg, err := captured()
	require.NoError(t, err)
	assert.Equal(t, "https://p.example", cfg.PortalURL)
}

func TestFactoryConfig_MemoizesSuccess(t *testing.T) {
	t.Parallel()

	f := newTestFactory()
	calls := 0

	f.SetConfigResolver(func() (*config.Config, error) {
		calls++

		return &config.Config{PortalURL: "https://first.example"}, nil
	})

	first, err := f.Config()
	require.NoError(t, err)

	f.SetConfigResolver(func() (*config.Config, error) {
		calls++

		return &config.Config{PortalURL: "https://second.example"}, nil
	})

	second, err := f.Config()
	require.NoError(t, err)

	assert.Same(t, first, second)
	assert.Equal(t, "https://first.example", second.PortalURL)
	assert.Equal(t, 1, calls)
}

func TestFactoryLazyFuncs_ReadConfigAtCallTime(t *testing.T) {
	t.Parallel()

	f := newTestFactory()
	tokenProvider, restClient := f.TokenProvider, f.RestClient
	cause := errors.New("resolver set after New")

	f.SetConfigResolver(func() (*config.Config, error) { return nil, cause })

	_, err := tokenProvider()
	require.ErrorIs(t, err, cause)

	_, err = restClient()
	require.ErrorIs(t, err, cause)
}

func TestFactoryTokenProvider_WritesTokenDiagnosticsToErrOut(t *testing.T) {
	t.Setenv("KRCI_KEYRING_BACKEND", "file")
	t.Setenv("KRCI_TOKEN", "")

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "tokens.enc")
	require.NoError(t, os.WriteFile(tokenPath, bytes.Repeat([]byte("x"), 64), 0o600))

	keyPath := filepath.Join(dir, ".keyfile")
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32))
	require.NoError(t, os.WriteFile(keyPath, []byte(encoded), 0o644))
	require.NoError(t, os.Chmod(keyPath, 0o644))

	var errOut bytes.Buffer

	f := New(iostreams.New(nil, io.Discard, &errOut, false))
	f.SetConfigResolver(func() (*config.Config, error) {
		return &config.Config{ConfigDir: dir, TokenPath: tokenPath, KeyringService: "krci-test"}, nil
	})

	tp, err := f.TokenProvider()
	require.NoError(t, err)

	_, err = tp.GetToken(t.Context())
	require.ErrorIs(t, err, auth.ErrNotAuthenticated)

	assert.Contains(t, errOut.String(), "Warning: removing undecryptable token file ("+tokenPath+")")
	assert.Contains(t, errOut.String(), "  Run 'krci auth login' to re-authenticate.\n")

	if runtime.GOOS != "windows" {
		assert.Contains(t, errOut.String(), "has permissions 0644")
	}
}

func TestFactoryTokenProvider_WritesAuthDiagnosticsToErrOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directories do not block writes")
	}

	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	t.Setenv("KRCI_KEYRING_BACKEND", "file")
	t.Setenv("KRCI_TOKEN", "")

	idp := newRefreshIDP(t)

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "tokens.enc")

	keyPath := filepath.Join(dir, ".keyfile")
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32))
	require.NoError(t, os.WriteFile(keyPath, []byte(encoded), 0o600))

	const keyringService = "krci-test"

	enc := token.NewAESEncryptor(keyringService, dir, io.Discard)
	require.NoError(t, token.NewEncryptedStore(tokenPath, enc, io.Discard).Save(&token.StoredToken{
		AccessToken:  "expired",
		RefreshToken: "old-refresh",
		IDToken:      "old-id",
		ExpiresAt:    time.Now().Add(-time.Minute),
		IssuerURL:    idp.URL,
		ClientID:     "krci-cli",
		PortalURL:    "https://portal.example.com",
	}))

	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	var out, errOut bytes.Buffer

	f := New(iostreams.New(nil, &out, &errOut, false))
	f.SetConfigResolver(func() (*config.Config, error) {
		return &config.Config{
			PortalURL:      "https://portal.example.com",
			IssuerURL:      idp.URL,
			ClientID:       "krci-cli",
			ConfigDir:      dir,
			TokenPath:      tokenPath,
			KeyringService: keyringService,
		}, nil
	})

	tp, err := f.TokenProvider()
	require.NoError(t, err)

	tok, err := tp.GetToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)
	require.Equal(t, "new-id", tok)

	assert.Contains(t, errOut.String(), "Warning: failed to persist refreshed credentials: ")
	assert.Contains(t, errOut.String(), "You may need to re-authenticate on next use.\n")
	assert.Empty(t, out.String())
}

// newRefreshIDP serves OIDC discovery and a refresh_token grant over TLS.
func newRefreshIDP(t *testing.T) *httptest.Server {
	t.Helper()

	var idp *httptest.Server

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 idp.URL,
			"authorization_endpoint": idp.URL + "/auth",
			"token_endpoint":         idp.URL + "/token",
			"jwks_uri":               idp.URL + "/keys",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"token_type":    "Bearer",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
			"id_token":      "new-id",
		})
	})

	idp = httptest.NewTLSServer(mux)
	t.Cleanup(idp.Close)

	return idp
}

func TestFactoryConfig_DoesNotCacheFailure(t *testing.T) {
	t.Parallel()

	f := newTestFactory()
	cause := errors.New("parse failure")

	f.SetConfigResolver(func() (*config.Config, error) { return nil, cause })

	_, err := f.Config()
	require.ErrorIs(t, err, cause)
	assert.ErrorContains(t, err, "loading config: ")

	f.SetConfigResolver(func() (*config.Config, error) { return &config.Config{Namespace: "ns"}, nil })

	cfg, err := f.Config()
	require.NoError(t, err)
	assert.Equal(t, "ns", cfg.Namespace)
}

// stubTokenProvider returns a fixed token.
type stubTokenProvider struct{ auth.TokenProvider }

func (stubTokenProvider) GetToken(context.Context) (string, error) { return "test-token", nil }

func newGatedFactory(portalURL string) *Factory {
	f := newTestFactory()
	f.SetConfigResolver(func() (*config.Config, error) {
		return &config.Config{PortalURL: portalURL, ClusterName: "c", Namespace: "ns"}, nil
	})
	f.TokenProvider = func() (auth.TokenProvider, error) { return stubTokenProvider{}, nil }

	return f
}

func TestFactoryRestClient_RequiresHTTPSOrLoopback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		portalURL string
		wantErr   error
	}{
		{portalURL: "https://portal.example.com"},
		{portalURL: "http://127.0.0.1:3001"},
		{portalURL: "http://portal.example.com", wantErr: portal.ErrInvalidPortalURL},
		{portalURL: "https://user@portal.example.com", wantErr: portal.ErrInvalidPortalURL},
	}

	for _, tt := range tests {
		t.Run(tt.portalURL, func(t *testing.T) {
			t.Parallel()

			client, err := newGatedFactory(tt.portalURL).RestClient()

			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.NotNil(t, client)

				return
			}

			require.ErrorIs(t, err, tt.wantErr)
			assert.Nil(t, client)
		})
	}
}

func TestFactoryRestClient_RefusesCrossHostRedirect(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	client, err := newGatedFactory(origin.URL).RestClient()
	require.NoError(t, err)

	_, err = client.ConfigGetWithResponse(t.Context())
	require.ErrorIs(t, err, portal.ErrRedirectRefused)
	assert.Zero(t, hits.Load(), "redirect target was requested")
}

// TestNew_SetsEveryFuncField: every exported func field of a Factory built by
// New is non-nil.
func TestNew_SetsEveryFuncField(t *testing.T) {
	t.Parallel()

	v := reflect.ValueOf(newTestFactory()).Elem()
	checked := 0

	for i := range v.NumField() {
		field := v.Type().Field(i)
		if !field.IsExported() || field.Type.Kind() != reflect.Func {
			continue
		}

		checked++

		if v.Field(i).IsNil() {
			t.Errorf("Factory.%s is nil", field.Name)
		}
	}

	if checked == 0 {
		t.Error("Factory has no exported func fields to check")
	}
}

func TestFactoryRestClient_UnboundStoredTokenNeverReachesPortal(t *testing.T) {
	t.Setenv("KRCI_KEYRING_BACKEND", "file")
	t.Setenv("KRCI_TOKEN", "")

	var hits atomic.Int32

	portalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(portalSrv.Close)

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "tokens.enc")

	keyPath := filepath.Join(dir, ".keyfile")
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32))
	require.NoError(t, os.WriteFile(keyPath, []byte(encoded), 0o600))

	const keyringService = "krci-test"

	enc := token.NewAESEncryptor(keyringService, dir, io.Discard)
	require.NoError(t, token.NewEncryptedStore(tokenPath, enc, io.Discard).Save(&token.StoredToken{
		IDToken:   "unbound-id",
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	f := newTestFactory()
	f.SetConfigResolver(func() (*config.Config, error) {
		return &config.Config{
			PortalURL:      portalSrv.URL,
			ClusterName:    "core",
			Namespace:      "krci",
			ConfigDir:      dir,
			TokenPath:      tokenPath,
			KeyringService: keyringService,
		}, nil
	})

	client, err := f.RestClient()
	require.NoError(t, err)

	_, err = client.ConfigGetWithResponse(t.Context())
	require.ErrorIs(t, err, auth.ErrNotAuthenticated)
	assert.Zero(t, hits.Load(), "an unbound stored token must not produce a portal request")
}
