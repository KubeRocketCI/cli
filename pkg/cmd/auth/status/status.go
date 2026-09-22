// Package status implements the "krci auth status" command.
package status

import (
	"context"
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
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// SchemaVersion is the JSON envelope version emitted by `krci auth status -o json`.
const SchemaVersion = "1"

// StatusOptions holds all inputs for the status command.
type StatusOptions struct {
	IO            *iostreams.IOStreams
	TokenProvider func() (auth.TokenProvider, error)
	RestClient    func() (*restapi.ClientWithResponses, error)
	OutputFormat  string
}

// StatusPayload is the envelope `data` block for `krci auth status -o json`.
type StatusPayload struct {
	Authenticated bool     `json:"authenticated"`
	User          string   `json:"user,omitempty"`
	Name          string   `json:"name,omitempty"`
	Groups        []string `json:"groups"`
	ExpiresAt     *string  `json:"expiresAt"`
}

// sessionError carries the user-facing message for a failed session check
// while errors.Is still matches the sentinel that caused it.
type sessionError struct {
	msg   string
	cause error
}

func (e *sessionError) Error() string { return e.msg }
func (e *sessionError) Unwrap() error { return e.cause }

// NewCmdStatus returns the "auth status" cobra.Command.
// runF is the business logic function; pass nil to use the default statusRun.
func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:            f.IOStreams,
		TokenProvider: f.TokenProvider,
		RestClient:    f.RestClient,
	}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long: `Show the signed-in user, the session expiry, and the user's groups.

Exits 1 when no session is stored or the session has expired, so the
command works as a shell guard. KRCI_TOKEN, and a stored token whose claims
cannot be read, are checked with one portal call and exit 1 when the portal
rejects the token or cannot be reached. With -o json the outcome is a schemaVersion
envelope: data.authenticated, data.user, data.name, data.groups, and
data.expiresAt on success; error.message on failure.`,
		Example: `  krci auth status

  # Shell guard
  krci auth status >/dev/null 2>&1 || krci auth login

  # JSON (for scripts and AI agents)
  krci auth status -o json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateOutputFormat(opts.OutputFormat); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return statusRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "", "Output format: table, json (default: table)")

	return cmd
}

func validateOutputFormat(format string) error {
	switch format {
	case "", output.FormatTable, output.FormatJSON:
		return nil
	default:
		return fmt.Errorf("unknown output format: %s (use 'json' or 'table')", format)
	}
}

func statusRun(ctx context.Context, opts *StatusOptions) error {
	tp, err := opts.TokenProvider()
	if err != nil {
		return err
	}

	// GetToken classifies the auth state (not-authenticated, expired,
	// refresh-failed, or valid) and refreshes and persists an expired token.
	if _, tokenErr := tp.GetToken(ctx); tokenErr != nil {
		return opts.fail(classifyTokenError(tokenErr))
	}

	info, infoErr := tp.UserInfo()

	// The stored session comes from this CLI's own login and is judged
	// locally. KRCI_TOKEN, or claims that cannot be read, prove nothing
	// locally: the portal decides.
	if infoErr == nil && !info.FromEnv {
		return opts.render(info, true)
	}

	if err := opts.verifySession(ctx); err != nil {
		return opts.fail(err)
	}

	if infoErr != nil {
		return opts.render(&auth.UserInfo{}, false)
	}

	return opts.render(info, true)
}

// verifySession asks the portal to validate the token through the same
// client every portal command uses, and returns the error the command exits
// with when the portal rejects the token or cannot be asked.
func (opts *StatusOptions) verifySession(ctx context.Context) error {
	client, err := opts.RestClient()
	if err != nil {
		return err
	}

	if err := portal.VerifySession(ctx, client); err != nil {
		if errors.Is(err, portal.ErrUnauthorized) {
			return &sessionError{msg: "not authenticated: the portal rejected the token", cause: err}
		}

		return fmt.Errorf("verifying the token with the portal: %w", err)
	}

	return nil
}

// classifyTokenError turns the token provider's failure into the error the
// command exits with. A missing session keeps the auth sentinel message; an
// expired or unrefreshable session gets a message of its own.
func classifyTokenError(err error) error {
	if errors.Is(err, auth.ErrTokenExpired) || errors.Is(err, auth.ErrRefreshFailed) {
		return &sessionError{msg: "session expired: run 'krci auth login'", cause: err}
	}

	return err
}

// fail writes the error envelope to stdout under -o json, so scripting
// consumers get a structured error next to the exit-1 signal, then returns
// err for the root command to print and exit on.
func (opts *StatusOptions) fail(err error) error {
	if output.ResolveFormat(opts.OutputFormat) == output.FormatJSON {
		_ = output.PrintJSONErrorEnvelope(opts.IO.Out, SchemaVersion, err)
	}

	return err
}

// render prints the authenticated state. haveInfo is false when the token's
// claims could not be read and the portal accepted the token.
func (opts *StatusOptions) render(info *auth.UserInfo, haveInfo bool) error {
	if output.ResolveFormat(opts.OutputFormat) == output.FormatJSON {
		return output.PrintJSONEnvelope(opts.IO.Out, SchemaVersion, buildPayload(info))
	}

	if !haveInfo {
		_, err := fmt.Fprintln(opts.IO.Out, "Status:  Authenticated (unable to read user info)")

		return err
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

// buildPayload shapes the JSON data block. Groups is always an array and
// expiresAt is RFC3339 in UTC, null when the token carries no expiry.
func buildPayload(info *auth.UserInfo) StatusPayload {
	p := StatusPayload{
		Authenticated: true,
		User:          info.Email,
		Name:          info.Name,
		Groups:        []string{},
	}

	if len(info.Groups) > 0 {
		p.Groups = info.Groups
	}

	if !info.ExpiresAt.IsZero() {
		s := info.ExpiresAt.UTC().Format(time.RFC3339)
		p.ExpiresAt = &s
	}

	return p
}
