package portal

import (
	"errors"
	"fmt"
)

// Sentinel errors for portal API failures.
var (
	ErrUnauthorized  = errors.New("unauthorized: please run 'krci auth login'")
	ErrNotFound      = errors.New("resource not found")
	ErrHTTPSRequired = errors.New("portal URL must use HTTPS")
)

// APIError represents a tRPC error response from the portal.
type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("portal API error (%s): %s", e.Code, e.Message)
}

func (e *APIError) Is(target error) bool {
	switch {
	case errors.Is(target, ErrUnauthorized):
		return e.Code == "UNAUTHORIZED"
	case errors.Is(target, ErrNotFound):
		return e.Code == "NOT_FOUND"
	default:
		return false
	}
}
