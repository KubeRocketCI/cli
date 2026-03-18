// Package deployment implements the "krci deployment" command group.
package deployment

import (
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/deployment/get"
	"github.com/KubeRocketCI/cli/pkg/cmd/deployment/list"
)

// NewCmdDeployment returns the "deployment" group cobra.Command with all subcommands attached.
func NewCmdDeployment(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deployment",
		Short:   "Manage deployments (CDPipelines)",
		Aliases: []string{"dp"},
	}

	cmd.AddCommand(
		list.NewCmdList(f, nil),
		get.NewCmdGet(f, nil),
	)

	return cmd
}
