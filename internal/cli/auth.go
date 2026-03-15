package cli

import "github.com/spf13/cobra"

func (a *App) newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
	}

	cmd.AddCommand(
		a.newAuthLoginCmd(),
		a.newAuthStatusCmd(),
		a.newAuthLogoutCmd(),
	)

	return cmd
}
