// Package root assembles the top-level cobra.Command for the krci CLI.
package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/pkg/cmd/auth"
	"github.com/KubeRocketCI/cli/pkg/cmd/deployment"
	"github.com/KubeRocketCI/cli/pkg/cmd/env"
	"github.com/KubeRocketCI/cli/pkg/cmd/pipelinerun"
	"github.com/KubeRocketCI/cli/pkg/cmd/project"
	"github.com/KubeRocketCI/cli/pkg/cmd/sca"
	"github.com/KubeRocketCI/cli/pkg/cmd/sonar"
	"github.com/KubeRocketCI/cli/pkg/cmd/version"
)

// NewCmdRoot builds the root cobra.Command with all subcommands attached.
// version, commit, and date are injected from ldflags at build time.
func NewCmdRoot(f *cmdutil.Factory, v, commit, date string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "krci",
		Short:         "KubeRocketCI CLI",
		Long:          "Command-line interface for the KubeRocketCI platform.",
		Version:       v,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	config.BindFlags(cmd)

	cmd.AddCommand(
		auth.NewCmdAuth(f),
		project.NewCmdProject(f),
		deployment.NewCmdDeployment(f),
		env.NewCmdEnv(f),
		pipelinerun.NewCmdPipelineRun(f),
		sca.NewCmdSca(f),
		sonar.NewCmdSonar(f),
		version.NewCmdVersion(f.IOStreams, v, commit, date),
	)

	// Cobra adds help and completion inside Execute; add them now so
	// FinalizeTree covers them.
	cmd.InitDefaultHelpCmd()
	redirectUnknownHelpTopic(cmd)
	cmd.InitDefaultCompletionCmd()

	cmdutil.FinalizeTree(cmd)

	return cmd
}

// Cobra writes an unknown help topic through OutOrStderr, which SetOut redirects to stdout; keep it on stderr.
func redirectUnknownHelpTopic(cmd *cobra.Command) {
	help, _, _ := cmd.Find([]string{"help"})
	defaultRun := help.Run

	help.Run = func(c *cobra.Command, args []string) {
		if target, _, err := c.Root().Find(args); target == nil || err != nil {
			_, _ = fmt.Fprintf(c.ErrOrStderr(), "Unknown help topic %#q\n", args)
			_, _ = fmt.Fprint(c.ErrOrStderr(), c.Root().UsageString())

			return
		}

		defaultRun(c, args)
	}
}
