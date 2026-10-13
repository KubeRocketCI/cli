// Package output provides rendering utilities for CLI command output.
package output

import (
	"bytes"
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
	"github.com/KubeRocketCI/cli/internal/termseq"
)

// ANSI color code used for borders and header accents.
const accentColor = "99"

// ANSI color code used for muted text.
const mutedColor = "245"

// detailLabelWidth is the column width of "<label>:" in detail output.
const detailLabelWidth = 14

// Style definitions shared across all commands.
var (
	HeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor)).Padding(0, 1)
	EvenRowStyle = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.BrightWhite)
	OddRowStyle  = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color(mutedColor))
	BorderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(accentColor))
	LabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(mutedColor)).Width(detailLabelWidth)
	ValueStyle   = lipgloss.NewStyle()
	DimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(mutedColor))
)

// Status color styles using lipgloss 4-bit color constants.
var (
	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	blueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Blue)
	grayStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(mutedColor))
)

// Cell is a table cell or detail value in two forms, plain and styled, each
// printable on one row. Only this package builds a styled form.
type Cell struct {
	plain  string
	styled string
}

// Text is s, printable on one row, unstyled in both forms. Text("") is the zero
// Cell.
func Text(s string) Cell {
	c := cell(s)

	return Cell{plain: c, styled: c}
}

// styledText is the zero Cell for empty text.
func styledText(st lipgloss.Style, s string) Cell {
	c := cell(s)
	if c == "" {
		return Cell{}
	}

	return Cell{plain: c, styled: st.Render(c)}
}

// Texts is Text applied to each of ss.
func Texts(ss ...string) []Cell {
	out := make([]Cell, len(ss))
	for i, s := range ss {
		out[i] = Text(s)
	}

	return out
}

// Green is s, printable on one row, styled green.
func Green(s string) Cell { return styledText(greenStyle, s) }

// Yellow is s, printable on one row, styled yellow.
func Yellow(s string) Cell { return styledText(yellowStyle, s) }

// Red is s, printable on one row, styled red.
func Red(s string) Cell { return styledText(redStyle, s) }

// Blue is s, printable on one row, styled blue.
func Blue(s string) Cell { return styledText(blueStyle, s) }

// Gray is s, printable on one row, styled gray.
func Gray(s string) Cell { return styledText(grayStyle, s) }

// Concat joins cells form by form.
func Concat(cells ...Cell) Cell {
	var plain, styled strings.Builder

	for _, c := range cells {
		plain.WriteString(c.plain)
		styled.WriteString(c.styled)
	}

	return Cell{plain: plain.String(), styled: styled.String()}
}

// Render returns the styled form when isTTY, the plain form otherwise.
func (c Cell) Render(isTTY bool) string {
	if isTTY {
		return c.styled
	}

	return c.plain
}

// IsZero reports whether c is the zero Cell.
func (c Cell) IsZero() bool { return c == Cell{} }

// StatusColor is the KubeRocketCI CRD status as a Cell, styled by value:
//   - "created"     green
//   - "in_progress" yellow
//   - "failed"      red
//
// Any other value is unstyled in both forms.
func StatusColor(status string) Cell {
	switch strings.ToLower(status) {
	case "created":
		return Green(status)
	case "in_progress":
		return Yellow(status)
	case "failed":
		return Red(status)
	default:
		return Text(status)
	}
}

