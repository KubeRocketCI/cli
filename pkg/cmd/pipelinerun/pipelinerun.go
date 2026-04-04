// Package pipelinerun implements the "krci pipelinerun" command group.
package pipelinerun

import (
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/pipelinerun/get"
	"github.com/KubeRocketCI/cli/pkg/cmd/pipelinerun/list"
)

// NewCmdPipelineRun returns the "pipelinerun" group cobra.Command with all subcommands attached.
func NewCmdPipelineRun(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pipelinerun",
		Short:   "Manage pipeline runs",
		Aliases: []string{"run"},
	}

	cmd.AddCommand(
		list.NewCmdList(f, nil),
		get.NewCmdGet(f, nil),
	)

	return cmd
}
