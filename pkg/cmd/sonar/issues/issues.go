// Package issues implements the "krci sonar issues" command.
package issues

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	sonarinternal "github.com/KubeRocketCI/cli/pkg/cmd/sonar/internal"
)

// Default / bound values mirror the shared Zod schema.
const (
	defaultPageSize  = 25
	defaultPage      = 1
	maxMessageLength = 80
)

// IssuesOptions holds all inputs for `krci sonar issues <project>`.
type IssuesOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Project      string
	PullRequest  string
	Branch       string
	Types        []string
	Severities   []string
	Statuses     []string
	Resolved     bool
	Sort         string
	Asc          bool
	Page         int
	PageSize     int
	OutputFormat string

	// normalized is the post-validation portal payload. Populated in RunE
	// after CSV + enum normalization so runF test hooks see the exact values
	// that would be sent upstream (instead of raw []string slices).
	normalized portal.SonarIssuesParams
}

// Normalized returns the post-validation portal payload. Intended for tests.
func (o *IssuesOptions) Normalized() portal.SonarIssuesParams { return o.normalized }

// NewCmdIssues returns the "sonar issues" cobra.Command.
func NewCmdIssues(f *cmdutil.Factory, runF func(*IssuesOptions) error) *cobra.Command {
	opts := &IssuesOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
	}

	cmd := &cobra.Command{
		Use:   "issues <project>",
		Short: "List SonarQube issues for a project",
		Args: cmdutil.ExactArgs(1, "a SonarQube project key",
			"to see available projects: krci sonar list"),
		Example: `  # First 25 unresolved issues
  krci sonar issues payments-api

  # Blockers + criticals only
  krci sonar issues payments-api --severity BLOCKER,CRITICAL

  # Bugs + vulnerabilities, include resolved, sort by severity ascending
  krci sonar issues payments-api --type BUG,VULNERABILITY --resolved --sort SEVERITY --asc

  # Pull-request scope
  krci sonar issues payments-api --pr 123 --severity BLOCKER

  # Branch scope (mirrors Sonar's ?branch= URL param)
  krci sonar issues payments-api --branch main --severity BLOCKER`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Project = args[0]

			if err := sonarinternal.ValidateProjectCommand(
				cmd, opts.OutputFormat, opts.Project, opts.PullRequest, opts.Branch,
			); err != nil {
				return err
			}

			if opts.PageSize < 1 || opts.PageSize > sonarinternal.MaxPageSize {
				return fmt.Errorf("--page-size must be between 1 and %d", sonarinternal.MaxPageSize)
			}

			if opts.Page < 1 {
				return fmt.Errorf("--page must be >= 1")
			}

			types, err := sonarinternal.ValidateEnumCSV("type", opts.Types, sonarinternal.IssueTypes)
			if err != nil {
				return err
			}

			severities, err := sonarinternal.ValidateEnumCSV("severity", opts.Severities, sonarinternal.IssueSeverities)
			if err != nil {
				return err
			}

			statuses, err := sonarinternal.ValidateEnumCSV("status", opts.Statuses, sonarinternal.IssueStatuses)
			if err != nil {
				return err
			}

			p := portal.SonarIssuesParams{
				ProjectKey:  opts.Project,
				PullRequest: opts.PullRequest,
				Branch:      opts.Branch,
				Types:       types,
				Severities:  severities,
				Statuses:    statuses,
				Sort:        opts.Sort,
				Page:        opts.Page,
				PageSize:    opts.PageSize,
			}

			// Only forward --resolved / --asc when the user explicitly set
			// them. Both flags have Sonar-side defaults; appending them
			// uninvited would silently override server behavior.
			if cmd.Flags().Changed("resolved") {
				p.Resolved = &opts.Resolved
			}

			if cmd.Flags().Changed("asc") {
				p.Asc = &opts.Asc
			}

			opts.normalized = p

			if runF != nil {
				return runF(opts)
			}

			return issuesRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.PullRequest, "pr", "",
		"SonarQube pull-request id (mutually exclusive with --branch)")
	cmd.Flags().StringVar(&opts.Branch, "branch", "",
		"SonarQube branch name (mutually exclusive with --pr)")
	cmd.Flags().StringSliceVar(&opts.Types, "type", nil,
		"Issue types (csv or repeatable): BUG,VULNERABILITY,CODE_SMELL")
	cmd.Flags().StringSliceVar(&opts.Severities, "severity", nil,
		"Severity filter (csv or repeatable): BLOCKER,CRITICAL,MAJOR,MINOR,INFO")
	cmd.Flags().StringSliceVar(&opts.Statuses, "status", nil,
		"Status filter (csv or repeatable): OPEN,CONFIRMED,REOPENED,RESOLVED,CLOSED")
	cmd.Flags().BoolVar(&opts.Resolved, "resolved", false, "Include resolved issues")
	cmd.Flags().StringVar(&opts.Sort, "sort", "", "Sort field (forwarded to SonarQube)")
	cmd.Flags().BoolVar(&opts.Asc, "asc", false, "Sort ascending")
	cmd.Flags().IntVar(&opts.Page, "page", defaultPage, "Page index (1-based)")
	cmd.Flags().IntVar(&opts.PageSize, "page-size", defaultPageSize, "Page size (max 500)")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func issuesRun(ctx context.Context, opts *IssuesOptions) error {
	client, err := opts.RestClient()
	if err != nil {
		return sonarinternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewSonarService(client)

	result, err := svc.Issues(ctx, opts.normalized)
	if err != nil {
		return sonarinternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	return sonarinternal.Render(opts.IO, opts.OutputFormat, result, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, opts.Project, opts.PullRequest, opts.Branch, result)
	})
}

func renderTable(w io.Writer, isTTY bool, project, pullRequest, branch string, result *portal.SonarIssueList) error {
	if _, err := fmt.Fprintf(
		w,
		"Issues for %s  (page %d/%d, %d total)\n\n",
		project,
		result.Paging.PageIndex,
		sonarinternal.PageCount(result.Paging.Total, result.Paging.PageSize),
		result.Paging.Total,
	); err != nil {
		return err
	}

	// Zero-result scoped queries are ambiguous (e.g. clean PR/branch,
	// unmatched filters, or a scope that is not analyzed upstream), so print
	// a non-assertive hint naming the scope the user supplied.
	if result.Paging.Total == 0 {
		switch {
		case pullRequest != "":
			_, err := fmt.Fprintf(
				w,
				"(no issues — verify the pull-request id %q and applied filters)\n",
				pullRequest,
			)
			return err
		case branch != "":
			_, err := fmt.Fprintf(
				w,
				"(no issues — verify the branch name %q and applied filters)\n",
				branch,
			)
			return err
		}
	}

	headers := []string{"KEY", "SEVERITY", "TYPE", "STATUS", "RULE", "COMPONENT", "LINE", "MESSAGE"}
	rows := make([][]string, 0, len(result.Issues))

	for _, i := range result.Issues {
		line := ""
		if i.Line > 0 {
			line = strconv.Itoa(i.Line)
		}

		// MESSAGE truncated for TTY only; piped output keeps the full text
		// so downstream tools can grep on it.
		msg := i.Message
		if isTTY {
			msg = output.Truncate(i.Message, maxMessageLength)
		}

		rows = append(rows, []string{
			i.Key,
			i.Severity,
			i.Type,
			i.Status,
			i.Rule,
			i.Component,
			line,
			msg,
		})
	}

	return sonarinternal.PrintTable(w, isTTY, headers, rows)
}
