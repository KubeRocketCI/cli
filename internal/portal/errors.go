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

	// ErrPipelineNotFound is returned by `pipelinerun start` when the named
	// Tekton Pipeline does not exist. Wraps ErrNotFound for generic-not-found
	// handling.
	ErrPipelineNotFound = fmt.Errorf("pipeline %w", ErrNotFound)

	// ErrTriggerTemplateNotFound is returned by `pipelinerun start` when the
	// Pipeline carries a TriggerTemplate label but the named TriggerTemplate
	// does not exist.
	ErrTriggerTemplateNotFound = fmt.Errorf("trigger template %w", ErrNotFound)

	// ErrPlatformReject is returned when the platform rejects the start
	// request (e.g. missing required Pipeline param).
	ErrPlatformReject = errors.New("platform rejected request")

	// ErrPermissionDenied is returned for HTTP 403 from the portal start
	// endpoint. The message must not leak resource metadata.
	ErrPermissionDenied = errors.New("permission denied")
)

// richNotFoundError carries a user-facing message while still matching
// errors.Is(err, sentinel) via Unwrap. Use it when the platform supplies a
// disambiguating message that should be shown verbatim instead of the bare
// sentinel text.
type richNotFoundError struct {
	msg      string
	sentinel error
}

func (e *richNotFoundError) Error() string { return e.msg }
func (e *richNotFoundError) Unwrap() error { return e.sentinel }

// sentinel must be ErrNotFound or a sentinel that wraps it (e.g.
// ErrPipelineNotFound) so generic not-found callers continue to match via
// errors.Is.
func newNotFoundErr(msg string, sentinel error) error {
	return &richNotFoundError{msg: msg, sentinel: sentinel}
}
