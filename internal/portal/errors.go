package portal

import (
	"errors"
	"fmt"
)

// Sentinel errors for portal API failures.
var (
	ErrUnauthorized        = errors.New("unauthorized: please run 'krci auth login'")
	ErrNotFound            = errors.New("resource not found")
	ErrHTTPSRequired       = errors.New("portal URL must use HTTPS")
	ErrUpstreamUnavailable = errors.New("upstream service unavailable")

	// ErrDeploymentNotFound is returned when a CDPipeline (deployment)
	// look-up fails. Wraps ErrNotFound so callers using errors.Is for
	// generic not-found handling still match.
	ErrDeploymentNotFound = fmt.Errorf("deployment %w", ErrNotFound)

	// ErrEnvNotFound is returned when a Stage (env) lookup within a known
	// deployment fails. Wraps ErrNotFound similarly.
	ErrEnvNotFound = fmt.Errorf("environment %w", ErrNotFound)
)
