// Package findings implements the "krci sca findings" command.
package findings

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
	scainternal "github.com/KubeRocketCI/cli/pkg/cmd/sca/internal"
)

const (
	maxIDColumnWidth   = 40
	maxCompColumnWidth = 40
)

// FindingsOptions holds all inputs for `krci sca findings <codebase>`.
type FindingsOptions struct {
	IO                *iostreams.IOStreams
	RestClient        func() (*restapi.ClientWithResponses, error)
	Codebase          string
	Branch            string
	Severity          []string
	IncludeSuppressed bool
	Source            string
	OutputFormat      string
}

// NewCmdFindings returns the "sca findings" cobra.Command.
func NewCmdFindings(f *cmdutil.Factory, runF func(*FindingsOptions) error) *cobra.Command {
	opts := &FindingsOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
	}

	cmd := &cobra.Command{
		Use:   "findings <project>",
		Short: "List Dep-Track vulnerability findings for a project",
		Long: fmt.Sprintf(
			"List Dep-Track vulnerability findings for a project.\n\n"+
				"Unpaginated: the portal returns a single response capped at %d rows\n"+
				"server-side. Only --source narrows the upstream query; --severity is\n"+
				"applied client-side after the cap, so it cannot recover findings beyond\n"+
				"row %d.",
			scainternal.FindingsServerCap, scainternal.FindingsServerCap),
		Args: cmdutil.ExactArgs(1, "a KubeRocketCI project name",
			"to see available projects: krci sca list"),
		Example: `  # All unsuppressed findings, default branch
  krci sca findings payments-api

  # Critical only
  krci sca findings payments-api --severity=critical

  # Suppressed findings included, filter by vuln source
  krci sca findings payments-api --include-suppressed --source=NVD

  # Scripting — CVE IDs for the default branch
  krci sca findings payments-api -o json | jq -r '.data.items[].vulnerability.vulnId'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Codebase = args[0]

			if err := scainternal.ValidateCodebaseCommand(
				cmd, opts.OutputFormat, opts.Codebase, opts.Branch,
			); err != nil {
				return err
			}

			if err := scainternal.ValidateNonEmptyFlag(
				"source", cmd.Flags().Changed("source"), opts.Source,
			); err != nil {
				return err
			}

			if _, err := scainternal.ValidateSeverityCSV(opts.Severity); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return findingsRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Branch, "branch", "", scainternal.BranchFlagUsage)
	cmd.Flags().StringSliceVar(&opts.Severity, "severity", nil, scainternal.SeverityFlagUsage)
	cmd.Flags().BoolVar(&opts.IncludeSuppressed, "include-suppressed", false,
		"Include findings whose analysis is marked suppressed")
	cmd.Flags().StringVar(&opts.Source, "source", "",
		"Filter by vulnerability data source (e.g. NVD, GITHUB, OSV)")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func findingsRun(ctx context.Context, opts *FindingsOptions) error {
	client, err := opts.RestClient()
	if err != nil {
		return scainternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	result, err := portal.NewSCAService(client).Findings(ctx, portal.SCAFindingsParams{
		Codebase:          opts.Codebase,
		Branch:            opts.Branch,
		IncludeSuppressed: opts.IncludeSuppressed,
		Source:            opts.Source,
	})
	if err != nil {
		return scainternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	// Client-side inclusive severity filter — server does not narrow by
	// severity. Truncated stays as-returned by the server: hiding it after a
	// client-side filter would silently drop findings that fell off the cap
	// before we ever saw them.
	if inclusive := scainternal.ExpandSeverityFlag(opts.Severity); len(inclusive) > 0 {
		filtered := make([]portal.SCAFinding, 0, len(result.Items))
		for _, f := range result.Items {
			if scainternal.SeverityMatches(f.Vulnerability.Severity, inclusive) {
				filtered = append(filtered, f)
			}
		}
		result.Items = filtered
	}

	return scainternal.Render(opts.IO, opts.OutputFormat, result, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, opts.Codebase, result)
	})
}

func renderTable(w io.Writer, isTTY bool, codebase string, result *portal.SCAFindingList) error {
	if result.Status == portal.SCAStatusNone {
		_, err := fmt.Fprintf(w, "status: NONE — no SCA scanner bound for %s\n", codebase)
		return err
	}

	headers := []string{"COMPONENT", "VERSION", "VULN_ID", "SRC", "SEVERITY", "CVSS", "ANALYSIS", "SUPPRESSED"}
	rows := make([][]string, 0, len(result.Items))

	for _, f := range result.Items {
		compName := f.Component.Name
		vulnID := f.Vulnerability.VulnID
		if isTTY {
			compName = output.Truncate(compName, maxCompColumnWidth)
			vulnID = output.Truncate(vulnID, maxIDColumnWidth)
		}

		severity := f.Vulnerability.Severity
		if isTTY {
			severity = output.SCASeverityColor(f.Vulnerability.Severity)
		}

		rows = append(rows, []string{
			compName,
			f.Component.Version,
			vulnID,
			f.Vulnerability.Source,
			severity,
			formatCVSS(f.Vulnerability.CvssV3BaseScore, f.Vulnerability.CvssV2BaseScore),
			f.Analysis.State,
			scainternal.BoolYesNo(f.Analysis.IsSuppressed),
		})
	}

	if err := scainternal.PrintTable(w, isTTY, headers, rows); err != nil {
		return err
	}

	if result.Truncated {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w,
			"(findings truncated to %d rows server-side — only --source narrows the upstream query;"+
				" --severity filters client-side and cannot recover capped rows)\n",
			scainternal.FindingsServerCap); err != nil {
			return err
		}
	}

	return nil
}

func formatCVSS(v3, v2 float32) string {
	if v3 > 0 {
		return fmt.Sprintf("%.1f (v3)", v3)
	}
	if v2 > 0 {
		return fmt.Sprintf("%.1f (v2)", v2)
	}
	return scainternal.EmptyPlaceholder
}
