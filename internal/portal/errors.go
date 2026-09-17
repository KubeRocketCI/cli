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

	// ErrPermissionDenied is returned for HTTP 403 from the portal. The
	// message must not leak resource metadata.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrPortalUnsupported is returned when the portal has no route for the
	// command at all (Fastify route-not-found): it predates the feature.
	ErrPortalUnsupported = errors.New("portal does not support this command")
)

// richError carries a user-facing message while still matching
// errors.Is(err, sentinel) via Unwrap.
type richError struct {
	msg      string
	sentinel error
}

func (e *richError) Error() string { return e.msg }
func (e *richError) Unwrap() error { return e.sentinel }

// newRichErr replaces the sentinel text with msg; errors.Is(err, sentinel)
// still matches. Sentinels for a missing resource must wrap ErrNotFound
// (e.g. ErrPipelineNotFound).
func newRichErr(msg string, sentinel error) error {
	return &richError{msg: msg, sentinel: sentinel}
}
