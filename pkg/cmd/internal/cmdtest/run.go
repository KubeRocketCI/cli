package cmdtest

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
)

// RunCmd builds the command with newCmd, executes it with argv, and returns
// the options runF captured. Validation runs; the network does not.
func RunCmd[T any](
	t *testing.T,
	newCmd func(*cmdutil.Factory, func(*T) error) *cobra.Command,
	argv []string,
) (*T, error) {
	t.Helper()

	var captured *T

	cmd := newCmd(NewFactory(), func(o *T) error {
		captured = o

		return nil
	})

	cmd.SetArgs(argv)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	return captured, cmd.Execute()
}
