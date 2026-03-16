package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/token"
)

// TokenProvider resolves a valid access token using the precedence chain:
// KRCI_TOKEN env → cached token → refresh → error.
type TokenProvider interface {
	// GetToken returns a valid access token.
	GetToken(ctx context.Context) (string, error)
	// Login performs the interactive OIDC login flow.
	Login(ctx context.Context) error
	// Logout clears stored credentials.
	Logout() error
	// UserInfo returns cached user claims from the stored ID token.
	UserInfo() (*UserInfo, error)
}

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

// GetToken returns a valid access token following the precedence chain.
func (p *tokenProvider) GetToken(ctx context.Context) (string, error) {
	// 1. Environment variable (highest priority, for CI/automation).
	if t := os.Getenv("KRCI_TOKEN"); t != "" {
		return t, nil
	}

	// 2. Load cached token.
	stored, err := p.store.Load()
	if err != nil {
		if errors.Is(err, token.ErrNoToken) {
			return "", ErrNotAuthenticated
		}
		return "", fmt.Errorf("loading cached token: %w", err)
	}

	// 3. Check if still valid (with 30-second buffer).
	if stored.Valid() {
		return stored.AccessToken, nil
	}

	// 4. Attempt refresh.
	if stored.RefreshToken != "" {
		refreshed, err := p.refresh(ctx, stored)
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrRefreshFailed, err)
		}
		return refreshed.AccessToken, nil
	}

	return "", ErrTokenExpired
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

// UserInfo returns user claims by decoding the stored ID token (unverified, display only).
func (p *tokenProvider) UserInfo() (*UserInfo, error) {
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
	if err := validateIssuerURL(stored.IssuerURL); err != nil {
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
	parts := strings.Split(rawIDToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ID token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding ID token payload: %w", err)
	}

	var claims UserInfo
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parsing ID token claims: %w", err)
	}

	return &claims, nil
}
