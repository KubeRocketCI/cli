// Package render owns the output contract shared by the verb groups: `-o`
// dispatch and validation, the table callback, the `{schemaVersion, data}`
// and `{schemaVersion, error}` envelopes, and the auth-error prefix.
// Importable only from packages below pkg/cmd/.
package render

import (
	"io"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
)

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
	format := ResolveFormat(outputFormat)

	switch format {
	case FormatJSON:
		return writeJSON(ios.Out)
	case FormatTable:
		return renderTable(ios.Out, ios.IsStdoutTTY())
	default:
		return unknownFormatError(format, FormatJSON, FormatTable)
	}
}
