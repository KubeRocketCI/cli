// Package versions implements the "krci project versions <project>" command.
package versions

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
)

// ListOptions holds all inputs for `krci project versions <project>`.
type ListOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Project      string
	Branch       string
	OutputFormat string
}

// NewCmdVersions returns the "project versions <project>" cobra.Command.
// runF is the business-logic function; pass nil to use the default run.
func NewCmdVersions(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "versions <project>",
		Short: "List the image versions a project has built, per branch",
		Long: `List the image versions the build pipeline has pushed for a project: one
row per branch with the image repository, the number of versions, and the
newest one. These are the versions a deployment can pick.

A version appears here once its build pipeline has finished; the list is
read from the project's CodebaseImageStream resources, so a branch that has
never been built shows 0 versions.

--branch narrows the output to one git branch and lists every version of it,
newest first.`,
		Args: cmdutil.ExactArgs(1, "a project name",
			"to see available projects: krci project list"),
		Example: `  # One row per branch
  krci project versions my-app

  # Every version of one branch, newest first
  krci project versions my-app --branch release/1.2

  # JSON envelope
  krci project versions my-app -o json

  # Scripting — the newest version of the main branch
  krci project versions my-app --branch main -o json |
    jq -r '.data.streams[0].versions[0].name'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Project = args[0]

			if err := discovery.ValidateOutputFormat(opts.OutputFormat); err != nil {
				return err
			}

			if err := cmdutil.ValidateK8sName("<project>", opts.Project); err != nil {
				return err
			}

			if len(opts.Branch) > portal.MaxBranchLength {
				return fmt.Errorf("--branch must be at most %d characters", portal.MaxBranchLength)
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Branch, "branch", "",
		"Git branch to list every version of (default: one row per branch)")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	client, err := opts.RestClient()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewProjectVersionsService(client, cfg.ClusterName, cfg.Namespace)

	streams, err := svc.List(ctx, opts.Project, opts.Branch)
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	payload := portal.ProjectVersionsPayload{
		Project: opts.Project,
		Streams: streams,
	}

	if err := discovery.Render(opts.IO, opts.OutputFormat, payload, func(w io.Writer, isTTY bool) error {
		if opts.Branch != "" {
			return renderVersionsTable(w, isTTY, streams)
		}

		return renderStreamsTable(w, isTTY, streams)
	}); err != nil {
		return err
	}

	if note := emptyNote(opts.Project, opts.Branch, streams); note != "" && opts.OutputFormat != output.FormatJSON {
		if _, err := fmt.Fprintln(opts.IO.ErrOut, note); err != nil {
			return err
		}
	}

	return nil
}

// emptyNote is the stderr line for a table view with nothing to show: no
// stream at all, or, with --branch, a branch that exists but has never
// produced a version. Empty when the table has rows.
func emptyNote(project, branch string, streams []portal.ProjectVersionStream) string {
	if branch == "" {
		if len(streams) == 0 {
			return fmt.Sprintf("No versions found for project %s.", project)
		}

		return ""
	}

	for _, s := range streams {
		if len(s.Versions) > 0 {
			return ""
		}
	}

	return fmt.Sprintf("No versions found for branch %s of project %s.", branch, project)
}

// renderStreamsTable prints one row per branch with its newest version.
func renderStreamsTable(w io.Writer, isTTY bool, streams []portal.ProjectVersionStream) error {
	headers := []string{"BRANCH", "VERSIONS", "LATEST", "CREATED", "IMAGE"}
	rows := make([][]string, 0, len(streams))

	for _, s := range streams {
		latest, created := output.EmptyCell, output.EmptyCell
		if len(s.Versions) > 0 {
			latest = output.OrDash(s.Versions[0].Name)
			created = output.OrDash(s.Versions[0].Created)
		}

		rows = append(rows, []string{
			s.Branch,
			strconv.Itoa(len(s.Versions)),
			latest,
			created,
			output.OrDash(s.Image),
		})
	}

	return discovery.PrintTable(w, isTTY, headers, rows)
}

// renderVersionsTable prints one row per version, newest first, for the
// --branch view.
func renderVersionsTable(w io.Writer, isTTY bool, streams []portal.ProjectVersionStream) error {
	headers := []string{"VERSION", "CREATED", "DIGEST", "IMAGE"}

	var rows [][]string

	for _, s := range streams {
		for _, v := range s.Versions {
			digest := v.Digest
			rows = append(rows, []string{
				output.OrDash(v.Name),
				output.OrDash(v.Created),
				discovery.ShortDigestCell(&digest),
				output.OrDash(s.Image),
			})
		}
	}

	return discovery.PrintTable(w, isTTY, headers, rows)
}
