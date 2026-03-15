package auth

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/pkg/browser"
	"golang.org/x/oauth2"

	"github.com/KubeRocketCI/cli/internal/token"
)

// UserInfo holds OIDC claims extracted from the ID token.
type UserInfo struct {
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Sub       string    `json:"sub"`
	Groups    []string  `json:"groups"`
	ExpiresAt time.Time `json:"-"` // set from token expiry, not from JWT claims
}

// validateIssuerURL ensures the issuer URL is well-formed and uses HTTPS.
func validateIssuerURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid issuer URL %q: %w", rawURL, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("issuer URL must use HTTPS, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("issuer URL has no host: %q", rawURL)
	}
	return nil
}

// login performs the full OIDC Authorization Code + PKCE flow:
// discovery → PKCE → browser → callback → code exchange → ID token verify → store.
func (p *tokenProvider) login(ctx context.Context) (*UserInfo, error) {
	// 0. Validate issuer URL.
	if err := validateIssuerURL(p.cfg.IssuerURL); err != nil {
		return nil, err
	}

	// 1. OIDC Discovery.
	provider, err := oidc.NewProvider(ctx, p.cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery for %s: %w", p.cfg.IssuerURL, err)
	}

	// 2. Generate PKCE verifier.
	verifier := oauth2.GenerateVerifier()

	// 3. Generate random state (32 bytes, base64url).
	state, err := generateState()
	if err != nil {
		return nil, err
	}

	// 4. Start localhost callback server on ephemeral port.
	// Listen on "localhost" to cover both IPv4 and IPv6 loopback.
	// Keycloak allows any port for "localhost" redirect URIs (RFC 8252 loopback exception).
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback server: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://localhost:%d/callback", port)

	// 5. OAuth2 config — public client (no ClientSecret).
	oauthCfg := &oauth2.Config{
		ClientID:    p.cfg.ClientID,
		Endpoint:    provider.Endpoint(),
		RedirectURL: redirectURL,
		Scopes:      strings.Fields(p.cfg.Scopes),
	}

	// 6. Build authorization URL with PKCE S256.
	// prompt=consent forces fresh authentication — ensures Keycloak returns new
	// tokens with updated group membership, even if an SSO session exists.
	authURL := oauthCfg.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	// 7. Open browser.
	fmt.Fprintf(os.Stderr, "Opening browser for authentication...\n")
	fmt.Fprintf(os.Stderr, "If browser doesn't open, visit:\n  %s\n\n", authURL)

	if err := browser.OpenURL(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
	}

	// 8. Wait for callback (5 minute timeout).
	result, err := waitForCallback(listener, state, 5*time.Minute)
	if err != nil {
		return nil, err
	}

	// 9. Exchange authorization code for tokens.
	tok, err := oauthCfg.Exchange(ctx, result.Code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchanging authorization code: %w", err)
	}

	// 10. Extract and verify ID token.
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in token response")
	}

	idTokenVerifier := provider.Verifier(&oidc.Config{ClientID: p.cfg.ClientID})
	idToken, err := idTokenVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verifying ID token: %w", err)
	}

	// 11. Extract user claims.
	var claims UserInfo
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("extracting claims: %w", err)
	}

	// 12. Store tokens encrypted.
	stored := token.NewStoredToken(tok, p.cfg.IssuerURL, p.cfg.ClientID)
	if err := p.store.Save(stored); err != nil {
		return nil, fmt.Errorf("saving credentials: %w", err)
	}

	return &claims, nil
}
