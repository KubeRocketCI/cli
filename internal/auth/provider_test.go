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
	"golang.org/x/oauth2"

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
					IDToken:      fakeJWT(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(-time.Minute).Unix())),
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

	return startRefreshIDP(t, accept, delay, defaultTokenResponse())
}

// defaultTokenResponse is the refresh response of the fake IdP. Its ID token
// has no exp, so the refreshed ExpiresAt is the access-token expiry.
func defaultTokenResponse() map[string]any {
	return map[string]any{
		"access_token":  "new-access",
		"token_type":    "Bearer",
		"refresh_token": "new-refresh",
		"expires_in":    3600,
		"id_token":      fakeJWT(`{"email":"user@example.com"}`),
	}
}

// noIDTokenResponse is defaultTokenResponse without an id_token.
func noIDTokenResponse() map[string]any {
	response := defaultTokenResponse()
	delete(response, "id_token")

	return response
}

// getTokenConcurrently calls tp.GetToken from n goroutines released together.
func getTokenConcurrently(ctx context.Context, tp *tokenProvider, n int) ([]string, []error) {
	var (
		wg     sync.WaitGroup
		start  = make(chan struct{})
		tokens = make([]string, n)
		errs   = make([]error, n)
	)

	for i := range n {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			tokens[i], errs[i] = tp.GetToken(ctx)
		}()
	}

	close(start)
	wg.Wait()

	return tokens, errs
}

// startRefreshIDP is newCountingRefreshIDP that answers a granted refresh with
// response.
func startRefreshIDP(
	t *testing.T, accept string, delay time.Duration, response map[string]any,
) (*httptest.Server, *atomic.Int32) {
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
		_ = json.NewEncoder(w).Encode(response)
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
	tokens, errs := getTokenConcurrently(oidc.ClientContext(t.Context(), idp.Client()), tp, callers)

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

func TestRefreshUsesUnsavedRefreshToken(t *testing.T) {
	t.Parallel()

	idp, calls := newCountingRefreshIDP(t, "rotated-refresh", 0)

	// The previous refresh rotated the refresh token, and its Save failed.
	unsaved := refreshableStoredToken(idp.URL, -time.Minute)
	unsaved.RefreshToken = "rotated-refresh"

	store := &mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute)}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)
	tp.unsaved = unsaved

	refreshed, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)
	assert.Equal(t, "new-access", refreshed.AccessToken)
	assert.Equal(t, int32(1), calls.Load(), "token requests to the IdP")
	assert.Nil(t, tp.unsaved, "a successful Save clears unsaved")
}

func TestRefreshPrefersStoreOverSavedCurrent(t *testing.T) {
	t.Parallel()

	idp, calls := newCountingRefreshIDP(t, oldRefreshToken, 0)

	// The in-memory token's refresh token was saved and has since been rotated by another process.
	resolved := refreshableStoredToken(idp.URL, -time.Minute)
	resolved.RefreshToken = "superseded-refresh"

	store := &mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute)}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)
	tp.current = resolved

	_, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load(), "token requests to the IdP")
}

func TestAdoptingStoredTokenClearsUnsaved(t *testing.T) {
	t.Parallel()

	fresh := &token.StoredToken{IDToken: "fresh", ExpiresAt: time.Now().Add(time.Hour), PortalURL: portalA}
	tp := NewTokenProvider(&mockStore{tok: fresh}, &config.Config{PortalURL: portalA}, io.Discard)
	tp.unsaved = refreshableStoredToken("https://idp.example.com", -time.Minute)

	tok, err := tp.GetToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "fresh", tok)
	assert.Nil(t, tp.unsaved)
}

