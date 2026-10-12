// Package render holds the output dispatch shared by the verb groups: the
// `{schemaVersion, data}` JSON envelope, table output, error-envelope
// plumbing, output-format validation, and paging arithmetic.
// Importable only from packages below pkg/cmd/.
package render

import (
	"io"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
)

// SchemaVersion is the JSON envelope version for every verb that renders
// through this package. Bump only when the envelope shape changes in a way
// scripting consumers must detect.
const SchemaVersion = "1"

// TableRenderer emits the table view. It receives the output writer and the
// TTY state of stdout.
type TableRenderer func(w io.Writer, isTTY bool) error

// Render dispatches on the `-o` flag. "json" emits the `{schemaVersion, data}`
// envelope, "table" and the empty default call renderTable, any other value is
// an unknown-format error.
func Render[T any](ios *iostreams.IOStreams, outputFormat string, data T, renderTable TableRenderer) error {
	format := output.ResolveFormat(outputFormat)

	switch format {
	case output.FormatJSON:
		return output.PrintJSONEnvelope(ios.Out, SchemaVersion, data)
	case output.FormatTable:
		return renderTable(ios.Out, ios.IsStdoutTTY())
	default:
		return unknownFormatError(format, output.FormatJSON, output.FormatTable)
	}
}

// HandleError is cmdutil.HandleError with SchemaVersion.
func HandleError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	return cmdutil.HandleError(ios, outputFormat, SchemaVersion, err)
}

// PrintTable is output.PrintTable.
func PrintTable(w io.Writer, isTTY bool, headers []string, rows [][]string) error {
	return output.PrintTable(w, isTTY, headers, rows)
}

// PageCount returns the number of pages of pageSize needed to hold total
// items. The result is at least 1, also for a non-positive pageSize.
func PageCount(total, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}

	n := total / pageSize
	if total%pageSize != 0 {
		n++
	}

	return max(n, 1)
}

// OrDash returns s, or "-" when s is empty.
func OrDash(s string) string {
	return output.OrDash(s)
}
