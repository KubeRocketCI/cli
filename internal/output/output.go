// Package output provides rendering utilities for CLI command output.
package output

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
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
// Any other value is made printable on one row and left unstyled.
func StatusColor(status string) string {
	switch strings.ToLower(status) {
	case "created":
		return greenStyle.Render(status)
	case "in_progress":
		return yellowStyle.Render(status)
	case "failed":
		return redStyle.Render(status)
	default:
		return cell(status)
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
		return cell(status)
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

// sgrShape and linkShape match the shapes of an SGR sequence and an OSC 8
// link. A match passes styledCell only when the termseq policy, the one the
// stream applies, accepts it.
const (
	sgrShape  = `\x1b\[[0-9;]*m`
	linkShape = `\x1b\]8;[^\x1b\x07]*(?:\x1b\\|\x07)`
)

var seqCandidate = regexp.MustCompile(sgrShape + "|" + linkShape)

// styledCell is cell applied to the text between the sequences the stream
// keeps, which pass unchanged. A sequence in cluster text passes too: it
// cannot be told from the caller's own.
func styledCell(s string) string {
	var b strings.Builder

	last := 0

	for _, loc := range seqCandidate.FindAllStringIndex(s, -1) {
		b.WriteString(cell(s[last:loc[0]]))

		seq := s[loc[0]:loc[1]]
		if kept(seq) {
			b.WriteString(seq)
		} else {
			b.WriteString(cell(seq))
		}

		last = loc[1]
	}

	b.WriteString(cell(s[last:]))

	return b.String()
}

// kept reports whether the stream passes seq, a seqCandidate match.
func kept(seq string) bool {
	if seq[1] == '[' {
		_, _, ok := termseq.SGR(seq[2 : len(seq)-1])

		return ok
	}

	payload := strings.TrimSuffix(strings.TrimSuffix(seq[2:], "\x07"), "\x1b\\")
	_, ok := termseq.OSC8(payload)

	return ok
}

func mapStrings(ss []string, f func(string) string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = f(s)
	}

	return out
}

func mapRows(rows [][]string, f func(string) string) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		out[i] = mapStrings(r, f)
	}

	return out
}

// Hyperlink renders text, on one row, as an OSC 8 link to target when
// termseq.Link accepts the target, which it percent-encodes. Otherwise it
// returns the text alone.
func Hyperlink(text, target string) string {
	text = cell(text)

	encoded, ok := termseq.Link(target)
	if !ok {
		return text
	}

	return lipgloss.NewStyle().Hyperlink(encoded).Render(text)
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
// otherwise. Every header and cell stays on one row with its control
// characters visible. The styled table passes SGR and OSC 8 sequences in a
// cell unchanged.
func PrintTable(w io.Writer, isTTY bool, headers []string, rows [][]string) error {
	headers = mapStrings(headers, cell)

	if isTTY {
		return printStyledTable(w, headers, mapRows(rows, styledCell))
	}

	return printPlainTable(w, headers, mapRows(rows, cell))
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
		return cell(severity)
	}
}

// SonarGateStatusColor colorizes a SonarQube quality-gate / issue status value.
// An unknown value is made printable on one row and left unstyled.
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
		return cell(s)
	}
}

// DetailLine holds one label-value pair for resource detail rendering.
// In a terminal, a non-empty Styled is shown instead of Value.
type DetailLine struct {
	Label  string
	Value  string
	Styled string
}

// PrintDetailLines writes one "<label>: <value>" line per entry, each on one
// row with its control characters visible. Plain output pads "<label>:" to
// detailLabelWidth columns. When isTTY, labels use LabelStyle and a non-empty
// Styled replaces the ValueStyle rendering of Value; SGR and OSC 8 sequences
// in it pass unchanged.
func PrintDetailLines(w io.Writer, isTTY bool, lines []DetailLine) error {
	for _, l := range lines {
		label, value := cell(l.Label), cell(l.Value)

		var err error
		if isTTY {
			_, err = fmt.Fprintf(w, "%s %s\n",
				LabelStyle.Render(label+":"), cmp.Or(styledCell(l.Styled), ValueStyle.Render(value)))
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
// alignedLabelStyle and a non-empty Styled replaces Value; SGR and OSC 8
// sequences in it pass unchanged.
func PrintAligned(w io.Writer, isTTY bool, lines []DetailLine, indent string, width int) error {
	width = max(width, LabelWidth(lines))

	for _, l := range lines {
		label, value := cell(l.Label), cell(l.Value)
		pad := width - lipgloss.Width(label)

		if isTTY {
			label = alignedLabelStyle.Render(label)
			value = cmp.Or(styledCell(l.Styled), value)
		}

		if _, err := fmt.Fprintf(w, "%s%s%*s   %s\n", indent, label, pad, "", value); err != nil {
			return err
		}
	}

	return nil
}