func TestRefreshFailureIsRemembered(t *testing.T) {
	t.Parallel()

	expiredIDToken, _ := idTokenExpiring(-time.Minute)

	tests := []struct {
		name     string
		accept   string
		response map[string]any
		// wantCalls is one refresh attempt: oauth2 retries a failed token
		// request once with the other client auth style.
		wantCalls int32
	}{
		{name: "no id_token", accept: oldRefreshToken, response: noIDTokenResponse(), wantCalls: 1},
		{name: "invalid_grant", accept: "other-refresh", response: defaultTokenResponse(), wantCalls: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const callers = 8

			idp, calls := startRefreshIDP(t, tt.accept, 0, tt.response)
			stored := refreshableStoredToken(idp.URL, -time.Minute)
			stored.IDToken = expiredIDToken

			tp := NewTokenProvider(&mockStore{tok: stored}, &config.Config{PortalURL: portalA}, io.Discard)
			_, errs := getTokenConcurrently(oidc.ClientContext(t.Context(), idp.Client()), tp, callers)

			for i := range callers {
				require.ErrorIs(t, errs[i], ErrRefreshFailed, "caller %d", i)
				assert.Equal(t, errs[0].Error(), errs[i].Error(), "caller %d", i)
			}

			assert.Equal(t, tt.wantCalls, calls.Load(), "token requests to the IdP")
		})
	}
}

func TestLoginAndLogoutForgetRefreshFailure(t *testing.T) {
	t.Parallel()

	store := &mockStore{}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	tp.refreshErr = ErrRefreshFailed
	require.NoError(t, tp.Logout())

	_, err := tp.GetToken(t.Context())
	require.ErrorIs(t, err, ErrNotAuthenticated)

	// No issuer is configured, so Login fails.
	tp.refreshErr = ErrRefreshFailed
	tp.unsaved = refreshableStoredToken("https://idp.example.com", -time.Minute)
	require.Error(t, tp.Login(t.Context()))
	assert.Nil(t, tp.refreshErr)
	assert.Nil(t, tp.unsaved)
}

func TestUserInfoUsesTheRefreshedTokenWhenSaveFails(t *testing.T) {
	t.Parallel()

	idp := newRefreshIDP(t)
	store := &mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute), saveErr: errors.New("disk full")}

	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	_, err := tp.GetToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)

	assert.Same(t, tp.current, tp.unsaved, "a failed Save keeps the result in unsaved")

	info, err := tp.UserInfo()
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", info.Email)
	assert.True(t, info.ExpiresAt.After(time.Now().Add(time.Minute)), "ExpiresAt = %s", info.ExpiresAt)
}

func TestRefreshCallsIdPInsideExpiryMargin(t *testing.T) {
	t.Parallel()

	idp, calls := newCountingRefreshIDP(t, oldRefreshToken, 0)

	stored := refreshableStoredToken(idp.URL, 20*time.Second)
	require.False(t, usable(stored), "the CLI must treat the token as expiring")
	require.True(t, (&oauth2.Token{AccessToken: stored.AccessToken, Expiry: stored.ExpiresAt}).Valid(),
		"oauth2 must still treat the token as valid")

	store := &mockStore{tok: stored}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	refreshed, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)

	assert.Equal(t, int32(1), calls.Load(), "token requests to the IdP")
	assert.Equal(t, "new-access", refreshed.AccessToken)
	assert.True(t, refreshed.ExpiresAt.After(time.Now().Add(time.Hour-time.Minute)), "ExpiresAt = %s", refreshed.ExpiresAt)
	assert.Equal(t, "new-access", store.tok.AccessToken, "the refreshed token is saved")
}

func TestGetTokenWithoutExpirySkipsRefresh(t *testing.T) {
	t.Parallel()

	idp, calls := newCountingRefreshIDP(t, oldRefreshToken, 0)

	stored := refreshableStoredToken(idp.URL, 0)
	stored.ExpiresAt = time.Time{}

	tp := NewTokenProvider(&mockStore{tok: stored}, &config.Config{PortalURL: portalA}, io.Discard)
	ctx := oidc.ClientContext(t.Context(), idp.Client())

	for range 3 {
		tok, err := tp.GetToken(ctx)
		require.NoError(t, err)
		assert.Equal(t, stored.IDToken, tok)
	}

	assert.Zero(t, calls.Load(), "token requests to the IdP")
}

