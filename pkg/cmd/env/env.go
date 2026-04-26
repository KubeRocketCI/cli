// Package env implements the "krci env" command group.
package env

import (
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/env/get"
	"github.com/KubeRocketCI/cli/pkg/cmd/env/list"
)

// NewCmdEnv returns the "env" group cobra.Command with all subcommands attached.
func NewCmdEnv(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "env",
		Aliases: []string{"e"},
		Short:   "Inspect KRCI environments (Stages)",
		Long: `Inspect KRCI environments — the Stage resources surfaced as
"environments" in the Portal — without leaving the terminal. Lists every
stage in the configured namespace or shows full detail for one (deployment,
env) pair, including infrastructure, quality gates, and the projects
deployed there with health, sync, version, image, and ingress URLs.`,
	}

	cmd.AddCommand(
		list.NewCmdList(f, nil),
		get.NewCmdGet(f, nil),
	)

	return cmd
}
