// Package list implements the "krci sonar list" command.
package list

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	sonarinternal "github.com/KubeRocketCI/cli/pkg/cmd/sonar/internal"
)

// Default SonarQube page size — 50 matches `/api/components/search`'s default.
const defaultPageSize = 50

// ListOptions holds all inputs for `krci sonar list`.
type ListOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Page         int
	PageSize     int
	Search       string
	OutputFormat string
}

// NewCmdList returns the "sonar list" cobra.Command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List SonarQube projects",
		Long:  "List SonarQube projects with their quality-gate status.",
		Args:  cobra.NoArgs,
		Example: `  # First page (default 50)
  krci sonar list

  # Paginate + search
  krci sonar list --search payments --page 2 --page-size 25

  # Scripting — failing-gate projects
  krci sonar list -o json | jq -r '.data.projects[] | select(.qualityGateStatus=="ERROR") | .key'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := sonarinternal.ValidateOutputFormat(opts.OutputFormat); err != nil {
				return err
			}

			if opts.PageSize < 1 || opts.PageSize > sonarinternal.MaxPageSize {
				return fmt.Errorf("--page-size must be between 1 and %d", sonarinternal.MaxPageSize)
			}

			if opts.Page < 1 {
				return fmt.Errorf("--page must be >= 1")
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().IntVar(&opts.Page, "page", 1, "Page index (1-based)")
	cmd.Flags().IntVar(&opts.PageSize, "page-size", defaultPageSize, "Page size (max 500)")
	cmd.Flags().StringVar(&opts.Search, "search", "", "Filter projects by partial name")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	client, err := opts.RestClient()
	if err != nil {
		return sonarinternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewSonarService(client)

	result, err := svc.List(ctx, portal.SonarListParams{
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		SearchTerm: opts.Search,
	})
	if err != nil {
		return sonarinternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	return sonarinternal.Render(opts.IO, opts.OutputFormat, result, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, result)
	})
}

func renderTable(w io.Writer, isTTY bool, result *portal.SonarProjectList) error {
	headers := []string{"KEY", "NAME", "GATE"}
	rows := make([][]string, 0, len(result.Projects))

	for _, p := range result.Projects {
		status := p.QualityGateStatus
		if status == "" {
			status = portal.QualityGateNone
		}

		gate := string(status)
		if isTTY {
			gate = output.SonarGateStatusColor(status)
		}

		rows = append(rows, []string{p.Key, p.Name, gate})
	}

	if err := sonarinternal.PrintTable(w, isTTY, headers, rows); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	_, err := fmt.Fprintln(w, pageFooter(result.Paging))

	return err
}

// pageFooter formats the paging summary printed below the table. It prefers
// clarity over raw arithmetic: a request beyond the last page reads
// "beyond last page" rather than the misleading "page 999 of 1", and a
// single-result page reads "1 project" (not "1 projects").
func pageFooter(paging portal.SonarPaging) string {
	totalPages := sonarinternal.PageCount(paging.Total, paging.PageSize)

	if paging.Total == 0 {
		return fmt.Sprintf("no projects (page-size %d)", paging.PageSize)
	}

	projectWord := "projects"
	if paging.Total == 1 {
		projectWord = "project"
	}

	if paging.PageIndex > totalPages {
		return fmt.Sprintf(
			"%d %s — page %d is beyond last page (%d of %d) (page-size %d)",
			paging.Total,
			projectWord,
			paging.PageIndex,
			totalPages,
			totalPages,
			paging.PageSize,
		)
	}

	return fmt.Sprintf(
		"%d %s, page %d of %d (page-size %d)",
		paging.Total,
		projectWord,
		paging.PageIndex,
		totalPages,
		paging.PageSize,
	)
}