// AvailableText is "Yes" or "No" in place of true/false; the styled form is
// green or red.
func AvailableText(available bool) Cell {
	if available {
		return Green("Yes")
	}

	return Red("No")
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

// printable makes every control character in s visible: a C0 byte other than
// \n, \t and \r in caret notation (ESC is ^[), DEL as ^?, and a C1 rune
// U+0080-U+009F as ^[ plus its 7-bit letter. The line and paragraph separators
// become \n. Bidirectional overrides and isolates are dropped. Invalid UTF-8
// becomes U+FFFD.
func printable(s string) string {
	var b strings.Builder

	b.Grow(len(s))

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size

		switch {
		case r == utf8.RuneError && size == 1:
			b.WriteRune(utf8.RuneError)
		case r == '\n' || r == '\t' || r == '\r':
			b.WriteRune(r)
		case r < 0x20:
			b.WriteByte('^')
			b.WriteByte(byte(r) + 0x40)
		case r == del:
			b.WriteString("^?")
		case unicode.IsControl(r): // C1, U+0080-U+009F
			b.WriteString("^[")
			b.WriteByte(byte(r) - 0x40)
		case unicode.In(r, unicode.Zl, unicode.Zp):
			b.WriteByte('\n')
		case bidi(r):
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}

// bidi reports the bidirectional overrides and isolates.
func bidi(r rune) bool {
	return (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

// SingleLine makes s printable and collapses every whitespace run, newlines
// included, into one space, so a multi-line operator message fits one row.
func SingleLine(s string) string {
	return strings.Join(strings.Fields(printable(s)), " ")
}

var oneRow = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ")

// cell is printable on one display row.
func cell(s string) string { return oneRow.Replace(printable(s)) }

func mapStrings(ss []string, f func(string) string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = f(s)
	}

	return out
}

func renderRows(rows [][]Cell, isTTY bool) [][]string {
	out := make([][]string, len(rows))

	for i, r := range rows {
		out[i] = make([]string, len(r))
		for j, c := range r {
			out[i][j] = c.Render(isTTY)
		}
	}

	return out
}

// Hyperlink is text, printable on one row, styled as an OSC 8 link to target
// when termseq.Link accepts the target, which it percent-encodes. Otherwise
// both forms are the text alone. Empty text is the zero Cell.
func Hyperlink(text, target string) Cell {
	c := Text(text)

	encoded, ok := termseq.Link(target)
	if !ok || c.IsZero() {
		return c
	}

	c.styled = lipgloss.NewStyle().Hyperlink(encoded).Render(c.plain)

	return c
}

// PrintTable writes a styled table of the styled forms when isTTY, a plain
// tabwriter table of the plain forms otherwise. Every header stays on one row
// with its control characters visible.
func PrintTable(w io.Writer, isTTY bool, headers []string, rows [][]Cell) error {
	headers = mapStrings(headers, cell)

	rendered := renderRows(rows, isTTY)

	if isTTY {
		return printStyledTable(w, headers, rendered)
	}

	return printPlainTable(w, headers, rendered)
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

	_, err := fmt.Fprintln(w, t)

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
	text = cell(text)

	if isTTY {
		_, err := fmt.Fprintln(w, HeaderStyle.Render(text))
		return err
	}

	_, err := fmt.Fprintln(w, text+":")

	return err
}

// PrintJSON encodes v as indented JSON to w. DEL and the C1 runes
// U+0080-U+009F are written as \u escapes: the process streams drop them raw.
func PrintJSON(w io.Writer, v any) error {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")

	if err := enc.Encode(v); err != nil {
		return err
	}

	_, err := w.Write(escapeC1(buf.Bytes()))

	return err
}

// escapeC1 replaces DEL and C1 runes with \u escapes. encoding/json emits them
// raw and only inside strings, so the decoded value is unchanged.
func escapeC1(b []byte) []byte {
	isC1 := func(r rune) bool { return r >= del && r <= 0x9f }
	if !bytes.ContainsFunc(b, isC1) {
		return b
	}

	out := make([]byte, 0, len(b)+16)

	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if isC1(r) {
			out = append(out, `\u00`...)
			out = append(out, hexDigit(byte(r)>>4), hexDigit(byte(r)&0xf))
		} else {
			out = append(out, b[i:i+size]...)
		}

		i += size
	}

	return out
}

const del = 0x7f

func hexDigit(n byte) byte { return "0123456789abcdef"[n] }

func PrintYAML(w io.Writer, v any) error {
	enc := yaml.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		return err
	}

	return enc.Close()
}

// SCASeverityColor colorizes a Dep-Track vulnerability severity for TTY
// rendering. An unknown severity is made printable on one row and left
// unstyled, so a future value never breaks a renderer.
//
//	CRITICAL  → red         HIGH      → yellow
//	MEDIUM    → blue        LOW       → gray
//	INFO      → gray        UNASSIGNED→ gray
func SCASeverityColor(severity string) Cell {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return Red(severity)
	case "HIGH":
		return Yellow(severity)
	case "MEDIUM":
		return Blue(severity)
	case "LOW", "INFO", "UNASSIGNED":
		return Gray(severity)
	default:
		return Text(severity)
	}
}

// SonarGateStatusColor colorizes a SonarQube quality-gate / issue status value.
// An unknown value is made printable on one row and left unstyled.
func SonarGateStatusColor(status portal.QualityGateStatus) Cell {
	s := string(status)

	switch status {
	case portal.QualityGateOK:
		return Green(s)
	case portal.QualityGateWarn:
		return Yellow(s)
	case portal.QualityGateError:
		return Red(s)
	case portal.QualityGateNone:
		return Gray(s)
	default:
		return Text(s)
	}
}

// DetailLine holds one label-value pair for resource detail rendering.
// In a terminal, the styled form of a non-zero Styled is shown instead of
// Value. Only the styled form of Styled is used; plain output prints Value.
type DetailLine struct {
	Label  string
	Value  string
	Styled Cell
}

// PrintDetailLines writes one "<label>: <value>" line per entry, each on one
// row with its control characters visible. Plain output pads "<label>:" to
// detailLabelWidth columns. When isTTY, labels use LabelStyle and a non-zero
// Styled replaces the ValueStyle rendering of Value.
func PrintDetailLines(w io.Writer, isTTY bool, lines []DetailLine) error {
	for _, l := range lines {
		label, value := cell(l.Label), cell(l.Value)

		var err error
		if isTTY {
			if l.Styled.IsZero() {
				value = ValueStyle.Render(value)
			} else {
				value = l.Styled.styled
			}

			_, err = fmt.Fprintf(w, "%s %s\n", LabelStyle.Render(label+":"), value)
		} else {
			_, err = fmt.Fprintf(w, "%-*s%s\n", detailLabelWidth, label+":", value)
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

// LabelWidth returns the display width of the longest label across groups, as
// PrintAligned measures it, so sibling blocks can share one label column.
func LabelWidth(groups ...[]DetailLine) int {
	width := 0

	for _, g := range groups {
		for _, l := range g {
			width = max(width, lipgloss.Width(cell(l.Label)))
		}
	}

	return width
}

// PrintAligned writes "<indent><label><pad>   <value>" per line, each on one
// row with its control characters visible. The label column is width or
// LabelWidth(lines), whichever is wider. When isTTY, labels use
// alignedLabelStyle and a non-zero Styled replaces Value.
func PrintAligned(w io.Writer, isTTY bool, lines []DetailLine, indent string, width int) error {
	width = max(width, LabelWidth(lines))

	for _, l := range lines {
		label, value := cell(l.Label), cell(l.Value)
		pad := width - lipgloss.Width(label)

		if isTTY {
			label = alignedLabelStyle.Render(label)
			if !l.Styled.IsZero() {
				value = l.Styled.styled
			}
		}

		if _, err := fmt.Fprintf(w, "%s%s%*s   %s\n", indent, label, pad, "", value); err != nil {
			return err
		}
	}

	return nil
}
