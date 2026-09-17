package scatestutil

import (
	"testing"

	"github.com/spf13/cobra"
)

// FlagUsage returns the usage text of the named flag. A missing flag fails
// the test.
func FlagUsage(t *testing.T, cmd *cobra.Command, name string) string {
	t.Helper()

	if f := cmd.Flag(name); f != nil {
		return f.Usage
	}

	t.Fatalf("--%s flag missing", name)

	return ""
}
