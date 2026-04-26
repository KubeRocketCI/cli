// Package root assembles the top-level cobra.Command for the krci CLI.
package root

import (
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
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmdutil.ValidateStringFlags(cmd); err != nil {
				return err
			}

			// Warm the config cache after Cobra has parsed all flags.
			// Subcommand RunE functions receive the cached result instantly.
			_, err := f.Config()
			return err
		},
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

	return cmd
}
