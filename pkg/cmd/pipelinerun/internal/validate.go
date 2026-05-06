package pipelineruninternal

import (
	"fmt"
	"strings"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/output"
)

// Kinds for ParseKeyValueList error messages.
const (
	KindParameter = "parameter"
	KindLabel     = "label"
)

// ValidatePipelineName returns an error when name does not match the DNS-1123
// subdomain shape. Tekton Pipeline objects follow Kubernetes resource-name
// conventions, which allow up to 253 chars (subdomain shape), not the tighter
// 63-char label ceiling used for codebase/sonar names.
func ValidatePipelineName(name string) error {
	if name == "" {
		return fmt.Errorf("<pipeline> must not be empty")
	}

	if !cmdutil.IsValidDNS1123Subdomain(name) {
		return fmt.Errorf(
			"<pipeline> must be a valid DNS-1123 subdomain (max 253 chars, lowercase alphanumeric, '-' and '.')")
	}

	return nil
}

// ValidateOutputAndDryRun enforces the joint contract between -o and --dry-run.
//
//   - "", "table", "json"  → ok (table only when not dry-run).
//   - "yaml"               → ok only when --dry-run is set.
//   - --dry-run + "table"  → rejected (no row to render for a manifest).
//   - any other -o value   → rejected as unknown.
func ValidateOutputAndDryRun(format string, dryRun bool) error {
	switch format {
	case "", output.FormatJSON:
		return nil
	case output.FormatTable:
		if dryRun {
			return fmt.Errorf("--dry-run cannot use -o table (use -o json or -o yaml)")
		}

		return nil
	case output.FormatYAML:
		if !dryRun {
			return fmt.Errorf("-o yaml requires --dry-run")
		}

		return nil
	default:
		return fmt.Errorf("unknown output format: %s (use 'json', 'yaml', or 'table')", format)
	}
}

// ParseKeyValueList parses a list of `--param key=value` (or `--label key=value`)
// strings into a map.
//
// Rules:
//   - Split on the first `=` only (so values containing `=` are preserved).
//   - Trim surrounding whitespace from key and value.
//   - Empty key after trim → error.
//   - Duplicate keys → error (no last-wins).
//   - Malformed entry (no `=`) → error.
//
// kind is used in error messages and SHOULD be one of KindParameter or KindLabel.
func ParseKeyValueList(values []string, kind string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	out := make(map[string]string, len(values))

	for _, raw := range values {
		rawKey, rawValue, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("%s must be key=value", kind)
		}

		key := strings.TrimSpace(rawKey)
		value := strings.TrimSpace(rawValue)

		if key == "" {
			return nil, fmt.Errorf("%s key must not be empty", kind)
		}

		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate %s '%s'", kind, key)
		}

		out[key] = value
	}

	return out, nil
}
