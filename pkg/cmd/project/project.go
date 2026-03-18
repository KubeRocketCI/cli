// Package project implements the "krci project" command group.
package project

import (
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/project/get"
	"github.com/KubeRocketCI/cli/pkg/cmd/project/list"
)

// NewCmdProject returns the "project" group cobra.Command with all subcommands attached.
func NewCmdProject(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Short:   "Manage projects (Codebases)",
		Aliases: []string{"proj"},
	}

	cmd.AddCommand(
		list.NewCmdList(f, nil),
		get.NewCmdGet(f, nil),
	)

	return cmd
}
