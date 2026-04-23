package scainternal

import (
	"errors"
	"fmt"
	"io"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// SchemaVersion is the shared JSON envelope version for every `krci sca *` verb.
const SchemaVersion = "1"

// TableRenderer emits the table/text view of an sca verb's payload. The
// callback receives the output writer and the TTY state so it can apply
// styling or truncation conditionally.
type TableRenderer func(w io.Writer, isTTY bool) error

// Render dispatches on the `-o` flag: emit `{schemaVersion, data}` for
// `-o json`, invoke the TableRenderer for `-o table` (the default), or return
// a validation error for anything else.
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

// HandleError promotes portal.ErrUnauthorized to the "run krci auth login"
// message; translates portal.ErrUpstreamUnavailable into a user-facing
// "upstream unavailable (dependency-track)" hint; and — when `-o json` is
// selected — also writes a structured error envelope to stdout so scripting
// consumers can read it alongside the exit-1 signal.
func HandleError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	if errors.Is(err, portal.ErrUnauthorized) {
		err = cmdutil.ErrAuthRequired(err)
	} else if errors.Is(err, portal.ErrUpstreamUnavailable) {
		err = fmt.Errorf("upstream unavailable (dependency-track): %w", err)
	}

	if output.ResolveFormat(outputFormat) == output.FormatJSON {
		_ = output.PrintJSONErrorEnvelope(ios.Out, SchemaVersion, err)
	}

	return err
}

// PrintTable is the shared TTY/non-TTY dispatcher for sca verbs that show a
// plain 2D table.
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

// PageFooter formats the paging summary printed below a paginated sca table.
func PageFooter(noun string, total, pageIndex, pageSize int) string {
	totalPages := PageCount(total, pageSize)

	if total == 0 {
		return fmt.Sprintf("no %s (page-size %d)", pluralise(noun, 0), pageSize)
	}

	word := pluralise(noun, total)

	if pageIndex > totalPages {
		return fmt.Sprintf(
			"%d %s — page %d is beyond last page %d (page-size %d)",
			total, word, pageIndex, totalPages, pageSize,
		)
	}

	return fmt.Sprintf(
		"%d %s, page %d of %d (page-size %d)",
		total, word, pageIndex, totalPages, pageSize,
	)
}

// pluralise does the "1 project" / "N projects" / "0 projects" English plural
// for footer output. No i18n here — identical to sonar's inline helper.
func pluralise(noun string, n int) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// EmptyPlaceholder is the unicode em-dash used in TTY output for missing values.
const EmptyPlaceholder = "—"

// OrDash returns s when non-empty, otherwise EmptyPlaceholder. Shared by every
// sca verb that renders a potentially-empty column value.
func OrDash(s string) string {
	if s == "" {
		return EmptyPlaceholder
	}
	return s
}

// BoolYesNo renders a bool as the lowercase "yes" / "no" used by every sca
// table column.
func BoolYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// FormatRisk renders a Dep-Track risk score, using EmptyPlaceholder for zero.
func FormatRisk(r float32) string {
	if r == 0 {
		return EmptyPlaceholder
	}
	return fmt.Sprintf("%.1f", r)
}

// FormatVulnCounts renders the C/H/M/L vulnerability counts column. When isTTY
// is true and there is at least one critical or high finding, the string is
// highlighted yellow so it visually pops in interactive listings.
func FormatVulnCounts(m *portal.SCAMetrics, isTTY bool) string {
	if m == nil {
		return "0/0/0/0"
	}
	s := fmt.Sprintf("%d/%d/%d/%d", m.Critical, m.High, m.Medium, m.Low)
	if isTTY && m.Critical+m.High > 0 {
		return output.YellowText(s)
	}
	return s
}
