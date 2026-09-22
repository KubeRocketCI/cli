// Package auth provides OIDC authentication for the krci CLI.
package auth

import "errors"

// Sentinel errors for auth failure classification.
// Callers use errors.Is() to branch on these.
var (
	ErrNotAuthenticated = errors.New("not authenticated: run 'krci auth login'")
	ErrTokenExpired     = errors.New("token expired")
	ErrRefreshFailed    = errors.New("token refresh failed")

	// ErrEnvTokenExpired is returned when KRCI_TOKEN is a JWT whose exp claim
	// has passed. KRCI_TOKEN is never refreshed; the caller supplies a new one.
	ErrEnvTokenExpired = errors.New("KRCI_TOKEN has expired: supply a fresh token")
)
