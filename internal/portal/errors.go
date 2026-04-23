package portal

import (
	"errors"
)

// Sentinel errors for portal API failures.
var (
	ErrUnauthorized        = errors.New("unauthorized: please run 'krci auth login'")
	ErrNotFound            = errors.New("resource not found")
	ErrHTTPSRequired       = errors.New("portal URL must use HTTPS")
	ErrUpstreamUnavailable = errors.New("upstream service unavailable")
)
