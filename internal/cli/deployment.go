package cli

import "github.com/spf13/cobra"

func (a *App) newDeploymentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deployment",
		Short:   "Manage deployments (CDPipelines)",
		Aliases: []string{"dp"},
	}

	cmd.AddCommand(
		a.newDeploymentListCmd(),
		a.newDeploymentGetCmd(),
	)

	return cmd
}
