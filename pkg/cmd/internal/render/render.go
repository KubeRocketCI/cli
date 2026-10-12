// Package render holds the output dispatch shared by the verb groups: the
// `{schemaVersion, data}` JSON envelope, the table callback, error-envelope
// plumbing, and output-format validation.
// Importable only from packages below pkg/cmd/.
package render

import (
	"io"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
)

// SchemaVersion is the version in every success and error envelope. Bump only
// when the envelope shape changes in a way scripting consumers must detect.
const SchemaVersion = "1"

// TableRenderer emits the table view. It receives the output writer and the
// TTY state of stdout.
type TableRenderer func(w io.Writer, isTTY bool) error

// Render dispatches on the `-o` flag. "json" emits the `{schemaVersion, data}`
// envelope, "table" and the empty default call renderTable, any other value is
// an unknown-format error.
func Render(ios *iostreams.IOStreams, outputFormat string, data any, renderTable TableRenderer) error {
	return dispatch(ios, outputFormat, renderTable, func(w io.Writer) error {
		return PrintEnvelope(w, data)
	})
}

// RenderBare is Render with "json" emitting data as is, without the envelope.
// Use it only for the bare verbs listed in docs/json-schemas.md; new verbs use
// Render.
func RenderBare(ios *iostreams.IOStreams, outputFormat string, data any, renderTable TableRenderer) error {
	return dispatch(ios, outputFormat, renderTable, func(w io.Writer) error {
		return output.PrintJSON(w, data)
	})
}

// dispatch resolves the `-o` value and calls writeJSON or renderTable with
// stdout.
func dispatch(
	ios *iostreams.IOStreams, outputFormat string, renderTable TableRenderer, writeJSON func(io.Writer) error,
) error {
	format := output.ResolveFormat(outputFormat)

	switch format {
	case output.FormatJSON:
		return writeJSON(ios.Out)
	case output.FormatTable:
		return renderTable(ios.Out, ios.IsStdoutTTY())
	default:
		return unknownFormatError(format, output.FormatJSON, output.FormatTable)
	}
}

// PrintEnvelope writes data in the `{schemaVersion, data}` envelope to w.
func PrintEnvelope(w io.Writer, data any) error {
	return output.PrintJSONEnvelope(w, SchemaVersion, data)
}

// PrintError writes the error envelope to stdout under -o json, unless err
// reports context cancellation. It adds no auth-error prefix. It returns err.
func PrintError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	return cmdutil.PrintError(ios, outputFormat, SchemaVersion, err)
}

// HandleError is cmdutil.HandleError with SchemaVersion: the auth-error prefix,
// then PrintError.
func HandleError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	return cmdutil.HandleError(ios, outputFormat, SchemaVersion, err)
}
