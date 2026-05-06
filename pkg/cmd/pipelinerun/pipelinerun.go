// Package pipelinerun implements the "krci pipelinerun" command group.
package pipelinerun

import (
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/pipelinerun/get"
	"github.com/KubeRocketCI/cli/pkg/cmd/pipelinerun/list"
	"github.com/KubeRocketCI/cli/pkg/cmd/pipelinerun/start"
)

func NewCmdPipelineRun(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pipelinerun",
		Short:   "Manage pipeline runs",
		Aliases: []string{"run"},
	}

	cmd.AddCommand(
		list.NewCmdList(f, nil),
		get.NewCmdGet(f, nil),
		start.NewCmdStart(f, nil),
	)

	return cmd
}
