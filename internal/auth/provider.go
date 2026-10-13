package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
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
	// when set, otherwise the resolved ID token (the stored one before any
	// resolution).
	UserInfo() (*UserInfo, error)
}

// envTokenVar names the environment variable that overrides the stored token.
const envTokenVar = "KRCI_TOKEN"

type tokenProvider struct {
	store token.Store
	cfg   *config.Config
	w     io.Writer

	// mu serialises resolution of the stored token: concurrent requests share
	// one load and one refresh.
	mu sync.Mutex
	// current is the token the last resolution returned; nil before the first
	// resolution, after a failed one, and after Login or Logout.
	current *token.StoredToken
}

// NewTokenProvider creates a TokenProvider with the given store and config.
// Login progress, the login summary, and refresh-persistence warnings are
// written to w, which must be non-nil.
// Panics if w is nil.
func NewTokenProvider(store token.Store, cfg *config.Config, w io.Writer) *tokenProvider {
	if w == nil {
		panic("auth.NewTokenProvider: nil writer")
	}

	return &tokenProvider{
		store: store,
		cfg:   cfg,
		w:     w,
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

// resolveStoredToken returns the resolved token while it is valid; otherwise
// it loads the stored token and refreshes it when expired. A stored token is
// usable only when bound to the configured portal URL. A token bound to
// another portal, or to none, is treated as absent: it is neither refreshed
// nor returned.
func (p *tokenProvider) resolveStoredToken(ctx context.Context) (*token.StoredToken, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.current != nil && p.current.Valid() {
		return p.current, nil
	}

	previous := p.current
	p.current = nil

	stored, err := p.loadStored()
	if err != nil {
		return nil, err
	}

	if stored.PortalURL == "" || stored.PortalURL != p.cfg.PortalURL {
		return nil, ErrNotAuthenticated
	}

	if stored.Valid() {
		p.current = stored

		return stored, nil
	}

	// After a failed Save the store holds an older refresh token than the
	// previous resolution; an IdP that rotates refresh tokens rejects it.
	if previous != nil && previous.RefreshToken != "" {
		stored = previous
	}

	if stored.RefreshToken != "" {
		refreshed, err := p.refresh(ctx, stored)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrRefreshFailed, err)
		}

		p.current = refreshed

		return refreshed, nil
	}

	return nil, ErrTokenExpired
}

// forgetCurrent drops the resolved token, so the next resolution loads the
// store.
func (p *tokenProvider) forgetCurrent() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = nil
}

// Login performs the interactive OIDC login flow.
func (p *tokenProvider) Login(ctx context.Context) error {
	defer p.forgetCurrent()

	claims, err := p.login(ctx)
	if err != nil {
		return err
	}

	writeLoginSummary(p.w, claims)

	return nil
}

func writeLoginSummary(w io.Writer, claims *UserInfo) {
	_, _ = fmt.Fprintf(w, "Logged in as %s (%s)\n", claims.Email, claims.Name)
	if len(claims.Groups) > 0 {
		_, _ = fmt.Fprintf(w, "Groups: %s\n", strings.Join(claims.Groups, ", "))
	}
}

// Logout clears stored credentials.
func (p *tokenProvider) Logout() error {
	defer p.forgetCurrent()

	return p.store.Clear()
}

// UserInfo returns user claims by decoding the token GetToken resolves
// (unverified, display only): the resolved token when there is one, the
// stored token otherwise. Non-authorising: it does not apply the portal
// binding check; callers resolve the token with GetToken first. For
// KRCI_TOKEN, FromEnv is true and ExpiresAt comes from its exp claim, zero
// when the token has none.
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

	stored, err := p.resolvedOrStored()
	if err != nil {
		return nil, err
	}
	info, err := decodeIDTokenClaims(stored.IDToken)
	if err != nil {
		return nil, err
	}
	info.ExpiresAt = stored.ExpiresAt
	return info, nil
}

// resolvedOrStored returns the resolved token, or loads the store when there
// is none. It checks neither expiry nor the portal binding.
func (p *tokenProvider) resolvedOrStored() (*token.StoredToken, error) {
	p.mu.Lock()
	current := p.current
	p.mu.Unlock()

	if current != nil {
		return current, nil
	}

	return p.loadStored()
}

// loadStored loads the stored token. A missing token is ErrNotAuthenticated.
func (p *tokenProvider) loadStored() (*token.StoredToken, error) {
	stored, err := p.store.Load()
	if err != nil {
		if errors.Is(err, token.ErrNoToken) {
			return nil, ErrNotAuthenticated
		}

		return nil, fmt.Errorf("loading cached token: %w", err)
	}

	return stored, nil
}

// refresh runs the OIDC refresh_token grant against the IdP. The token source
// holds only the refresh token, which oauth2 never treats as valid.
// The caller has already rejected a token not bound to the configured
// portal, so the refreshed token is bound to the configured one.
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

	src := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: stored.RefreshToken})
	newTok, err := src.Token()
	if err != nil {
		return nil, err
	}

	refreshed := token.NewStoredToken(newTok, stored.IssuerURL, stored.ClientID, p.cfg.PortalURL)
	if refreshed.IDToken == "" {
		refreshed.IDToken = stored.IDToken
	}

	if err := p.store.Save(refreshed); err != nil {
		_, _ = fmt.Fprintf(p.w, "Warning: failed to persist refreshed credentials: %v\n", err)
		_, _ = fmt.Fprintf(p.w, "You may need to re-authenticate on next use.\n")
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
