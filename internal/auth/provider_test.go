package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/token"
)

// fakeJWT builds an unsigned alg:none test JWT from a raw JSON payload.
func fakeJWT(payload string) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	p := base64.RawURLEncoding.EncodeToString([]byte(payload))

	return h + "." + p + "."
}

// mockStore implements token.Store for testing. It is safe for concurrent use
// and counts Load calls.
type mockStore struct {
	mu      sync.Mutex
	tok     *token.StoredToken
	loadErr error
	saveErr error
	cleared bool
	loads   int
}

func (m *mockStore) Save(tok *token.StoredToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.saveErr != nil {
		return m.saveErr
	}
	m.tok = tok
	return nil
}

func (m *mockStore) Load() (*token.StoredToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.loads++
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.tok == nil {
		return nil, token.ErrNoToken
	}
	return m.tok, nil
}

func (m *mockStore) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tok = nil
	m.cleared = true
	return nil
}

func (m *mockStore) loadCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.loads
}

// TestGetTokenEnvVarTakesPrecedence cannot use t.Parallel because t.Setenv
// is incompatible with parallel tests.
func TestGetTokenEnvVarTakesPrecedence(t *testing.T) {
	t.Setenv("KRCI_TOKEN", "env-token-value")

	store := &mockStore{
		tok: &token.StoredToken{IDToken: "cached-id-token", ExpiresAt: time.Now().Add(time.Hour)},
	}

	tp := NewTokenProvider(store, &config.Config{}, io.Discard)
	tok, err := tp.GetToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "env-token-value", tok)
}

func TestGetTokenEnvVarExpired(t *testing.T) {
	expired := time.Now().Add(-time.Minute).Unix()
	t.Setenv("KRCI_TOKEN", fakeJWT(fmt.Sprintf(`{"sub":"ci","exp":%d}`, expired)))

	store := &mockStore{
		tok: &token.StoredToken{IDToken: "cached-id-token", ExpiresAt: time.Now().Add(time.Hour)},
	}

	tp := NewTokenProvider(store, &config.Config{}, io.Discard)
	_, err := tp.GetToken(context.Background())
	require.ErrorIs(t, err, ErrEnvTokenExpired)
}

func TestGetTokenEnvVarUnexpiredJWT(t *testing.T) {
	valid := time.Now().Add(time.Hour).Unix()
	envTok := fakeJWT(fmt.Sprintf(`{"sub":"ci","exp":%d}`, valid))
	t.Setenv("KRCI_TOKEN", envTok)

	tp := NewTokenProvider(&mockStore{}, &config.Config{}, io.Discard)
	tok, err := tp.GetToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, envTok, tok)
}

func TestUserInfoFromEnvToken(t *testing.T) {
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	t.Setenv("KRCI_TOKEN", fakeJWT(fmt.Sprintf(`{"email":"ci@example.com","groups":["developers"],"exp":%d}`, exp.Unix())))

	store := &mockStore{
		tok: &token.StoredToken{
			IDToken:   fakeJWT(`{"email":"stored@example.com"}`),
			ExpiresAt: time.Now().Add(2 * time.Hour),
		},
	}

	tp := NewTokenProvider(store, &config.Config{}, io.Discard)
	info, err := tp.UserInfo()
	require.NoError(t, err)

	assert.Equal(t, "ci@example.com", info.Email)
	assert.Equal(t, []string{"developers"}, info.Groups)
	assert.True(t, exp.Equal(info.ExpiresAt), "ExpiresAt = %v, want %v", info.ExpiresAt, exp)
	assert.True(t, info.FromEnv, "claims of KRCI_TOKEN must be marked FromEnv")
}

func TestUserInfoFromOpaqueEnvToken(t *testing.T) {
	t.Setenv("KRCI_TOKEN", "opaque-token")

	tp := NewTokenProvider(&mockStore{}, &config.Config{}, io.Discard)
	_, err := tp.UserInfo()
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotAuthenticated)
}

func TestGetToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		store   *mockStore
		wantTok string
		wantErr error
	}{
		{
			name: "from cache",
			store: &mockStore{
				tok: &token.StoredToken{
					IDToken:   "cached-id-token",
					ExpiresAt: time.Now().Add(time.Hour),
					PortalURL: portalA,
				},
			},
			wantTok: "cached-id-token",
		},
		{
			name:    "not authenticated",
			store:   &mockStore{}, // nil token -> ErrNoToken
			wantErr: ErrNotAuthenticated,
		},
		{
			name: "expired no refresh",
			store: &mockStore{
				tok: &token.StoredToken{
					AccessToken:  "expired",
					RefreshToken: "", // no refresh token
					ExpiresAt:    time.Now().Add(-time.Minute),
					PortalURL:    portalA,
				},
			},
			wantErr: ErrTokenExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTokenProvider(tt.store, &config.Config{PortalURL: portalA}, io.Discard)
			tok, err := tp.GetToken(context.Background())

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantTok, tok)
		})
	}
}

const (
	portalA = "https://portal-a.example.com"
	portalB = "https://portal-b.example.com"

	oldRefreshToken = "old-refresh"
)

func TestGetTokenPortalBinding(t *testing.T) {
	t.Parallel()

	const loginHint = "not authenticated: run 'krci auth login'"

	valid := func(portalURL string) *token.StoredToken {
		return &token.StoredToken{
			IDToken:   "cached-id-token",
			ExpiresAt: time.Now().Add(time.Hour),
			PortalURL: portalURL,
		}
	}

	tests := []struct {
		name      string
		stored    *token.StoredToken
		cfgPortal string
		wantTok   string
		wantErr   bool
	}{
		{name: "bound to the configured portal", stored: valid(portalA), cfgPortal: portalA, wantTok: "cached-id-token"},
		{name: "unbound token is rejected", stored: valid(""), cfgPortal: portalB, wantErr: true},
		{name: "bound to another portal", stored: valid(portalA), cfgPortal: portalB, wantErr: true},
		{name: "bound, no portal configured", stored: valid(portalA), cfgPortal: "", wantErr: true},
		{
			// Non-HTTPS issuer: any refresh attempt fails with ErrRefreshFailed.
			name: "expired and bound to another portal",
			stored: &token.StoredToken{
				RefreshToken: oldRefreshToken,
				ExpiresAt:    time.Now().Add(-time.Minute),
				IssuerURL:    "http://idp.invalid",
				ClientID:     "krci-cli",
				PortalURL:    portalA,
			},
			cfgPortal: portalB,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &mockStore{tok: tt.stored}
			tp := NewTokenProvider(store, &config.Config{PortalURL: tt.cfgPortal}, io.Discard)

			tok, err := tp.GetToken(context.Background())

			if tt.wantErr {
				require.ErrorIs(t, err, ErrNotAuthenticated)
				assert.NotErrorIs(t, err, ErrRefreshFailed)
				assert.EqualError(t, err, loginHint)
				assert.Same(t, tt.stored, store.tok, "a rejected token must be left in the store")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantTok, tok)
		})
	}
}

func TestGetTokenEnvVarIgnoresPortalBinding(t *testing.T) {
	t.Setenv("KRCI_TOKEN", "env-token-value")

	store := &mockStore{
		tok: &token.StoredToken{
			IDToken:   "cached-id-token",
			ExpiresAt: time.Now().Add(time.Hour),
			PortalURL: portalA,
		},
	}

	tp := NewTokenProvider(store, &config.Config{PortalURL: portalB}, io.Discard)
	tok, err := tp.GetToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "env-token-value", tok)
}

func TestGetTokenLoadError(t *testing.T) {
	t.Parallel()

	store := &mockStore{loadErr: errors.New("disk failure")}

	tp := NewTokenProvider(store, &config.Config{}, io.Discard)
	_, err := tp.GetToken(context.Background())

	require.Error(t, err)

	// Should NOT be ErrNotAuthenticated -- it's a different kind of error.
	assert.False(t, errors.Is(err, ErrNotAuthenticated), "load failure should not be classified as ErrNotAuthenticated")
}

func TestLogout(t *testing.T) {
	t.Parallel()

	store := &mockStore{
		tok: &token.StoredToken{AccessToken: "test"},
	}

	tp := NewTokenProvider(store, &config.Config{}, io.Discard)
	require.NoError(t, tp.Logout())
	assert.True(t, store.cleared, "Logout() did not call Clear()")
}

func TestUserInfoNotAuthenticated(t *testing.T) {
	t.Parallel()

	store := &mockStore{} // nil token

	tp := NewTokenProvider(store, &config.Config{}, io.Discard)
	_, err := tp.UserInfo()

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotAuthenticated)
}

