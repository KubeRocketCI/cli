package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func (a *App) newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored credentials",
		Long:  "Remove all locally stored tokens and credentials. You will need to run 'krci auth login' again.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.tokenProvider.Logout(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Logged out. Credentials removed.")
			return nil
		},
	}
}
