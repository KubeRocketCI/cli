// Package status implements the "krci auth status" command.
package status

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
)

// StatusOptions holds all inputs for the status command.
type StatusOptions struct {
	IO            *iostreams.IOStreams
	TokenProvider func() (auth.TokenProvider, error)
}

// NewCmdStatus returns the "auth status" cobra.Command.
// runF is the business logic function; pass nil to use the default statusRun.
func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:            f.IOStreams,
		TokenProvider: f.TokenProvider,
	}

	return &cobra.Command{
		Use:     "status",
		Short:   "Show authentication status",
		Example: "  krci auth status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}

			return statusRun(cmd, opts)
		},
	}
}

func statusRun(cmd *cobra.Command, opts *StatusOptions) error {
	tp, err := opts.TokenProvider()
	if err != nil {
		return err
	}

	// GetToken is called for its error (to classify auth state: not-authenticated,
	// expired, refresh-failed, or valid) and its side-effect (refreshing and
	// persisting the token if expired). The token value itself is not needed.
	_, tokenErr := tp.GetToken(cmd.Context())

	info, infoErr := tp.UserInfo()

	if tokenErr != nil {
		if errors.Is(tokenErr, auth.ErrNotAuthenticated) {
			_, _ = fmt.Fprintln(opts.IO.ErrOut, "Not authenticated. Run: krci auth login")
			return nil
		}

		if errors.Is(tokenErr, auth.ErrRefreshFailed) || errors.Is(tokenErr, auth.ErrTokenExpired) {
			if infoErr == nil {
				_, _ = fmt.Fprintf(opts.IO.ErrOut, "User:    %s\n", info.Email)
			}

			_, _ = fmt.Fprintln(opts.IO.ErrOut, "Status:  Session expired. Run: krci auth login")

			return nil
		}

		return tokenErr
	}

	if infoErr != nil {
		_, _ = fmt.Fprintln(opts.IO.Out, "Status:  Authenticated (unable to read user info)")
		return nil
	}

	lines := []output.DetailLine{
		{Label: "User", Value: info.Email},
	}

	if info.Name != "" {
		lines = append(lines, output.DetailLine{Label: "Name", Value: info.Name})
	}

	lines = append(lines, output.DetailLine{Label: "Status", Value: "Authenticated"})

	if expiry := info.ExpiresAt; !expiry.IsZero() {
		remaining := time.Until(expiry).Round(time.Second)
		expiresVal := fmt.Sprintf("%s (%s)", expiry.Local().Format(time.RFC822), remaining)
		lines = append(lines, output.DetailLine{Label: "Expires", Value: expiresVal})
	}

	if len(info.Groups) > 0 {
		lines = append(lines, output.DetailLine{Label: "Groups", Value: strings.Join(info.Groups, ", ")})
	}

	return output.RenderDetail(opts.IO, "", lines, output.DetailRenderer[[]output.DetailLine]{
		Styled: func(w io.Writer, ls []output.DetailLine) error {
			for i, l := range ls {
				switch l.Label {
				case "Status":
					ls[i].Styled = output.GreenText(l.Value)
				case "Expires":
					remaining := time.Until(info.ExpiresAt).Round(time.Second)
					if remaining < 5*time.Minute {
						ls[i].Styled = output.YellowText(l.Value)
					}
				}
			}
			return output.PrintStyledDetailLines(w, ls)
		},
		Plain: output.PrintPlainDetailLines,
	})
}
