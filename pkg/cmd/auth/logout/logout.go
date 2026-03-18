// Package logout implements the "krci auth logout" command.
package logout

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
)

// LogoutOptions holds all inputs for the logout command.
type LogoutOptions struct {
	IO            *iostreams.IOStreams
	TokenProvider func() (auth.TokenProvider, error)
}

// NewCmdLogout returns the "auth logout" cobra.Command.
// runF is the business logic function; pass nil to use the default logoutRun.
func NewCmdLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{
		IO:            f.IOStreams,
		TokenProvider: f.TokenProvider,
	}

	return &cobra.Command{
		Use:     "logout",
		Short:   "Clear stored credentials",
		Long:    "Remove all locally stored tokens and credentials. You will need to run 'krci auth login' again.",
		Example: "  krci auth logout",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}

			return logoutRun(opts)
		},
	}
}

func logoutRun(opts *LogoutOptions) error {
	tp, err := opts.TokenProvider()
	if err != nil {
		return err
	}

	if err := tp.Logout(); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(opts.IO.ErrOut, "Logged out. Credentials removed.")

	return nil
}
