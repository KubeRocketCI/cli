package sonarinternal

import (
	"errors"
	"fmt"
	"io"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// SchemaVersion is the shared JSON envelope version for every `krci sonar *` verb.
// Bump only when the envelope shape changes in a way scripting consumers must
// detect (e.g. removal of a previously-required key).
const SchemaVersion = "1"

// TableRenderer emits the table/text view of a sonar verb's payload. The
// callback receives the output writer and the TTY state so it can apply
// styling or truncation conditionally.
type TableRenderer func(w io.Writer, isTTY bool) error

// Render dispatches on the `-o` flag: emit the shared `{schemaVersion, data}`
// JSON envelope for `-o json`, invoke the supplied TableRenderer for
// `-o table` (the default), or return a validation error for anything else.
func Render[T any](ios *iostreams.IOStreams, outputFormat string, data T, renderTable TableRenderer) error {
	format := output.ResolveFormat(outputFormat)

	switch format {
	case output.FormatJSON:
		return output.PrintJSONEnvelope(ios.Out, SchemaVersion, data)
	case output.FormatTable:
		return renderTable(ios.Out, ios.IsStdoutTTY())
	default:
		return fmt.Errorf("unknown output format: %s (use 'json' or 'table')", format)
	}
}

// HandleError promotes portal.ErrUnauthorized to the shared "run krci auth login"
// message, and — when `-o json` is selected — also writes the
// `{schemaVersion, error: { message }}` envelope to stdout so scripting
// consumers get a structured error alongside the exit-1 signal. Callers still
// return the returned error so root-level main.go emits the plain-text form on
// stderr and exits 1.
func HandleError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	if errors.Is(err, portal.ErrUnauthorized) {
		err = cmdutil.ErrAuthRequired(err)
	}

	if output.ResolveFormat(outputFormat) == output.FormatJSON {
		_ = output.PrintJSONErrorEnvelope(ios.Out, SchemaVersion, err)
	}

	return err
}

// PrintTable is the shared TTY/non-TTY dispatcher for sonar verbs that show a
// plain 2D table. Styled output for interactive terminals, plain tabwriter
// output when stdout is piped.
func PrintTable(w io.Writer, isTTY bool, headers []string, rows [][]string) error {
	if isTTY {
		return output.PrintStyledTable(w, headers, rows)
	}

	return output.PrintTable(w, headers, rows)
}

// PageCount returns the number of pages of pageSize required to hold total
// items. Always returns at least 1 so footers never read "page 1 of 0".
func PageCount(total, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}

	n := total / pageSize
	if total%pageSize != 0 {
		n++
	}

	if n < 1 {
		return 1
	}

	return n
}
