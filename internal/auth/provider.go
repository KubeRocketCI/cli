package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/token"
)

// TokenProvider resolves a valid ID token using the precedence chain:
// KRCI_TOKEN env → cached token → refresh → error.
type TokenProvider interface {
	// GetToken returns a valid ID token for portal Bearer auth.
	GetToken(ctx context.Context) (string, error)
	// Login performs the interactive OIDC login flow.
	Login(ctx context.Context) error
	// Logout clears stored credentials.
	Logout() error
	// UserInfo returns the claims of the token GetToken resolves: KRCI_TOKEN
	// when set, the stored ID token otherwise.
	UserInfo() (*UserInfo, error)
}

// envTokenVar names the environment variable that overrides the stored token.
const envTokenVar = "KRCI_TOKEN"

type tokenProvider struct {
	store token.Store
	cfg   *config.Config
}

// NewTokenProvider creates a TokenProvider with the given store and config.
func NewTokenProvider(store token.Store, cfg *config.Config) *tokenProvider {
	return &tokenProvider{
		store: store,
		cfg:   cfg,
	}
}

// GetToken returns a valid ID token for portal Bearer auth.
// Precedence: KRCI_TOKEN env → cached ID token → refresh → error.
// KRCI_TOKEN is rejected only when it is a JWT with an exp claim in the past;
// an opaque token or one without exp is passed through for the portal to judge.
func (p *tokenProvider) GetToken(ctx context.Context) (string, error) {
	if t := os.Getenv(envTokenVar); t != "" {
		if exp, ok := jwtExpiry(t); ok && !time.Now().Before(exp) {
			return "", ErrEnvTokenExpired
		}

		return t, nil
	}

	stored, err := p.resolveStoredToken(ctx)
	if err != nil {
		return "", err
	}

	return stored.IDToken, nil
}

// resolveStoredToken loads, validates, and optionally refreshes the cached token.
func (p *tokenProvider) resolveStoredToken(ctx context.Context) (*token.StoredToken, error) {
	stored, err := p.store.Load()
	if err != nil {
		if errors.Is(err, token.ErrNoToken) {
			return nil, ErrNotAuthenticated
		}
		return nil, fmt.Errorf("loading cached token: %w", err)
	}

	if stored.Valid() {
		return stored, nil
	}

	if stored.RefreshToken != "" {
		refreshed, err := p.refresh(ctx, stored)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrRefreshFailed, err)
		}
		return refreshed, nil
	}

	return nil, ErrTokenExpired
}

// Login performs the interactive OIDC login flow.
func (p *tokenProvider) Login(ctx context.Context) error {
	claims, err := p.login(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Logged in as %s (%s)\n", claims.Email, claims.Name)
	if len(claims.Groups) > 0 {
		fmt.Fprintf(os.Stderr, "Groups: %s\n", strings.Join(claims.Groups, ", "))
	}

	return nil
}

// Logout clears stored credentials.
func (p *tokenProvider) Logout() error {
	return p.store.Clear()
}

// UserInfo returns user claims by decoding the token GetToken resolves
// (unverified, display only). For KRCI_TOKEN, FromEnv is true and ExpiresAt
// comes from its exp claim, zero when the token has none.
func (p *tokenProvider) UserInfo() (*UserInfo, error) {
	if t := os.Getenv(envTokenVar); t != "" {
		info, err := decodeIDTokenClaims(t)
		if err != nil {
			return nil, err
		}

		if exp, ok := jwtExpiry(t); ok {
			info.ExpiresAt = exp
		}

		info.FromEnv = true

		return info, nil
	}

	stored, err := p.store.Load()
	if err != nil {
		if errors.Is(err, token.ErrNoToken) {
			return nil, ErrNotAuthenticated
		}
		return nil, fmt.Errorf("loading stored token: %w", err)
	}
	info, err := decodeIDTokenClaims(stored.IDToken)
	if err != nil {
		return nil, err
	}
	info.ExpiresAt = stored.ExpiresAt
	return info, nil
}

// refresh uses the OIDC refresh_token grant to obtain new tokens.
// oauth2.TokenSource handles the grant automatically.
func (p *tokenProvider) refresh(ctx context.Context, stored *token.StoredToken) (*token.StoredToken, error) {
	if err := ValidateIssuerURL(stored.IssuerURL); err != nil {
		return nil, fmt.Errorf("stored issuer URL: %w", err)
	}

	provider, err := oidc.NewProvider(ctx, stored.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery: %w", err)
	}

	oauthCfg := &oauth2.Config{
		ClientID: stored.ClientID,
		Endpoint: provider.Endpoint(),
	}

	src := oauthCfg.TokenSource(ctx, stored.ToOAuth2Token())
	newTok, err := src.Token()
	if err != nil {
		return nil, err
	}

	refreshed := token.NewStoredToken(newTok, stored.IssuerURL, stored.ClientID)
	if refreshed.IDToken == "" {
		refreshed.IDToken = stored.IDToken
	}

	if err := p.store.Save(refreshed); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to persist refreshed credentials: %v\n", err)
		fmt.Fprintf(os.Stderr, "You may need to re-authenticate on next use.\n")
	}

	return refreshed, nil
}

// decodeIDTokenClaims extracts claims from a JWT without verification (display only).
func decodeIDTokenClaims(rawIDToken string) (*UserInfo, error) {
	payload, err := jwtPayload(rawIDToken)
	if err != nil {
		return nil, err
	}

	var claims UserInfo
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parsing ID token claims: %w", err)
	}

	return &claims, nil
}

// jwtExpiry returns the exp claim of a JWT, unverified. ok is false for a
// token that is not a JWT or carries no exp.
func jwtExpiry(rawToken string) (time.Time, bool) {
	payload, err := jwtPayload(rawToken)
	if err != nil {
		return time.Time{}, false
	}

	var claims struct {
		Exp float64 `json:"exp"`
	}

	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp <= 0 {
		return time.Time{}, false
	}

	return time.Unix(int64(claims.Exp), 0), true
}

// jwtPayload returns the decoded payload segment of a JWT.
func jwtPayload(rawToken string) ([]byte, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ID token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding ID token payload: %w", err)
	}

	return payload, nil
}
