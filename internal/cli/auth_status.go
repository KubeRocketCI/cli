package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/auth"
)

func (a *App) newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := a.tokenProvider.GetToken(cmd.Context())

			info, infoErr := a.tokenProvider.UserInfo()

			if err != nil {
				if errors.Is(err, auth.ErrNotAuthenticated) {
					fmt.Fprintln(os.Stderr, "Not authenticated. Run: krci auth login")
					return nil
				}
				if errors.Is(err, auth.ErrRefreshFailed) || errors.Is(err, auth.ErrTokenExpired) {
					if infoErr == nil {
						fmt.Fprintf(os.Stderr, "User:    %s\n", info.Email)
					}
					fmt.Fprintln(os.Stderr, "Status:  Session expired. Run: krci auth login")
					return nil
				}
				return err
			}

			if infoErr != nil {
				fmt.Fprintln(os.Stderr, "Status:  Authenticated (unable to read user info)")
				return nil
			}

			fmt.Fprintf(os.Stderr, "User:    %s\n", info.Email)
			if info.Name != "" {
				fmt.Fprintf(os.Stderr, "Name:    %s\n", info.Name)
			}
			fmt.Fprintf(os.Stderr, "Status:  Authenticated\n")

			if expiry := info.ExpiresAt; !expiry.IsZero() {
				remaining := time.Until(expiry).Round(time.Second)
				fmt.Fprintf(os.Stderr, "Expires: %s (%s)\n", expiry.Local().Format(time.RFC822), remaining)
			}

			if len(info.Groups) > 0 {
				fmt.Fprintf(os.Stderr, "Groups:  %s\n", strings.Join(info.Groups, ", "))
			}

			return nil
		},
	}
}
