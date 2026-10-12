package render

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// SchemaVersion is the version in every success and error envelope. Bump only
// when the envelope shape changes in a way scripting consumers must detect.
const SchemaVersion = "1"

// envelope is the success shape of the envelope verbs.
type envelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Data          any    `json:"data"`
}

// ErrorEnvelope is the error shape of the envelope verbs.
type ErrorEnvelope struct {
	SchemaVersion string    `json:"schemaVersion"`
	Error         ErrorBody `json:"error"`
}

// ErrorBody carries the human-readable error message.
type ErrorBody struct {
	Message string `json:"message"`
}

// PrintEnvelope writes data in the `{schemaVersion, data}` envelope to w.
func PrintEnvelope(w io.Writer, data any) error {
	return output.PrintJSON(w, envelope{SchemaVersion: SchemaVersion, Data: data})
}

// HandleAuthError prefixes a portal.ErrUnauthorized chain with
// "authentication required: ". The "krci auth login" hint comes from the
// ErrUnauthorized text. Other errors pass through unchanged.
func HandleAuthError(err error) error {
	if errors.Is(err, portal.ErrUnauthorized) {
		return fmt.Errorf("authentication required: %w", err)
	}

	return err
}

// PrintError writes the error envelope to stdout under -o json, unless err
// reports context cancellation. It adds no auth-error prefix. It returns err,
// so the root command prints the message on stderr and exits 1, or exits
// silently if the command was interrupted.
func PrintError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	// Matches only while main cancels with context.WithCancel: under
	// signal.NotifyContext, net/http returns the signal cause, which does not
	// wrap context.Canceled.
	if errors.Is(err, context.Canceled) {
		return err
	}

	if output.ResolveFormat(outputFormat) == output.FormatJSON {
		_ = output.PrintJSON(ios.Out, ErrorEnvelope{
			SchemaVersion: SchemaVersion,
			Error:         ErrorBody{Message: err.Error()},
		})
	}

	return err
}

// HandleError is the error path of an envelope verb: HandleAuthError, then
// PrintError.
func HandleError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	return PrintError(ios, outputFormat, HandleAuthError(err))
}
