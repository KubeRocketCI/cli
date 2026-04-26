// Package discovery holds helpers shared by the deployment-discovery verbs
// (`krci env list`, `krci env get`, `krci project deployments`):
// DNS-1123 validation, output-format dispatch, JSON envelope rendering, and
// shared error promotion. Placed under `pkg/cmd/internal/` so Go enforces
// package visibility: only packages below `pkg/cmd/...` may import it.
package discovery

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// SchemaVersion is the shared JSON envelope version for every deployment-
// discovery verb. Bump only when the envelope shape changes in a way scripting
// consumers must detect.
const SchemaVersion = "1"

// ShortDigestLen is the maximum length of an image digest as shown in table
// mode: `sha256:` (7 chars) + first 8 hex chars = 15 visible characters.
const ShortDigestLen = 15

// MaxIngressHostLen caps the visible width of an ingress hostname in table
// mode (mirrors the pipelinerun-list approach of truncating long names).
// The OSC 8 hyperlink target carries the full URL regardless.
const MaxIngressHostLen = 50

// validateDNS1123Label rejects empty / over-length / non-DNS-1123 values,
// formatting messages with the supplied placeholder (e.g. `<deployment>`,
// `--cluster`).
func validateDNS1123Label(placeholder, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", placeholder)
	}

	if len(value) > cmdutil.DNS1123SubdomainMaxLength {
		return fmt.Errorf("%s must be at most %d characters (DNS-1123)", placeholder, cmdutil.DNS1123SubdomainMaxLength)
	}

	if !cmdutil.IsValidDNS1123Label(value) {
		return fmt.Errorf("%s must be a valid DNS-1123 label", placeholder)
	}

	return nil
}

// ValidateDeployment rejects values that fail DNS-1123 label validation. Used
// by both the positional `<deployment>` (env get) and the `--deployment` flag
// value (env list).
func ValidateDeployment(deployment string) error {
	return validateDNS1123Label("<deployment>", deployment)
}

// ValidateEnv rejects values that fail DNS-1123 label validation. <env> maps
// to Stage.spec.name (a short user-facing identifier like "dev", "stage",
// "prod").
func ValidateEnv(env string) error {
	return validateDNS1123Label("<env>", env)
}

// ValidateCluster rejects values that fail DNS-1123 label validation when the
// `--cluster` flag is supplied with a non-empty value. Empty is allowed since
// the flag is optional.
func ValidateCluster(cluster string) error {
	if cluster == "" {
		return nil
	}

	return validateDNS1123Label("--cluster", cluster)
}

// ValidateProject rejects project names that fail DNS-1123 label validation.
// Used by `krci project deployments <project>`.
func ValidateProject(project string) error {
	return validateDNS1123Label("<project>", project)
}

// ValidateOutputFormat rejects `-o` values other than "", "table", or "json".
func ValidateOutputFormat(format string) error {
	switch format {
	case "", output.FormatTable, output.FormatJSON:
		return nil
	default:
		return fmt.Errorf("unknown output format: %s (use 'json' or 'table')", format)
	}
}

// TableRenderer emits the table/text view. The callback receives the writer
// and the TTY state so it can apply styling or truncation conditionally.
type TableRenderer func(w io.Writer, isTTY bool) error

// Render dispatches on the `-o` flag: emit `{schemaVersion, data}` envelope
// for `-o json`, invoke the supplied TableRenderer for `-o table` (default),
// or return a validation error.
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

// HandleError promotes portal.ErrUnauthorized to the shared
// "run krci auth login" message and — when `-o json` is selected — also
// writes the `{schemaVersion, error: { message }}` envelope to stdout so
// scripting consumers get a structured error alongside the exit-1 signal.
func HandleError(ios *iostreams.IOStreams, outputFormat string, err error) error {
	if errors.Is(err, portal.ErrUnauthorized) {
		err = cmdutil.ErrAuthRequired(err)
	}

	if output.ResolveFormat(outputFormat) == output.FormatJSON {
		_ = output.PrintJSONErrorEnvelope(ios.Out, SchemaVersion, err)
	}

	return err
}

// PrintTable is the shared TTY/non-TTY dispatcher.
func PrintTable(w io.Writer, isTTY bool, headers []string, rows [][]string) error {
	if isTTY {
		return output.PrintStyledTable(w, headers, rows)
	}

	return output.PrintTable(w, headers, rows)
}

// OptCell renders an optional string field as the dereferenced value, or "-"
// when the pointer is nil or the value is empty.
func OptCell(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}

// OptStatusCell renders an ArgoCD health-status cell with optional TTY color.
// "-" when the pointer is nil or the value is empty.
func OptStatusCell(s *string, isTTY bool) string {
	if s == nil || *s == "" {
		return "-"
	}

	if isTTY {
		return ColorForArgoStatus(*s)
	}

	return *s
}

// ShortDigestCell shortens a sha256 digest for table display: keeps the
// `sha256:` prefix plus the first 8 hex chars (ShortDigestLen visible chars).
// Returns "-" when the pointer is nil or the value is empty.
func ShortDigestCell(digest *string) string {
	if digest == nil || *digest == "" {
		return "-"
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
		row = append(row, "-")
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
