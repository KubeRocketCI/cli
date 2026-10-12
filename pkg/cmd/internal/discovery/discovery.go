// Package discovery holds the table cells, section output and empty-result note
// shared by the deployment-discovery verbs.
// Importable only from packages below pkg/cmd/.
package discovery

import (
	"fmt"
	"io"
	"strings"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/ptr"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
)

// ShortDigestLen is the maximum length of an image digest in table mode: the
// `sha256:` prefix plus the first 8 hex characters.
const ShortDigestLen = len("sha256:") + 8

// MaxIngressHostLen caps the visible width of an ingress hostname in table
// mode. The OSC 8 hyperlink target carries the full URL regardless.
const MaxIngressHostLen = 50

// PrintEmptyNote writes the note of an empty result to stderr. Under -o json
// it writes nothing: the empty array is the answer.
func PrintEmptyNote(ios *iostreams.IOStreams, outputFormat, note string) error {
	if outputFormat == render.FormatJSON {
		return nil
	}

	_, err := fmt.Fprintln(ios.ErrOut, note)

	return err
}

// Bullet returns a PrintSection line: "  - " followed by text on one row with
// its control characters visible. The prefix stays outside SingleLine.
func Bullet(text string) string {
	return "  - " + output.SingleLine(text)
}

// PrintSection prints a blank line, the heading "<title> (<number of lines>)"
// and the lines. It prints nothing when there are no lines.
func PrintSection(w io.Writer, isTTY bool, title string, lines []string) error {
	if len(lines) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := output.PrintHeading(w, isTTY, fmt.Sprintf("%s (%d)", title, len(lines))); err != nil {
		return err
	}

	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}

	return nil
}

// OptCell renders an optional string field, output.EmptyCell when the pointer
// is nil or the value is empty.
func OptCell(s *string) string {
	return output.OrDash(ptr.Deref(s, ""))
}

// OptTimeCell renders an optional RFC 3339 timestamp as a relative time,
// output.EmptyCell when the pointer is nil.
func OptTimeCell(at *string) string {
	if at == nil {
		return output.EmptyCell
	}

	return output.FormatRelativeTime(*at)
}

// OptStatusCell renders an ArgoCD health-status cell with optional TTY color,
// output.EmptyCell when the pointer is nil or the value is empty.
func OptStatusCell(s *string, isTTY bool) string {
	v := ptr.Deref(s, "")
	if isTTY && v != "" {
		return ColorForArgoStatus(v)
	}

	return output.OrDash(v)
}

// ShortDigestCell truncates a digest to ShortDigestLen characters for table
// display. output.EmptyCell when the pointer is nil or the value is empty.
func ShortDigestCell(digest *string) string {
	if digest == nil || *digest == "" {
		return output.EmptyCell
	}

	d := *digest
	if len(d) <= ShortDigestLen {
		return d
	}

	return d[:ShortDigestLen]
}

// IngressLines renders an ingress-URL slice as one cell per URL. Each line is
// the hostname (no scheme), truncated to MaxIngressHostLen visible characters
// and wrapped in an OSC 8 hyperlink (full URL preserved as the link target)
// when isTTY. Returns nil when urls is empty; callers display a single "-"
// row in that case.
func IngressLines(urls []string, isTTY bool) []string {
	if len(urls) == 0 {
		return nil
	}

	lines := make([]string, 0, len(urls))
	for _, u := range urls {
		host := output.Truncate(HostnameFromURL(u), MaxIngressHostLen)
		if isTTY {
			host = output.Hyperlink(host, u)
		}
		lines = append(lines, host)
	}

	return lines
}

// ExpandIngressRows turns one logical row (cells without INGRESS) plus a list
// of ingress lines into N visual rows. The first visual row carries the full
// data; subsequent rows blank out every cell except INGRESS, producing a
// vertically-stacked layout:
//
//	project1   …    ingress1
//	                ingress2
//	project2   …    ingress1
//
// When ingressLines is empty, a single row is emitted with "-" in INGRESS.
func ExpandIngressRows(cells []string, ingressLines []string) [][]string {
	if len(ingressLines) == 0 {
		row := make([]string, 0, len(cells)+1)
		row = append(row, cells...)
		row = append(row, output.EmptyCell)
		return [][]string{row}
	}

	out := make([][]string, 0, len(ingressLines))

	first := make([]string, 0, len(cells)+1)
	first = append(first, cells...)
	first = append(first, ingressLines[0])
	out = append(out, first)

	for _, ing := range ingressLines[1:] {
		row := make([]string, len(cells)+1)
		row[len(cells)] = ing
		out = append(out, row)
	}

	return out
}

// HostnameFromURL strips the scheme and the path from a URL, returning
// host[:port]. Input without "://" has only its path stripped.
func HostnameFromURL(u string) string {
	stripped := u

	if idx := strings.Index(stripped, "://"); idx >= 0 {
		stripped = stripped[idx+3:]
	}

	if idx := strings.Index(stripped, "/"); idx >= 0 {
		stripped = stripped[:idx]
	}

	return stripped
}

// ColorForArgoStatus renders an ArgoCD health-status string with a TTY color.
// healthy → green, degraded/missing → red, progressing → blue (matching the
// running-pipeline color in `output.PipelineStatusColor`). Anything else
// (including suspended/unknown) is made printable on one row and left unstyled.
func ColorForArgoStatus(s string) string {
	switch portal.ArgoHealthStatus(strings.ToLower(s)) {
	case portal.ArgoHealthHealthy:
		return output.GreenText(s)
	case portal.ArgoHealthDegraded, portal.ArgoHealthMissing:
		return output.RedText(s)
	case portal.ArgoHealthProgressing:
		return output.BlueText(s)
	default:
		return output.SingleLine(s)
	}
}
