// Package output provides rendering utilities for CLI command output.
package output

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"gopkg.in/yaml.v3"

	"github.com/KubeRocketCI/cli/internal/portal"
)

// ANSI color code used for borders and header accents.
const accentColor = "99"

// detailLabelWidth is the column width of "<label>:" in detail output.
const detailLabelWidth = 14

// Style definitions shared across all commands.
var (
	HeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor)).Padding(0, 1)
	EvenRowStyle = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.BrightWhite)
	OddRowStyle  = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	BorderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(accentColor))
	LabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Width(detailLabelWidth)
	ValueStyle   = lipgloss.NewStyle()
)

// Status color styles using lipgloss 4-bit color constants.
var (
	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	blueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Blue)
	grayStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// StatusColor returns the status string with color applied based on its value.
// Recognized values (from KubeRocketCI CRD status field):
//   - "created"     renders green
//   - "in_progress" renders yellow
//   - "failed"      renders red
//
// Any other value is returned unstyled.
func StatusColor(status string) string {
	switch strings.ToLower(status) {
	case "created":
		return greenStyle.Render(status)
	case "in_progress":
		return yellowStyle.Render(status)
	case "failed":
		return redStyle.Render(status)
	default:
		return status
	}
}

func PipelineStatusColor(status string) string {
	switch status {
	case portal.StatusSucceeded:
		return greenStyle.Render(status)
	case portal.StatusFailed, portal.StatusCancelled, portal.StatusTimeout:
		return redStyle.Render(status)
	case portal.StatusRunning:
		return blueStyle.Render(status)
	default:
		return status
	}
}

// AvailableText returns a colorized "Yes" or "No" instead of true/false.
func AvailableText(available bool) string {
	if available {
		return greenStyle.Render("Yes")
	}

	return redStyle.Render("No")
}

// FormatRelativeTime converts an RFC3339 timestamp to a short relative display.
// Returns the original string if parsing fails.
func FormatRelativeTime(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}

	return RelativeTime(t)
}

// RelativeTime renders t as a short "Xm ago"/"Xh ago"/"Xd ago" string, or a
// Jan-Mon date for anything older than a week.
func RelativeTime(t time.Time) string {
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 02 15:04")
	}
}

// EmptyCell is the table placeholder for an absent value.
const EmptyCell = "-"

// OrDash returns s, or EmptyCell when s is empty.
func OrDash(s string) string {
	if s == "" {
		return EmptyCell
	}

	return s
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

// MaxMessageLen caps a free-text MESSAGE column on a TTY. Piped output is not
// truncated.
const MaxMessageLen = 80

// Truncate shortens s to maxWidth characters, appending "..." if truncated.
// Returns s unchanged if it fits within maxWidth or maxWidth is 0 (no limit).
func Truncate(s string, maxWidth int) string {
	if maxWidth <= 0 || utf8.RuneCountInString(s) <= maxWidth {
		return s
	}

	runes := []rune(s)
	if maxWidth <= 3 {
		return string(runes[:maxWidth])
	}

	return string(runes[:maxWidth-3]) + "..."
}

// SingleLine collapses every whitespace run, newlines included, into one
// space so a multi-line operator message fits one display row. It drops every
// other control character: the text comes from the cluster, and an escape
// sequence in it would drive the terminal.
func SingleLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return -1
		}

		return r
	}, s)

	return strings.Join(strings.Fields(s), " ")
}

func Hyperlink(text, url string) string {
	if url == "" {
		return text
	}

	return lipgloss.NewStyle().Hyperlink(url).Render(text)
}

// GreenText returns s rendered in green.
func GreenText(s string) string { return greenStyle.Render(s) }

// YellowText returns s rendered in yellow.
func YellowText(s string) string { return yellowStyle.Render(s) }

// RedText returns s rendered in red.
func RedText(s string) string { return redStyle.Render(s) }

// BlueText returns s rendered in blue.
func BlueText(s string) string { return blueStyle.Render(s) }

// PrintTable writes a styled table when isTTY, a plain tabwriter table
// otherwise.
func PrintTable(w io.Writer, isTTY bool, headers []string, rows [][]string) error {
	if isTTY {
		return printStyledTable(w, headers, rows)
	}

	return printPlainTable(w, headers, rows)
}

