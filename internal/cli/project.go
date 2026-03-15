package cli

import "github.com/spf13/cobra"

func (a *App) newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Short:   "Manage projects (Codebases)",
		Aliases: []string{"proj"},
	}

	cmd.AddCommand(
		a.newProjectListCmd(),
		a.newProjectGetCmd(),
	)

	return cmd
}
