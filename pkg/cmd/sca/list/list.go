// Package list implements the "krci sca list" command.
package list

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	scainternal "github.com/KubeRocketCI/cli/pkg/cmd/sca/internal"
)

// Default Dep-Track page size — matches Portal UI for projects.
const defaultListPageSize = 25

// ListOptions holds all inputs for `krci sca list`.
type ListOptions struct {
	IO              *iostreams.IOStreams
	RestClient      func() (*restapi.ClientWithResponses, error)
	Page            int
	PageSize        int
	Search          string
	IncludeInactive bool
	IncludeChildren bool
	OutputFormat    string
}

// NewCmdList returns the "sca list" cobra.Command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Dependency-Track SCA projects",
		Long: `List Software Composition Analysis projects known to Dependency-Track.
By default excludes inactive projects and child versions.`,
		Args: cobra.NoArgs,
		Example: `  # First page (default 25), active root projects only
  krci sca list

  # Paginate + search
  krci sca list --search payments --page 2 --page-size 50

  # Include inactive and child (non-root) projects
  krci sca list --include-inactive --include-children

  # Scripting — every project with at least one CRITICAL finding
  krci sca list -o json | jq -r '.data.items[] | select(.metrics.critical>0) | .name'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := scainternal.ValidateOutputFormat(opts.OutputFormat); err != nil {
				return err
			}

			if err := scainternal.ValidatePageBounds(opts.Page, opts.PageSize); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().IntVar(&opts.Page, "page", 1, "Page index (1-based)")
	cmd.Flags().IntVar(&opts.PageSize, "page-size", defaultListPageSize, "Page size (max 500)")
	cmd.Flags().StringVar(&opts.Search, "search", "", "Filter projects by partial name / version")
	cmd.Flags().BoolVar(&opts.IncludeInactive, "include-inactive", false, "Include inactive projects")
	cmd.Flags().BoolVar(&opts.IncludeChildren, "include-children", false, "Include child (non-root) versions")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	client, err := opts.RestClient()
	if err != nil {
		return scainternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	result, err := portal.NewSCAService(client).List(ctx, portal.SCAListParams{
		Page:            opts.Page,
		PageSize:        opts.PageSize,
		Search:          opts.Search,
		IncludeInactive: opts.IncludeInactive,
		IncludeChildren: opts.IncludeChildren,
	})
	if err != nil {
		return scainternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	return scainternal.Render(opts.IO, opts.OutputFormat, result, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, opts.Page, opts.PageSize, result)
	})
}

func renderTable(w io.Writer, isTTY bool, page, pageSize int, result *portal.SCAProjectList) error {
	headers := []string{"NAME", "VERSION", "CLASSIFIER", "ACTIVE", "LAST_BOM", "RISK", "VULNS (C/H/M/L)"}
	rows := make([][]string, 0, len(result.Items))

	for _, p := range result.Items {
		rows = append(rows, []string{
			p.Name,
			p.Version,
			p.Classifier,
			scainternal.BoolYesNo(p.Active),
			formatLastBom(p.LastBomImport),
			scainternal.FormatRisk(p.RiskScore),
			scainternal.FormatVulnCounts(p.Metrics, isTTY),
		})
	}

	if err := scainternal.PrintTable(w, isTTY, headers, rows); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	_, err := fmt.Fprintln(w, scainternal.PageFooter("project", result.TotalCount, page, pageSize))
	return err
}

func formatLastBom(ms int64) string {
	if ms <= 0 {
		return scainternal.EmptyPlaceholder
	}
	return output.RelativeTime(time.UnixMilli(ms))
}