// printStyledTable renders a lipgloss table with colored headers, alternating row colors, and no borders.
// Uses lipgloss for ANSI-aware column width calculation.
func printStyledTable(w io.Writer, headers []string, rows [][]string) error {
	t := table.New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return HeaderStyle
			}

			if row%2 == 0 {
				return EvenRowStyle
			}

			return OddRowStyle
		}).
		Headers(headers...).
		Rows(rows...)

	_, err := lipgloss.Fprintln(w, t)

	return err
}

// printPlainTable renders a plain-text table for piped or non-TTY output.
func printPlainTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)

	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}

	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}

	return tw.Flush()
}

// PrintHeading writes a section heading: HeaderStyle when isTTY, "<text>:"
// otherwise.
func PrintHeading(w io.Writer, isTTY bool, text string) error {
	if isTTY {
		_, err := fmt.Fprintln(w, HeaderStyle.Render(text))
		return err
	}

	_, err := fmt.Fprintln(w, text+":")

	return err
}

// PrintJSON encodes v as indented JSON to w.
func PrintJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}

func PrintYAML(w io.Writer, v any) error {
	enc := yaml.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		return err
	}

	return enc.Close()
}

// SCASeverityColor colorizes a Dep-Track vulnerability severity for TTY
// rendering. Unknown severities pass through unstyled so future values never
// break existing renderers.
//
//	CRITICAL  → red         HIGH      → yellow
//	MEDIUM    → blue        LOW       → gray
//	INFO      → gray        UNASSIGNED→ gray
func SCASeverityColor(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return redStyle.Render(severity)
	case "HIGH":
		return yellowStyle.Render(severity)
	case "MEDIUM":
		return blueStyle.Render(severity)
	case "LOW", "INFO", "UNASSIGNED":
		return grayStyle.Render(severity)
	default:
		return severity
	}
}

// SonarGateStatusColor colorizes a SonarQube quality-gate / issue status value.
// Unknown values pass through unstyled.
func SonarGateStatusColor(status portal.QualityGateStatus) string {
	s := string(status)

	switch status {
	case portal.QualityGateOK:
		return greenStyle.Render(s)
	case portal.QualityGateWarn:
		return yellowStyle.Render(s)
	case portal.QualityGateError:
		return redStyle.Render(s)
	case portal.QualityGateNone:
		return grayStyle.Render(s)
	default:
		return s
	}
}

// DetailLine holds one label-value pair for resource detail rendering.
// In a terminal, a non-empty Styled is shown instead of Value.
type DetailLine struct {
	Label  string
	Value  string
	Styled string
}

// PrintDetailLines writes one "<label>: <value>" line per entry. Plain output
// pads "<label>:" to detailLabelWidth columns. When isTTY, labels use
// LabelStyle and a non-empty Styled replaces the ValueStyle rendering of Value.
func PrintDetailLines(w io.Writer, isTTY bool, lines []DetailLine) error {
	for _, l := range lines {
		var err error
		if isTTY {
			_, err = lipgloss.Fprintf(w, "%s %s\n", LabelStyle.Render(l.Label+":"), cmp.Or(l.Styled, ValueStyle.Render(l.Value)))
		} else {
			_, err = fmt.Fprintf(w, "%-*s%s\n", detailLabelWidth, l.Label+":", l.Value)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// alignedLabelStyle is LabelStyle without its fixed width; PrintAligned pads
// labels to the longest one itself.
var alignedLabelStyle = LabelStyle.UnsetWidth()

// PrintAligned writes "<indent><label><pad>   <value>" per line, padding every
// label to the longest in lines. When isTTY, labels use alignedLabelStyle and
// a non-empty Styled replaces Value.
func PrintAligned(w io.Writer, isTTY bool, lines []DetailLine, indent string) error {
	width := 0
	for _, l := range lines {
		width = max(width, len(l.Label))
	}

	for _, l := range lines {
		label, value := l.Label, l.Value
		if isTTY {
			label = alignedLabelStyle.Render(l.Label)
			value = cmp.Or(l.Styled, l.Value)
		}

		if _, err := fmt.Fprintf(w, "%s%s%*s   %s\n", indent, label, width-len(l.Label), "", value); err != nil {
			return err
		}
	}

	return nil
}
