// Package sonar implements the "krci sonar" command group.
package sonar

import (
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/sonar/gate"
	"github.com/KubeRocketCI/cli/pkg/cmd/sonar/get"
	"github.com/KubeRocketCI/cli/pkg/cmd/sonar/issues"
	"github.com/KubeRocketCI/cli/pkg/cmd/sonar/list"
)

// NewCmdSonar returns the "sonar" group cobra.Command with all subcommands attached.
func NewCmdSonar(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sonar",
		Short: "Inspect SonarQube projects, quality gates, and issues",
		Long: `Inspect SonarQube state — projects, measures, quality gate, and issues —
without leaving the terminal. Uses the SonarQube binding already configured
on the Portal (SONAR_HOST_URL / SONAR_TOKEN).`,
	}

	cmd.AddCommand(
		list.NewCmdList(f, nil),
		get.NewCmdGet(f, nil),
		gate.NewCmdGate(f, nil),
		issues.NewCmdIssues(f, nil),
	)

	return cmd
}
