package render

import (
	"fmt"
	"slices"
	"strings"
)

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
