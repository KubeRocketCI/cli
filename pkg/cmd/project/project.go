// Package project implements the "krci project" command group.
package project

import (
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/project/build"
	"github.com/KubeRocketCI/cli/pkg/cmd/project/deployments"
	"github.com/KubeRocketCI/cli/pkg/cmd/project/get"
	"github.com/KubeRocketCI/cli/pkg/cmd/project/list"
	"github.com/KubeRocketCI/cli/pkg/cmd/project/versions"
)

// NewCmdProject returns the "project" group cobra.Command with all subcommands attached.
func NewCmdProject(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Short:   "Manage projects",
		Aliases: []string{"proj"},
	}

	cmd.AddCommand(
		list.NewCmdList(f, nil),
		get.NewCmdGet(f, nil),
		deployments.NewCmdDeployments(f, nil),
		versions.NewCmdVersions(f, nil),
		build.NewCmdBuild(f, nil),
	)

	return cmd
}
