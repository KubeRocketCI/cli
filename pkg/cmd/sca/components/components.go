// Package components implements the "krci sca components" command.
package components

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	scainternal "github.com/KubeRocketCI/cli/pkg/cmd/sca/internal"
)

const defaultComponentsPageSize = 50

// ComponentsOptions holds all inputs for `krci sca components <codebase>`.
type ComponentsOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Codebase     string
	Branch       string
	Severity     []string
	OnlyOutdated bool
	OnlyDirect   bool
	Page         int
	PageSize     int
	OutputFormat string
}

// NewCmdComponents returns the "sca components" cobra.Command.
func NewCmdComponents(f *cmdutil.Factory, runF func(*ComponentsOptions) error) *cobra.Command {
	opts := &ComponentsOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
	}

	cmd := &cobra.Command{
		Use:   "components <codebase>",
		Short: "List dependencies (components) for an SCA project",
		Args: cmdutil.ExactArgs(1, "a KubeRocketCI codebase name",
			"to see available projects: krci sca list"),
		Example: `  # Default branch, first 50 dependencies
  krci sca components payments-api

  # Outdated, direct dependencies only
  krci sca components payments-api --only-outdated --only-direct

  # Dependencies with at least one HIGH (or CRITICAL) finding, explicit branch
  krci sca components payments-api --branch=release/1.0 --severity=high

  # Scripting — package names for the default branch
  krci sca components payments-api -o json | jq -r '.data.items[].name'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Codebase = args[0]

			if err := scainternal.ValidateCodebaseCommand(
				cmd, opts.OutputFormat, opts.Codebase, opts.Branch,
			); err != nil {
				return err
			}

			if err := scainternal.ValidatePageBounds(opts.Page, opts.PageSize); err != nil {
				return err
			}

			if _, err := scainternal.ValidateSeverityCSV(opts.Severity); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return componentsRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Branch, "branch", "", scainternal.BranchFlagUsage)
	cmd.Flags().StringSliceVar(&opts.Severity, "severity", nil,
		scainternal.SeverityFlagUsage+
			" Applied client-side to the fetched page only; "+
			"increase --page-size to examine more components.")
	cmd.Flags().BoolVar(&opts.OnlyOutdated, "only-outdated", false, "Only components marked outdated by Dependency-Track")
	cmd.Flags().BoolVar(&opts.OnlyDirect, "only-direct", false, "Only direct (non-transitive) dependencies")
	cmd.Flags().IntVar(&opts.Page, "page", 1, "Page index (1-based)")
	cmd.Flags().IntVar(&opts.PageSize, "page-size", defaultComponentsPageSize, "Page size (max 500)")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func componentsRun(ctx context.Context, opts *ComponentsOptions) error {
	client, err := opts.RestClient()
	if err != nil {
		return scainternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	result, err := portal.NewSCAService(client).Components(ctx, portal.SCAComponentsParams{
		Codebase:     opts.Codebase,
		Branch:       opts.Branch,
		Page:         opts.Page,
		PageSize:     opts.PageSize,
		OnlyOutdated: opts.OnlyOutdated,
		OnlyDirect:   opts.OnlyDirect,
	})
	if err != nil {
		return scainternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	if inclusive := scainternal.ExpandSeverityFlag(opts.Severity); len(inclusive) > 0 {
		filtered := applySeverityFilter(result.Items, inclusive)
		result.Items = filtered
		// The server returns a TotalCount across the unfiltered page; once we
		// narrow client-side, both the JSON envelope and the table footer must
		// reflect the visible row count or the "page X of Y" line lies.
		result.TotalCount = len(filtered)
	}

	return scainternal.Render(opts.IO, opts.OutputFormat, result, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, opts.Codebase, opts.Page, opts.PageSize, result)
	})
}

// applySeverityFilter keeps only components whose metrics contain at least
// one vulnerability in the allowed severity set. Empty allowed means no
// filter. Client-side because Dep-Track's /component/project endpoint does
// not filter by severity.
func applySeverityFilter(items []portal.SCAComponent, allowed []string) []portal.SCAComponent {
	if len(allowed) == 0 {
		return items
	}
	out := make([]portal.SCAComponent, 0, len(items))
	for _, c := range items {
		if c.Metrics == nil {
			continue
		}
		if scainternal.ComponentMatchesSeverity(func(sev string) int {
			return metricsCountFor(c.Metrics, sev)
		}, allowed) {
			out = append(out, c)
		}
	}
	return out
}

func metricsCountFor(m *portal.SCAMetrics, severity string) int {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return m.Critical
	case "HIGH":
		return m.High
	case "MEDIUM":
		return m.Medium
	case "LOW":
		return m.Low
	case "INFO", "UNASSIGNED":
		// Dep-Track folds these together in its component metrics.
		return m.Unassigned
	}
	return 0
}

func renderTable(
	w io.Writer, isTTY bool, codebase string, page, pageSize int, result *portal.SCAComponentList,
) error {
	if result.Status == portal.SCAStatusNone {
		_, err := fmt.Fprintf(w, "status: NONE — no SCA scanner bound for %s\n", codebase)
		return err
	}

	headers := []string{"COMPONENT", "CURRENT", "LATEST", "OUTDATED", "LICENSE", "RISK", "VULNS (C/H/M/L)"}
	rows := make([][]string, 0, len(result.Items))

	for _, c := range result.Items {
		rows = append(rows, []string{
			c.Name,
			c.Version,
			scainternal.OrDash(c.LatestVersion),
			scainternal.BoolYesNo(c.Outdated),
			scainternal.OrDash(c.License),
			scainternal.FormatRisk(c.RiskScore),
			scainternal.FormatVulnCounts(c.Metrics, isTTY),
		})
	}

	if err := scainternal.PrintTable(w, isTTY, headers, rows); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	_, err := fmt.Fprintln(w, scainternal.PageFooter("component", result.TotalCount, page, pageSize))
	return err
}
