// Package auth provides OIDC authentication for the krci CLI.
package auth

import "errors"

// Sentinel errors for auth failure classification.
// Callers use errors.Is() to branch on these.
var (
	ErrNotAuthenticated = errors.New("not authenticated: run 'krci auth login'")
	ErrTokenExpired     = errors.New("token expired")
	ErrRefreshFailed    = errors.New("token refresh failed")
)
