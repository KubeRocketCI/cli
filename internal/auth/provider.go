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
	// unsaved is the last refresh result whose Save failed; its refresh token
	// is newer than the store's. nil after a successful Save, when a usable
	// stored token is adopted, and after Login or Logout.
	unsaved *token.StoredToken
	// refreshErr is the first refresh failure; GetToken returns it until Login
	// or Logout.
	refreshErr error
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

	if p.refreshErr != nil {
		return nil, p.refreshErr
	}

	if usable(p.current) {
		return p.current, nil
	}

	p.current = nil

	stored, err := p.loadStored()
	if err != nil {
		return nil, err
	}

	if !p.bound(stored) {
		return nil, ErrNotAuthenticated
	}

	if usable(stored) {
		p.current, p.unsaved = stored, nil

		return stored, nil
	}

	base := stored
	if p.unsaved != nil {
		base = p.unsaved
	}

	if base.RefreshToken == "" {
		if base.IDToken == "" {
			return nil, ErrNotAuthenticated
		}

		return nil, ErrTokenExpired
	}

	refreshed, fromResponse, err := p.refresh(ctx, base)
	if err == nil {
		p.unsaved = nil
		if saveErr := p.store.Save(refreshed); saveErr != nil {
			p.unsaved = refreshed

			_, _ = fmt.Fprintf(p.w, "Warning: failed to persist refreshed credentials: %v\n", saveErr)
			_, _ = fmt.Fprintf(p.w, "You may need to re-authenticate on next use.\n")
		}

		err = checkRefreshed(refreshed, fromResponse)
	}

	if err != nil {
		return p.refreshFailed(ctx, err)
	}

	p.current = refreshed

	return refreshed, nil
}

// refreshFailed handles a failed refresh. A failure while ctx is done is
// returned and not remembered. Otherwise the store is read once more: a usable
// token saved by another process is adopted; if there is none, the failure is
// remembered in refreshErr.
func (p *tokenProvider) refreshFailed(ctx context.Context, cause error) (*token.StoredToken, error) {
	err := fmt.Errorf("%w: %w", ErrRefreshFailed, cause)
	if ctx.Err() != nil {
		return nil, err
	}

	if reloaded, loadErr := p.loadStored(); loadErr == nil && p.bound(reloaded) && usable(reloaded) {
		p.current, p.unsaved = reloaded, nil

		return reloaded, nil
	}

	p.refreshErr = err

	return nil, err
}

// bound reports whether t is bound to the configured portal URL.
func (p *tokenProvider) bound(t *token.StoredToken) bool {
	return t.PortalURL != "" && t.PortalURL == p.cfg.PortalURL
}

// expiryMargin is how long before its expiry a bearer counts as expired.
const expiryMargin = 30 * time.Second

// usable reports whether t can be sent as the bearer: it has an ID token whose
// bearerExpiry is zero (no expiry) or more than expiryMargin away.
func usable(t *token.StoredToken) bool {
	if t == nil || t.IDToken == "" {
		return false
	}

	exp := bearerExpiry(t)

	return exp.IsZero() || time.Until(exp) > expiryMargin
}

// bearerExpiry returns the expiry of the bearer GetToken sends: the ID token's
// exp claim, or the access-token expiry when the ID token has none.
func bearerExpiry(t *token.StoredToken) time.Time {
	if exp, ok := jwtExpiry(t.IDToken); ok {
		return exp
	}

	return t.ExpiresAt
}

// forgetCurrent drops the resolved token, the unsaved refresh result and the
// refresh failure, so the next resolution loads the store.
func (p *tokenProvider) forgetCurrent() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current, p.unsaved, p.refreshErr = nil, nil, nil
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
	info.ExpiresAt = bearerExpiry(stored)
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

// refresh runs the OIDC refresh_token grant against the IdP for base and
// returns the new token, bound to the configured portal. The token source
// holds only the refresh token, which oauth2 never treats as valid. A response
// without an ID token keeps base's; fromResponse reports whether the response
// carried one.
func (p *tokenProvider) refresh(
	ctx context.Context, base *token.StoredToken,
) (refreshed *token.StoredToken, fromResponse bool, err error) {
	if err := ValidateIssuerURL(base.IssuerURL); err != nil {
		return nil, false, fmt.Errorf("stored issuer URL: %w", err)
	}

	provider, err := oidc.NewProvider(ctx, base.IssuerURL)
	if err != nil {
		return nil, false, fmt.Errorf("OIDC discovery: %w", err)
	}

	oauthCfg := &oauth2.Config{
		ClientID: base.ClientID,
		Endpoint: provider.Endpoint(),
	}

	newTok, err := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: base.RefreshToken}).Token()
	if err != nil {
		return nil, false, err
	}

	refreshed = token.NewStoredToken(newTok, base.IssuerURL, base.ClientID, p.cfg.PortalURL)
	fromResponse = refreshed.IDToken != ""

	if !fromResponse {
		refreshed.IDToken = base.IDToken
	}

	return refreshed, fromResponse, nil
}

// checkRefreshed rejects a refresh result without an ID token or with an
// expired one. An ID token inside expiryMargin is accepted.
func checkRefreshed(refreshed *token.StoredToken, fromResponse bool) error {
	if refreshed.IDToken == "" {
		return errors.New("refresh response has no id_token; the CLI needs a new ID token from each refresh")
	}

	exp := bearerExpiry(refreshed)
	if exp.IsZero() || time.Now().Before(exp) {
		return nil
	}

	expiredAt := exp.UTC().Format(time.RFC3339)
	if fromResponse {
		return fmt.Errorf("refresh response id_token expired at %s; check the local clock", expiredAt)
	}

	return fmt.Errorf("refresh response has no id_token and the stored one expired at %s", expiredAt)
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

// maxJWTExpiry is 9999-12-31T23:59:59Z, the last time RFC 3339 can print.
const maxJWTExpiry = 253402300799

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

	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp <= 0 || claims.Exp > maxJWTExpiry {
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
