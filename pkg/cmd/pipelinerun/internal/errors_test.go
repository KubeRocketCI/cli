package pipelineruninternal

import (
	"errors"
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/portal"
)

func TestHandleAuthError_AddsAuthHintForUnauthorized(t *testing.T) {
	t.Parallel()

	mapped := HandleAuthError(portal.ErrUnauthorized)
	if !errors.Is(mapped, portal.ErrUnauthorized) {
		t.Fatalf("auth wrap must preserve errors.Is chain to ErrUnauthorized; got %v", mapped)
	}

	want := cmdutil.ErrAuthRequired(portal.ErrUnauthorized).Error()
	if mapped.Error() != want {
		t.Errorf("auth-required message mismatch:\n got  %q\n want %q", mapped.Error(), want)
	}
}

func TestHandleAuthError_PassesThroughOtherErrors(t *testing.T) {
	t.Parallel()

	cases := []error{
		portal.ErrPipelineNotFound,
		portal.ErrTriggerTemplateNotFound,
		portal.ErrUpstreamUnavailable,
		portal.ErrPlatformReject,
		portal.ErrPermissionDenied,
		errors.New("boom"),
	}

	for _, in := range cases {
		if got := HandleAuthError(in); got != in {
			t.Errorf("HandleAuthError(%v) should pass through unchanged, got %v", in, got)
		}
	}
}
