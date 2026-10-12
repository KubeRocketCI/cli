// Package discovery holds the table cells and section output shared by the
// deployment-discovery verbs. SchemaVersion, TableRenderer, Render,
// HandleError and ValidateOutputFormat delegate to
// pkg/cmd/internal/render; new callers import render directly.
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

// SchemaVersion is render.SchemaVersion.
const SchemaVersion = render.SchemaVersion

// ShortDigestLen is the maximum length of an image digest as shown in table
// mode: `sha256:` (7 chars) + first 8 hex chars = 15 visible characters.
const ShortDigestLen = 15

// MaxIngressHostLen caps the visible width of an ingress hostname in table
// mode (mirrors the pipelinerun-list approach of truncating long names).
// The OSC 8 hyperlink target carries the full URL regardless.
const MaxIngressHostLen = 50

// TableRenderer is render.TableRenderer.
type TableRenderer = render.TableRenderer

// ValidateOutputFormat is render.ValidateOutputFormat with "json" and "table"
// allowed.
func ValidateOutputFormat(format string) error {
	return render.ValidateOutputFormat(format, output.FormatJSON, output.FormatTable)
}

// Render is render.Render.
func Render[T any](ios *iostreams.IOStreams, outputFormat string, data T, renderTable TableRenderer) error {
	return render.Render(ios, outputFormat, data, renderTable)
}

// HandleError is render.HandleError.
func HandleError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	return render.HandleError(ios, outputFormat, err)
}

// PrintEmptyNote writes the note of an empty result to stderr. Under -o json
// it writes nothing: the empty array is the answer.
func PrintEmptyNote(ios *iostreams.IOStreams, outputFormat, note string) error {
	if outputFormat == output.FormatJSON {
		return nil
	}

	_, err := fmt.Fprintln(ios.ErrOut, note)

	return err
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

// ShortDigestCell shortens a sha256 digest for table display: keeps the
// `sha256:` prefix plus the first 8 hex chars (ShortDigestLen visible chars).
// output.EmptyCell when the pointer is nil or the value is empty.
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

// HostnameFromURL strips the scheme + path from a URL, returning just the
// host. Falls back to the original string if no scheme separator is present.
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
// (including suspended/unknown) passes through unstyled.
func ColorForArgoStatus(s string) string {
	switch portal.ArgoHealthStatus(strings.ToLower(s)) {
	case portal.ArgoHealthHealthy:
		return output.GreenText(s)
	case portal.ArgoHealthDegraded, portal.ArgoHealthMissing:
		return output.RedText(s)
	case portal.ArgoHealthProgressing:
		return output.BlueText(s)
	default:
		return s
	}
}