func TestRefreshWithoutIDToken(t *testing.T) {
	t.Parallel()

	expiredIDToken, expiredAt := idTokenExpiring(-time.Minute)

	tests := []struct {
		name     string
		storedID string
		wantErr  string
	}{
		{
			name: "stored ID token expired", storedID: expiredIDToken,
			wantErr: "refresh response has no id_token and the stored one expired at " +
				expiredAt.UTC().Format(time.RFC3339),
		},
		{
			name:    "no ID token stored",
			wantErr: "refresh response has no id_token; the CLI needs a new ID token from each refresh",
		},
		{name: "stored ID token without exp", storedID: fakeJWT(`{"email":"old@example.com"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idp, calls := startRefreshIDP(t, oldRefreshToken, 0, noIDTokenResponse())
			stored := refreshableStoredToken(idp.URL, -time.Minute)
			stored.IDToken = tt.storedID
			store := &mockStore{tok: stored}

			tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

			refreshed, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
			assert.Equal(t, int32(1), calls.Load(), "token requests to the IdP")
			assert.Equal(t, "new-refresh", store.tok.RefreshToken, "the rotated refresh token is saved")
			assert.Equal(t, tt.storedID, store.tok.IDToken, "the stored ID token is kept")

			if tt.wantErr != "" {
				require.ErrorIs(t, err, ErrRefreshFailed)
				require.EqualError(t, err, "token refresh failed: "+tt.wantErr)
				assert.Nil(t, tp.current)

				return
			}

			require.NoError(t, err)
			assert.True(t, usable(refreshed), "ExpiresAt = %s", refreshed.ExpiresAt)
		})
	}
}

// idTokenExpiring returns a test ID token that expires after in, and its exp
// truncated to the second.
func idTokenExpiring(in time.Duration) (string, time.Time) {
	exp := time.Now().Add(in).Truncate(time.Second)

	return fakeJWT(fmt.Sprintf(`{"email":"user@example.com","exp":%d}`, exp.Unix())), exp
}

func TestBearerExpiry(t *testing.T) {
	t.Parallel()

	access := time.Now().Add(time.Hour).Truncate(time.Second)
	withExp, idExp := idTokenExpiring(10 * time.Minute)

	assert.True(t, bearerExpiry(&token.StoredToken{IDToken: withExp, ExpiresAt: access}).Equal(idExp), "ID token exp")
	assert.Equal(t, access, bearerExpiry(&token.StoredToken{
		IDToken: fakeJWT(`{"email":"user@example.com"}`), ExpiresAt: access,
	}), "no exp")
	assert.Equal(t, access, bearerExpiry(&token.StoredToken{IDToken: "opaque", ExpiresAt: access}), "not a JWT")
}

func TestUsable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tok  *token.StoredToken
		want bool
	}{
		{"nil", nil, false},
		{"no ID token", &token.StoredToken{ExpiresAt: time.Now().Add(time.Hour)}, false},
		{"future", &token.StoredToken{IDToken: "opaque", ExpiresAt: time.Now().Add(time.Hour)}, true},
		{"outside the margin", &token.StoredToken{IDToken: "opaque", ExpiresAt: time.Now().Add(90 * time.Second)}, true},
		{"inside the margin", &token.StoredToken{IDToken: "opaque", ExpiresAt: time.Now().Add(29 * time.Second)}, false},
		{"expired", &token.StoredToken{IDToken: "opaque", ExpiresAt: time.Now().Add(-time.Minute)}, false},
		{"no expiry", &token.StoredToken{IDToken: "opaque"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, usable(tt.tok))
		})
	}
}

func TestRefreshExpiresWithIDToken(t *testing.T) {
	t.Parallel()

	idToken, idExp := idTokenExpiring(10 * time.Minute)
	response := defaultTokenResponse()
	response["id_token"] = idToken

	idp, _ := startRefreshIDP(t, oldRefreshToken, 0, response)
	store := &mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute)}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	refreshed, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)

	assert.True(t, bearerExpiry(refreshed).Equal(idExp), "bearer expiry = %s, want %s", bearerExpiry(refreshed), idExp)
	assert.Equal(t, refreshed.IDToken, store.tok.IDToken, "the refreshed token is saved")
}

func TestGetTokenRefreshesWhenStoredIDTokenExpires(t *testing.T) {
	t.Parallel()

	idp, calls := newCountingRefreshIDP(t, oldRefreshToken, 0)

	// ExpiresAt holds the access-token expiry; the ID token expires first.
	stored := refreshableStoredToken(idp.URL, time.Hour)

	var idExp time.Time
	stored.IDToken, idExp = idTokenExpiring(20 * time.Second)
	require.False(t, usable(&token.StoredToken{IDToken: "opaque", ExpiresAt: idExp}), "the ID token must be inside the margin")

	store := &mockStore{tok: stored}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	tok, err := tp.GetToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)
	assert.NotEqual(t, stored.IDToken, tok)
	assert.Equal(t, int32(1), calls.Load(), "token requests to the IdP")
}

func TestUserInfoShowsIDTokenExpiry(t *testing.T) {
	t.Parallel()

	idToken, idExp := idTokenExpiring(10 * time.Minute)
	stored := &token.StoredToken{IDToken: idToken, ExpiresAt: time.Now().Add(time.Hour), PortalURL: portalA}

	tp := NewTokenProvider(&mockStore{tok: stored}, &config.Config{PortalURL: portalA}, io.Discard)

	info, err := tp.UserInfo()
	require.NoError(t, err)
	assert.True(t, info.ExpiresAt.Equal(idExp), "ExpiresAt = %s, want %s", info.ExpiresAt, idExp)
}

func TestRefreshRejectsExpiredIDToken(t *testing.T) {
	t.Parallel()

	response := defaultTokenResponse()

	var expiredAt time.Time
	response["id_token"], expiredAt = idTokenExpiring(-time.Minute)

	idp, _ := startRefreshIDP(t, oldRefreshToken, 0, response)
	store := &mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute)}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	_, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.ErrorIs(t, err, ErrRefreshFailed)
	require.EqualError(t, err, "token refresh failed: refresh response id_token expired at "+
		expiredAt.UTC().Format(time.RFC3339)+"; check the local clock")
	assert.Equal(t, "new-refresh", store.tok.RefreshToken, "the rotated refresh token is saved")
	assert.Nil(t, tp.current)
}

func TestRefreshAcceptsIDTokenInsideMargin(t *testing.T) {
	t.Parallel()

	response := defaultTokenResponse()

	idToken, idExp := idTokenExpiring(20 * time.Second)
	response["id_token"] = idToken
	require.False(t, usable(&token.StoredToken{IDToken: "opaque", ExpiresAt: idExp}), "the ID token must be inside the margin")

	idp, _ := startRefreshIDP(t, oldRefreshToken, 0, response)
	store := &mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute)}
	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	refreshed, err := tp.resolveStoredToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)
	assert.Equal(t, idToken, refreshed.IDToken)
	assert.Equal(t, idToken, store.tok.IDToken, "the refreshed token is saved")
}

func TestGetTokenEmptyStoredIDToken(t *testing.T) {
	t.Parallel()

	t.Run("refreshable", func(t *testing.T) {
		t.Parallel()

		idp, calls := newCountingRefreshIDP(t, oldRefreshToken, 0)
		stored := refreshableStoredToken(idp.URL, time.Hour)
		stored.IDToken = ""

		tp := NewTokenProvider(&mockStore{tok: stored}, &config.Config{PortalURL: portalA}, io.Discard)

		tok, err := tp.GetToken(oidc.ClientContext(t.Context(), idp.Client()))
		require.NoError(t, err)
		assert.NotEmpty(t, tok)
		assert.Equal(t, int32(1), calls.Load(), "token requests to the IdP")
	})

	t.Run("no refresh token", func(t *testing.T) {
		t.Parallel()

		stored := &token.StoredToken{ExpiresAt: time.Now().Add(time.Hour), PortalURL: portalA}
		tp := NewTokenProvider(&mockStore{tok: stored}, &config.Config{PortalURL: portalA}, io.Discard)

		_, err := tp.GetToken(t.Context())
		require.ErrorIs(t, err, ErrNotAuthenticated)
	})
}

func TestJWTExpiryOutOfRange(t *testing.T) {
	t.Parallel()

	for _, exp := range []string{"1e300", "9223372036854775808"} {
		_, ok := jwtExpiry(fakeJWT(`{"exp":` + exp + `}`))
		assert.False(t, ok, "exp %s", exp)
	}

	for _, exp := range []string{"9223372036854770000", "99999999999999", "253402300800"} {
		_, ok := jwtExpiry(fakeJWT(`{"exp":` + exp + `}`))
		assert.False(t, ok, "exp %s", exp)
	}

	for _, exp := range []int64{1500000000, 253402300799} {
		got, ok := jwtExpiry(fakeJWT(fmt.Sprintf(`{"exp":%d}`, exp)))
		require.True(t, ok, "exp %d", exp)
		assert.Equal(t, exp, got.Unix())
	}
}

func TestGetTokenUsesValidIDTokenAfterAccessTokenExpires(t *testing.T) {
	t.Parallel()

	idp, calls := newCountingRefreshIDP(t, oldRefreshToken, 0)
	stored := refreshableStoredToken(idp.URL, -time.Minute)
	stored.IDToken, _ = idTokenExpiring(10 * time.Minute)

	tp := NewTokenProvider(&mockStore{tok: stored}, &config.Config{PortalURL: portalA}, io.Discard)

	tok, err := tp.GetToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)
	assert.Equal(t, stored.IDToken, tok)
	assert.Zero(t, calls.Load(), "token requests to the IdP")
}

func TestGetTokenExpiredIDTokenWithoutRefreshToken(t *testing.T) {
	t.Parallel()

	stored := &token.StoredToken{ExpiresAt: time.Now().Add(time.Hour), PortalURL: portalA}
	stored.IDToken, _ = idTokenExpiring(-time.Minute)

	tp := NewTokenProvider(&mockStore{tok: stored}, &config.Config{PortalURL: portalA}, io.Discard)

	_, err := tp.GetToken(t.Context())
	require.ErrorIs(t, err, ErrTokenExpired)
}

func TestRefreshFailureWithDoneContextIsNotRemembered(t *testing.T) {
	t.Parallel()

	idp, calls := newCountingRefreshIDP(t, oldRefreshToken, 0)
	tp := NewTokenProvider(&mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute)},
		&config.Config{PortalURL: portalA}, io.Discard)
	live := oidc.ClientContext(t.Context(), idp.Client())

	done, cancel := context.WithCancel(live)
	cancel()

	_, err := tp.GetToken(done)
	require.ErrorIs(t, err, ErrRefreshFailed)
	require.ErrorIs(t, err, context.Canceled)

	_, err = tp.GetToken(live)
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load(), "token requests to the IdP")
}

// reloadingStore serves tok on the first Load and then on every later Load,
// like a store another process writes to after the first read.
type reloadingStore struct {
	mockStore
	then   *token.StoredToken
	loaded bool
}

func (s *reloadingStore) Load() (*token.StoredToken, error) {
	if s.loaded {
		return s.then, nil
	}

	s.loaded = true

	return s.mockStore.Load()
}

func TestRefreshFailureAdoptsTokenSavedByAnotherProcess(t *testing.T) {
	t.Parallel()

	// The IdP rejects the stored refresh token: another process rotated it.
	idp, _ := newCountingRefreshIDP(t, "other-refresh", 0)
	fresh := &token.StoredToken{IDToken: "fresh", ExpiresAt: time.Now().Add(time.Hour), PortalURL: portalA}
	store := &reloadingStore{mockStore: mockStore{tok: refreshableStoredToken(idp.URL, -time.Minute)}, then: fresh}

	tp := NewTokenProvider(store, &config.Config{PortalURL: portalA}, io.Discard)

	tok, err := tp.GetToken(oidc.ClientContext(t.Context(), idp.Client()))
	require.NoError(t, err)
	assert.Equal(t, "fresh", tok)
	assert.NoError(t, tp.refreshErr)
}
