package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/fatih/color"

	"github.com/KubeRocketCI/cli/internal/k8s"
)

const (
	outputFormatTable = "table"
	outputFormatJSON  = "json"
)

// ANSI color code used for borders and header accents.
const accentColor = "99"

// Style definitions shared across all commands.
var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor)).Padding(0, 1)
	evenRowStyle = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("252"))
	oddRowStyle  = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	borderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(accentColor))
	labelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Width(14)
	valueStyle   = lipgloss.NewStyle()
)

// Status color helpers (fatih/color auto-disables when stdout is not a TTY).
var (
	greenText  = color.New(color.FgGreen).SprintFunc()
	yellowText = color.New(color.FgYellow).SprintFunc()
	redText    = color.New(color.FgRed).SprintFunc()
)

// statusColor returns the status string with color applied based on its value.
func statusColor(status string) string {
	switch strings.ToLower(status) {
	case k8s.StatusCreated:
		return greenText(status)
	case k8s.StatusInProgress:
		return yellowText(status)
	case k8s.StatusFailed:
		return redText(status)
	default:
		return status
	}
}

// availableText returns a colorized "Yes" or "No" instead of true/false.
func availableText(available bool) string {
	if available {
		return greenText("Yes")
	}

	return redText("No")
}

// printStyledTable renders a lipgloss table with rounded borders and styled rows.
func printStyledTable(w io.Writer, headers []string, rows [][]string) error {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}

			if row%2 == 0 {
				return evenRowStyle
			}

			return oddRowStyle
		}).
		Headers(headers...).
		Rows(rows...)

	_, err := fmt.Fprintln(w, t)

	return err
}

// printTable renders a plain-text table for piped or non-TTY output.
func printTable(w io.Writer, headers []string, rows [][]string) error {
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

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}

// resolveFormat returns the explicit format if provided, otherwise auto-detects
// based on whether the writer is a terminal (table) or a pipe/file (JSON).
func resolveFormat(explicit string, w io.Writer) string {
	if explicit != "" {
		return explicit
	}

	if isTerminal(w) {
		return outputFormatTable
	}

	return outputFormatJSON
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}

	info, err := f.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

// detailRenderer defines styled and plain rendering for a resource detail view.
type detailRenderer[T any] struct {
	styled func(io.Writer, T) error
	plain  func(io.Writer, T) error
}

// renderDetail handles the common format-resolution and output logic for detail commands.
func renderDetail[T any](w io.Writer, output string, data T, r detailRenderer[T]) error {
	format := resolveFormat(output, w)

	switch format {
	case outputFormatJSON:
		return printJSON(w, data)
	case outputFormatTable:
		if isTerminal(w) {
			return r.styled(w, data)
		}

		return r.plain(w, data)
	default:
		return fmt.Errorf("unknown output format: %s (use 'json' or 'table')", format)
	}
}

// printStyledDetailLines renders detail lines with lipgloss styling.
func printStyledDetailLines(w io.Writer, lines []detailLine) error {
	for _, l := range lines {
		v := l.styled
		if v == "" {
			v = valueStyle.Render(l.value)
		}

		if _, err := fmt.Fprintf(w, "%s %s\n", labelStyle.Render(l.label+":"), v); err != nil {
			return err
		}
	}

	return nil
}

// printPlainDetailLines renders detail lines as plain text for piped output.
func printPlainDetailLines(w io.Writer, lines []detailLine) error {
	for _, l := range lines {
		if _, err := fmt.Fprintf(w, "%-14s%s\n", l.label+":", l.value); err != nil {
			return err
		}
	}

	return nil
}
