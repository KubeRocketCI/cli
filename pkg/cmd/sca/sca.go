// Package sca implements the "krci sca" command group (Software Composition
// Analysis views backed by Dependency-Track).
package sca

import (
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/sca/components"
	"github.com/KubeRocketCI/cli/pkg/cmd/sca/findings"
	"github.com/KubeRocketCI/cli/pkg/cmd/sca/get"
	"github.com/KubeRocketCI/cli/pkg/cmd/sca/list"
)

// NewCmdSca returns the "sca" group cobra.Command with all subcommands attached.
func NewCmdSca(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sca",
		Short: "Inspect Dependency-Track projects, components, and vulnerability findings",
		Long: `Inspect Software Composition Analysis (SCA) state — projects, dependencies,
and vulnerability findings — without leaving the terminal. Uses the
Dependency-Track binding already configured on the Portal
(DEPENDENCY_TRACK_URL / DEPENDENCY_TRACK_API_KEY).`,
	}

	cmd.AddCommand(
		list.NewCmdList(f, nil),
		get.NewCmdGet(f, nil),
		components.NewCmdComponents(f, nil),
		findings.NewCmdFindings(f, nil),
	)

	return cmd
}
