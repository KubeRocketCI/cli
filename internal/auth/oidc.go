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

// ValidateIssuerURL ensures the issuer URL is well-formed and uses HTTPS.
func ValidateIssuerURL(rawURL string) error {
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
	if err := ValidateIssuerURL(p.cfg.IssuerURL); err != nil {
		return nil, err
	}

	provider, err := oidc.NewProvider(ctx, p.cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery for %s: %w", p.cfg.IssuerURL, err)
	}

	verifier := oauth2.GenerateVerifier()

	state, err := generateState()
	if err != nil {
		return nil, err
	}

	// Listen on "localhost" to cover both IPv4 and IPv6 loopback.
	// OIDC providers support any port for localhost redirect URIs (RFC 8252 §7.3).
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback server: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://localhost:%d/callback", port)

	oauthCfg := &oauth2.Config{
		ClientID:    p.cfg.ClientID,
		Endpoint:    provider.Endpoint(),
		RedirectURL: redirectURL,
		Scopes:      strings.Fields(p.cfg.Scopes),
	}

	// prompt=consent forces fresh authentication — ensures the provider returns new
	// tokens with updated group membership, even if an SSO session exists.
	authURL := oauthCfg.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	fmt.Fprintf(os.Stderr, "Opening browser for authentication...\n")
	fmt.Fprintf(os.Stderr, "If browser doesn't open, visit:\n  %s\n\n", authURL)

	if err := browser.OpenURL(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
	}

	result, err := waitForCallback(listener, state, 5*time.Minute)
	if err != nil {
		return nil, err
	}

	tok, err := oauthCfg.Exchange(ctx, result.Code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchanging authorization code: %w", err)
	}

	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in token response")
	}

	idTokenVerifier := provider.Verifier(&oidc.Config{ClientID: p.cfg.ClientID})
	idToken, err := idTokenVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verifying ID token: %w", err)
	}

	var claims UserInfo
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("extracting claims: %w", err)
	}

	stored := token.NewStoredToken(tok, p.cfg.IssuerURL, p.cfg.ClientID)
	if err := p.store.Save(stored); err != nil {
		return nil, fmt.Errorf("saving credentials: %w", err)
	}

	return &claims, nil
}
