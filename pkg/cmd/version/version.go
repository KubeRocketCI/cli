// Package version implements the "krci version" command.
package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/iostreams"
)

// NewCmdVersion returns the "version" cobra.Command.
// Version info is provided at construction time from ldflags injected by main.
func NewCmdVersion(ios *iostreams.IOStreams, version, commit, date string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print krci CLI version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(ios.Out,
				"krci version %s (commit: %s, built: %s)\n",
				version, commit, date,
			)

			return err
		},
	}
}