func TestUserInfoDecodesIDToken(t *testing.T) {
	t.Parallel()

	idToken := fakeJWT(`{"email":"test@example.com","name":"Test User","sub":"123","groups":["admin"]}`)

	store := &mockStore{
		tok: &token.StoredToken{
			IDToken:   idToken,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}

	tp := NewTokenProvider(store, &config.Config{}, io.Discard)
	info, err := tp.UserInfo()
	require.NoError(t, err)

	assert.Equal(t, "test@example.com", info.Email)
	assert.Equal(t, "Test User", info.Name)
	assert.Equal(t, []string{"admin"}, info.Groups)
	assert.False(t, info.ExpiresAt.IsZero(), "ExpiresAt should be set from stored token")
	assert.False(t, info.FromEnv, "claims of the stored session must not be marked FromEnv")
}

func TestNewTokenProviderNilWriterPanics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "auth.NewTokenProvider: nil writer", func() {
		NewTokenProvider(&mockStore{}, &config.Config{}, nil)
	})
}

func TestWriteLoginSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		claims UserInfo
		want   string
	}{
		{
			name:   "without groups",
			claims: UserInfo{Email: "user@example.com", Name: "Test User"},
			want:   "Logged in as user@example.com (Test User)\n",
		},
		{
			name:   "with groups",
			claims: UserInfo{Email: "user@example.com", Name: "Test User", Groups: []string{"admin", "developers"}},
			want:   "Logged in as user@example.com (Test User)\nGroups: admin, developers\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			writeLoginSummary(&buf, &tt.claims)

			assert.Equal(t, tt.want, buf.String())
		})
	}
}

// newRefreshIDP serves OIDC discovery and a refresh_token grant for
// oldRefreshToken over TLS.
func newRefreshIDP(t *testing.T) *httptest.Server {
	t.Helper()

	idp, _ := newCountingRefreshIDP(t, oldRefreshToken, 0)

	return idp
}

// newCountingRefreshIDP serves OIDC discovery and a refresh_token grant over
// TLS. It counts token requests, holds each one for delay, and answers 400
// invalid_grant to anything but a refresh_token grant for accept.
func newCountingRefreshIDP(t *testing.T, accept string, delay time.Duration) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var (
		idp   *httptest.Server
		calls atomic.Int32
	)

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
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(delay)

		if r.PostFormValue("grant_type") != "refresh_token" || r.PostFormValue("refresh_token") != accept {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"token_type":    "Bearer",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
			"id_token":      fakeJWT(`{"email":"user@example.com"}`),
		})
	})

	idp = httptest.NewTLSServer(mux)
	t.Cleanup(idp.Close)

	return idp, &calls
}

func TestRefreshPersistenceWarning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		saveErr error
		want    string
	}{
		{
			name:    "save fails",
			saveErr: errors.New("disk full"),
			want: "Warning: failed to persist refreshed credentials: disk full\n" +
				"You may need to re-authenticate on next use.\n",
		},
		{name: "save succeeds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idp := newRefreshIDP(t)

			store := &mockStore{
				tok: &token.StoredToken{
					AccessToken:  "expired",
					RefreshToken: oldRefreshToken,
					ExpiresAt:    time.Now().Add(-time.Minute),
					IssuerURL:    idp.URL,
					ClientID:     "krci-cli",
					PortalURL:    portalA,
				},
				saveErr: tt.saveErr,
			}

			var buf bytes.Buffer

			tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, &buf)

			refreshed, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
			require.NoError(t, err)

			assert.Equal(t, "new-refresh", refreshed.RefreshToken)
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func TestRefreshRecordsPortalURL(t *testing.T) {
	t.Parallel()

	idp := newRefreshIDP(t)

	store := &mockStore{
		tok: &token.StoredToken{
			AccessToken:  "expired",
			RefreshToken: oldRefreshToken,
			ExpiresAt:    time.Now().Add(-time.Minute),
			IssuerURL:    idp.URL,
			ClientID:     "krci-cli",
			PortalURL:    portalA,
		},
	}

	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	refreshed, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)

	assert.Equal(t, "new-refresh", refreshed.RefreshToken)
	assert.Equal(t, portalA, refreshed.PortalURL)
	assert.Equal(t, portalA, store.tok.PortalURL, "the persisted token must carry the portal")
}

func TestValidateIssuerURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://idp.example.com/realms/shared", false},
		{"http rejected", "http://idp.example.com/realms/shared", true},
		{"empty", "", true},
		{"no host", "https://", true},
		{"file scheme", "file:///etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateIssuerURL(tt.url)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

// refreshableStoredToken returns a token bound to portalA with
// oldRefreshToken whose access token expires in expiresIn.
func refreshableStoredToken(issuerURL string, expiresIn time.Duration) *token.StoredToken {
	return &token.StoredToken{
		AccessToken:  "old-access",
		RefreshToken: oldRefreshToken,
		IDToken:      fakeJWT(`{"email":"old@example.com"}`),
		ExpiresAt:    time.Now().Add(expiresIn),
		IssuerURL:    issuerURL,
		ClientID:     "krci-cli",
		PortalURL:    portalA,
	}
}

func TestGetTokenConcurrentCallersShareOneRefresh(t *testing.T) {
	t.Parallel()

	const callers = 8

	idp, calls := newCountingRefreshIDP(t, oldRefreshToken, 50*time.Millisecond)
	store := &mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute)}

	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)
	ctx := oidc.ClientContext(t.Context(), idp.Client())

	var (
		wg     sync.WaitGroup
		start  = make(chan struct{})
		tokens = make([]string, callers)
		errs   = make([]error, callers)
	)

	for i := range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			tokens[i], errs[i] = tp.GetToken(ctx)
		}()
	}

	close(start)
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i], "caller %d", i)
		assert.Equal(t, tokens[0], tokens[i], "caller %d", i)
	}

	assert.Equal(t, int32(1), calls.Load(), "token requests to the IdP")
}

func TestGetTokenLoadsStoreOnceWhileValid(t *testing.T) {
	t.Parallel()

	store := &mockStore{tok: &token.StoredToken{
		IDToken: "id", ExpiresAt: time.Now().Add(time.Hour), PortalURL: portalA,
	}}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	for range 3 {
		tok, err := tp.GetToken(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "id", tok)
	}

	assert.Equal(t, 1, store.loadCount())
}

func TestGetTokenReloadsStoreWhenResolvedTokenExpires(t *testing.T) {
	t.Parallel()

	idp, calls := newCountingRefreshIDP(t, oldRefreshToken, 0)
	fresh := &token.StoredToken{IDToken: "fresh", ExpiresAt: time.Now().Add(time.Hour), PortalURL: portalA}
	store := &mockStore{tok: fresh}

	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)
	tp.current = refreshableStoredToken(idp.URL, 10*time.Second)

	tok, err := tp.GetToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)
	assert.Equal(t, "fresh", tok, "a token another process saved is used")
	assert.Zero(t, calls.Load(), "token requests to the IdP")
}

func TestLoginAndLogoutForgetResolvedToken(t *testing.T) {
	t.Parallel()

	store := &mockStore{tok: &token.StoredToken{
		IDToken: "id", ExpiresAt: time.Now().Add(time.Hour), PortalURL: portalA,
	}}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	_, err := tp.GetToken(t.Context())
	require.NoError(t, err)

	require.NoError(t, tp.Logout())

	_, err = tp.GetToken(t.Context())
	require.ErrorIs(t, err, ErrNotAuthenticated)

	// No issuer is configured, so Login fails.
	tp.current = &token.StoredToken{IDToken: "id", ExpiresAt: time.Now().Add(time.Hour), PortalURL: portalA}
	require.Error(t, tp.Login(t.Context()))
	assert.Nil(t, tp.current)
}

func TestRefreshUsesResolvedRefreshTokenWhenStoreIsStale(t *testing.T) {
	t.Parallel()

	idp, calls := newCountingRefreshIDP(t, "rotated-refresh", 0)

	// The previous refresh rotated the refresh token, and its Save failed.
	resolved := refreshableStoredToken(idp.URL, -time.Minute)
	resolved.RefreshToken = "rotated-refresh"

	store := &mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute)}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)
	tp.current = resolved

	refreshed, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)
	assert.Equal(t, "new-access", refreshed.AccessToken)
	assert.Equal(t, int32(1), calls.Load(), "token requests to the IdP")
}

func TestUserInfoUsesTheRefreshedTokenWhenSaveFails(t *testing.T) {
	t.Parallel()

	idp := newRefreshIDP(t)
	store := &mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute), saveErr: errors.New("disk full")}

	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	_, err := tp.GetToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)

	info, err := tp.UserInfo()
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", info.Email)
	assert.True(t, info.ExpiresAt.After(time.Now().Add(time.Minute)), "ExpiresAt = %s", info.ExpiresAt)
}
