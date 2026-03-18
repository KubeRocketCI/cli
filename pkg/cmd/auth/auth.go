// Package auth implements the "krci auth" command group.
package auth

import (
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/auth/login"
	"github.com/KubeRocketCI/cli/pkg/cmd/auth/logout"
	"github.com/KubeRocketCI/cli/pkg/cmd/auth/status"
)

// NewCmdAuth returns the "auth" group cobra.Command with all subcommands attached.
func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
	}

	cmd.AddCommand(
		login.NewCmdLogin(f, nil),
		status.NewCmdStatus(f, nil),
		logout.NewCmdLogout(f, nil),
	)

	return cmd
}
