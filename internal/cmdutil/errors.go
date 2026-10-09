package cmdutil

import (
	"context"
	"errors"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// HandleAuthError promotes portal.ErrUnauthorized to the "run krci auth login"
// hint; other errors pass through unchanged.
func HandleAuthError(err error) error {
	if errors.Is(err, portal.ErrUnauthorized) {
		return ErrAuthRequired(err)
	}

	return err
}

// PrintError writes err to stdout as the JSON error envelope when -o json is
// selected and err does not report context cancellation. It returns err, so the
// root command prints the message on stderr and exits 1, or exits silently if
// the command was interrupted.
func PrintError(ios *iostreams.IOStreams, outputFormat, schemaVersion string, err error) error {
	// Matches only while main cancels with context.WithCancel: under
	// signal.NotifyContext, net/http returns the signal cause, which does not
	// wrap context.Canceled.
	if errors.Is(err, context.Canceled) {
		return err
	}

	if output.ResolveFormat(outputFormat) == output.FormatJSON {
		_ = output.PrintJSONErrorEnvelope(ios.Out, schemaVersion, err)
	}

	return err
}

// HandleError is the error path of a verb that prints the JSON envelope:
// HandleAuthError, then PrintError.
func HandleError(ios *iostreams.IOStreams, outputFormat, schemaVersion string, err error) error {
	return PrintError(ios, outputFormat, schemaVersion, HandleAuthError(err))
}
