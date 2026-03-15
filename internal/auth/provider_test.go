package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/token"
)

// fakeJWT builds a test JWT from raw JSON header and payload (alg:none, no signature).
func fakeJWT(header, payload string) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(header))
	p := base64.RawURLEncoding.EncodeToString([]byte(payload))

	return h + "." + p + "."
}

// mockStore implements token.Store for testing.
type mockStore struct {
	tok     *token.StoredToken
	loadErr error
	saveErr error
	cleared bool
}

func (m *mockStore) Save(tok *token.StoredToken) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.tok = tok
	return nil
}

func (m *mockStore) Load() (*token.StoredToken, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.tok == nil {
		return nil, token.ErrNoToken
	}
	return m.tok, nil
}

func (m *mockStore) Clear() error {
	m.tok = nil
	m.cleared = true
	return nil
}

// TestGetTokenEnvVarTakesPrecedence cannot use t.Parallel because t.Setenv
// is incompatible with parallel tests.
func TestGetTokenEnvVarTakesPrecedence(t *testing.T) {
	t.Setenv("KRCI_TOKEN", "env-token-value")

	store := &mockStore{
		tok: &token.StoredToken{AccessToken: "cached-token", ExpiresAt: time.Now().Add(time.Hour)},
	}

	tp := NewTokenProvider(store, &config.Config{})
	tok, err := tp.GetToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "env-token-value", tok)
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
					AccessToken: "cached-access-token",
					ExpiresAt:   time.Now().Add(time.Hour),
				},
			},
			wantTok: "cached-access-token",
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
				},
			},
			wantErr: ErrTokenExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tp := NewTokenProvider(tt.store, &config.Config{})
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

func TestGetTokenLoadError(t *testing.T) {
	t.Parallel()

	store := &mockStore{loadErr: errors.New("disk failure")}

	tp := NewTokenProvider(store, &config.Config{})
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

	tp := NewTokenProvider(store, &config.Config{})
	require.NoError(t, tp.Logout())
	assert.True(t, store.cleared, "Logout() did not call Clear()")
}

func TestUserInfoNotAuthenticated(t *testing.T) {
	t.Parallel()

	store := &mockStore{} // nil token

	tp := NewTokenProvider(store, &config.Config{})
	_, err := tp.UserInfo()

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotAuthenticated)
}

func TestUserInfoDecodesIDToken(t *testing.T) {
	t.Parallel()

	idToken := fakeJWT(
		`{"alg":"none"}`,
		`{"email":"test@example.com","name":"Test User","sub":"123","groups":["admin"]}`,
	)

	store := &mockStore{
		tok: &token.StoredToken{
			IDToken:   idToken,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}

	tp := NewTokenProvider(store, &config.Config{})
	info, err := tp.UserInfo()
	require.NoError(t, err)

	assert.Equal(t, "test@example.com", info.Email)
	assert.Equal(t, "Test User", info.Name)
	assert.Equal(t, []string{"admin"}, info.Groups)
	assert.False(t, info.ExpiresAt.IsZero(), "ExpiresAt should be set from stored token")
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

			err := validateIssuerURL(tt.url)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}
