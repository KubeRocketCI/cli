// Package output provides rendering utilities for CLI command output.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/KubeRocketCI/cli/internal/iostreams"
)

const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// ANSI color code used for borders and header accents.
const accentColor = "99"

// Style definitions shared across all commands.
var (
	HeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor)).Padding(0, 1)
	EvenRowStyle = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.BrightWhite)
	OddRowStyle  = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	BorderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(accentColor))
	LabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Width(14)
	ValueStyle   = lipgloss.NewStyle()
)

// Status color styles using lipgloss 4-bit color constants.
var (
	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Red)
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

// AvailableText returns a colorized "Yes" or "No" instead of true/false.
func AvailableText(available bool) string {
	if available {
		return greenStyle.Render("Yes")
	}

	return redStyle.Render("No")
}

// GreenText returns s rendered in green.
func GreenText(s string) string { return greenStyle.Render(s) }

// YellowText returns s rendered in yellow.
func YellowText(s string) string { return yellowStyle.Render(s) }

// PrintStyledTable renders a lipgloss table with rounded borders and styled rows.
func PrintStyledTable(w io.Writer, headers []string, rows [][]string) error {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(BorderStyle).
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

// PrintTable renders a plain-text table for piped or non-TTY output.
func PrintTable(w io.Writer, headers []string, rows [][]string) error {
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

// PrintJSON encodes v as indented JSON to w.
func PrintJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}

// ResolveFormat returns the explicit format if provided, otherwise defaults to table.
// Use -o json for JSON output.
func ResolveFormat(explicit string) string {
	if explicit != "" {
		return explicit
	}

	return FormatTable
}

// DetailLine holds one label-value pair for resource detail rendering.
// When Styled is populated, the styled renderer uses it instead of Value.
type DetailLine struct {
	Label  string
	Value  string
	Styled string
}

// DetailRenderer defines styled and plain rendering for a resource detail view.
type DetailRenderer[T any] struct {
	Styled func(io.Writer, T) error
	Plain  func(io.Writer, T) error
}

// RenderList handles the common format-resolution and output logic for list commands.
// toRows receives the isTTY flag so callers can apply color only when rendering to a terminal.
func RenderList[T any](
	ios *iostreams.IOStreams,
	outputFormat string,
	data T,
	toRows func(isTTY bool) (headers []string, rows [][]string),
) error {
	isTTY := ios.IsStdoutTTY()
	format := ResolveFormat(outputFormat)

	switch format {
	case FormatJSON:
		return PrintJSON(ios.Out, data)
	case FormatTable:
		headers, rows := toRows(isTTY)
		if isTTY {
			return PrintStyledTable(ios.Out, headers, rows)
		}

		return PrintTable(ios.Out, headers, rows)
	default:
		return fmt.Errorf("unknown output format: %s (use 'json' or 'table')", format)
	}
}

// RenderDetail handles the common format-resolution and output logic for detail commands.
func RenderDetail[T any](ios *iostreams.IOStreams, outputFormat string, data T, r DetailRenderer[T]) error {
	format := ResolveFormat(outputFormat)

	switch format {
	case FormatJSON:
		return PrintJSON(ios.Out, data)
	case FormatTable:
		if ios.IsStdoutTTY() {
			return r.Styled(ios.Out, data)
		}

		return r.Plain(ios.Out, data)
	default:
		return fmt.Errorf("unknown output format: %s (use 'json' or 'table')", format)
	}
}

// PrintStyledDetailLines renders detail lines with lipgloss styling.
func PrintStyledDetailLines(w io.Writer, lines []DetailLine) error {
	for _, l := range lines {
		v := l.Styled
		if v == "" {
			v = ValueStyle.Render(l.Value)
		}

		if _, err := lipgloss.Fprintf(w, "%s %s\n", LabelStyle.Render(l.Label+":"), v); err != nil {
			return err
		}
	}

	return nil
}

// PrintPlainDetailLines renders detail lines as plain text for piped output.
func PrintPlainDetailLines(w io.Writer, lines []DetailLine) error {
	for _, l := range lines {
		if _, err := fmt.Fprintf(w, "%-14s%s\n", l.Label+":", l.Value); err != nil {
			return err
		}
	}

	return nil
}
