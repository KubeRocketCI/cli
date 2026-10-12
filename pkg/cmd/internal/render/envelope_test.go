package render_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
)

func TestHandleAuthError(t *testing.T) {
	t.Parallel()

	got := render.HandleAuthError(fmt.Errorf("list: %w", portal.ErrUnauthorized))

	if !errors.Is(got, portal.ErrUnauthorized) {
		t.Errorf("HandleAuthError(...) = %v, want it to wrap portal.ErrUnauthorized", got)
	}

	if want := "authentication required: list: unauthorized: please run 'krci auth login'"; got.Error() != want {
		t.Errorf("HandleAuthError(...) = %q, want %q", got.Error(), want)
	}

	if n := strings.Count(got.Error(), "krci auth login"); n != 1 {
		t.Errorf("login hint appears %d times, want 1", n)
	}

	for _, err := range []error{portal.ErrNotFound, portal.ErrUpstreamUnavailable, errors.New("boom")} {
		if got := render.HandleAuthError(err); got != err {
			t.Errorf("HandleAuthError(%v) = %v, want it unchanged", err, got)
		}
	}
}

func TestPrintError(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("x: %w", errors.New("y"))

	tests := []struct {
		name         string
		outputFormat string
		wantStdout   string
	}{
		{
			name:         "json prints the envelope with the flattened chain",
			outputFormat: "json",
			wantStdout:   "{\n  \"schemaVersion\": \"1\",\n  \"error\": {\n    \"message\": \"x: y\"\n  }\n}\n",
		},
		{name: "table leaves stdout empty", outputFormat: "table"},
		{name: "default format leaves stdout empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ios, stdout := newBufferedIO()

			if got := render.PrintError(ios, tt.outputFormat, wrapped); got != wrapped {
				t.Errorf("PrintError must return the original error; got %v", got)
			}

			if stdout.String() != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}

func TestPrintError_CancelledWritesNoEnvelope(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()
	cancelled := fmt.Errorf("calling sonar list: %w", context.Canceled)

	if got := render.PrintError(ios, "json", cancelled); got != cancelled {
		t.Errorf("PrintError must return the original error; got %v", got)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

// TestPrintError_NoAuthPrefix pins the difference from HandleError: an
// unauthorized error keeps its own text, as auth status prints it.
func TestPrintError_NoAuthPrefix(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()

	if got := render.PrintError(ios, "json", portal.ErrUnauthorized); got != portal.ErrUnauthorized {
		t.Errorf("PrintError must return the original error; got %v", got)
	}

	want := render.ErrorEnvelope{
		SchemaVersion: render.SchemaVersion,
		Error:         render.ErrorBody{Message: portal.ErrUnauthorized.Error()},
	}
	if got := cmdtest.DecodeErrorEnvelope(t, stdout); got != want {
		t.Errorf("envelope = %+v, want %+v", got, want)
	}
}

func TestPrintEnvelope(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := render.PrintEnvelope(&buf, map[string]int{"bugs": 3}); err != nil {
		t.Fatalf("PrintEnvelope: %v", err)
	}

	if want := "{\n  \"schemaVersion\": \"1\",\n  \"data\": {\n    \"bugs\": 3\n  }\n}\n"; buf.String() != want {
		t.Errorf("PrintEnvelope = %q, want %q", buf.String(), want)
	}
}

func TestHandleError_EnvelopeCarriesTheLoginHint(t *testing.T) {
	t.Parallel()

	ios, stdout := newBufferedIO()

	if got := render.HandleError(ios, "json", portal.ErrUnauthorized); !errors.Is(got, portal.ErrUnauthorized) {
		t.Errorf("HandleError(...) = %v, want it to wrap portal.ErrUnauthorized", got)
	}

	want := render.ErrorEnvelope{
		SchemaVersion: render.SchemaVersion,
		Error:         render.ErrorBody{Message: "authentication required: unauthorized: please run 'krci auth login'"},
	}
	if got := cmdtest.DecodeErrorEnvelope(t, stdout); got != want {
		t.Errorf("envelope = %+v, want %+v", got, want)
	}
}
