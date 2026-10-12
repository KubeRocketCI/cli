package render

import (
	"fmt"
	"slices"
	"strings"
)

// The `-o` values.
const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatYAML  = "yaml"
)

// OutputFlagUsage is the `-o` help text of the verbs that accept json and table.
const OutputFlagUsage = "Output format: table, json (default: table)"

// ResolveFormat returns explicit, or FormatTable when it is empty.
func ResolveFormat(explicit string) string {
	if explicit != "" {
		return explicit
	}

	return FormatTable
}

// ValidateOutputFormat accepts "" (the default) and every value in allowed;
// the match is case-sensitive. Any other value is an unknown-format error
// that lists allowed, in order. allowed must not be empty.
func ValidateOutputFormat(format string, allowed ...string) error {
	if len(allowed) == 0 {
		panic("render.ValidateOutputFormat: allowed is empty")
	}

	if format == "" || slices.Contains(allowed, format) {
		return nil
	}

	return unknownFormatError(format, allowed...)
}

// ValidateJSONOrTable is ValidateOutputFormat with json and table allowed.
func ValidateJSONOrTable(format string) error {
	return ValidateOutputFormat(format, FormatJSON, FormatTable)
}

// unknownFormatError reads `unknown output format: xml (use 'json' or 'table')`
// for two values and `(use 'json', 'yaml', or 'table')` for three.
func unknownFormatError(format string, allowed ...string) error {
	quoted := make([]string, len(allowed))
	for i, v := range allowed {
		quoted[i] = "'" + v + "'"
	}

	list := strings.Join(quoted, " or ")
	if len(quoted) > 2 {
		list = strings.Join(quoted[:len(quoted)-1], ", ") + ", or " + quoted[len(quoted)-1]
	}

	return fmt.Errorf("unknown output format: %s (use %s)", format, list)
}
