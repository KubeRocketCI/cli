// Package list implements the "krci pipelinerun list" command.
package list

import (
	"context"
	"errors"
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// ListOptions holds all inputs for the pipelinerun list command.
type ListOptions struct {
	IO            *iostreams.IOStreams
	PortalClient  func() (*portal.Client, error)
	Config        func() (*config.Config, error)
	Project       string
	PRNumber      int
	Author        string
	Branch        string
	Type          string
	Status        string
	OutputFormat  string
	IncludeLogs   bool
	IncludeReason bool
}

// NewCmdList returns the "pipelinerun list" cobra.Command.
// runF is the business logic function; pass nil to use the default listRun.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:           f.IOStreams,
		PortalClient: f.PortalClient,
		Config:       f.Config,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List pipeline runs",
		Aliases: []string{"ls"},
		Example: `  # List last 10 pipeline runs
  krci pipelinerun list

  # Filter by project
  krci pipelinerun list --project edp-tekton

  # Filter by project and pull request
  krci pipelinerun list --project edp-tekton --pr 44

  # Combine filters
  krci pipelinerun list --project edp-tekton --author "John Doe" --type review

  # Include logs for the most recent pipeline run
  krci pipelinerun list --project edp-tekton --pr 44 --logs

  # Diagnose why the most recent run failed
  krci pipelinerun list --project edp-tekton --pr 44 --reason

  # Output as JSON (for agent consumption)
  krci pipelinerun list --project edp-tekton --pr 44 -o json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}

			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Project, "project", "", "Filter by project name")
	cmd.Flags().IntVar(&opts.PRNumber, "pr", 0, "Filter by pull request number")
	cmd.Flags().StringVar(&opts.Author, "author", "", "Filter by author name")
	cmd.Flags().StringVar(&opts.Branch, "branch", "", "Filter by source branch")
	cmd.Flags().StringVar(&opts.Type, "type", "", "Filter by pipeline type (review, build)")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status (succeeded, failed, running, timeout, cancelled)")
	cmd.Flags().BoolVar(&opts.IncludeLogs, "logs", false, "Include pipeline run logs")
	cmd.Flags().BoolVar(&opts.IncludeReason, "reason", false, "Show task tree and failure diagnosis")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "", "Output format: table, json (default: auto-detect)")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	client, err := opts.PortalClient()
	if err != nil {
		return err
	}

	svc := portal.NewPipelineRunService(client, cfg.PortalURL, cfg.ClusterName, cfg.Namespace)

	result, err := svc.List(ctx, portal.PipelineRunListOptions{
		Filter: portal.PipelineRunFilter{
			Project:  opts.Project,
			PRNumber: opts.PRNumber,
			Author:   opts.Author,
			Branch:   opts.Branch,
			Type:     opts.Type,
			Status:   opts.Status,
		},
		IncludeLogs:   opts.IncludeLogs,
		IncludeReason: opts.IncludeReason,
	})
	if err != nil {
		if errors.Is(err, portal.ErrUnauthorized) {
			return cmdutil.ErrAuthRequired(err)
		}

		return err
	}

	if len(result.PipelineRuns) == 0 {
		_, err = lipgloss.Fprintln(opts.IO.Out, output.DimStyle.Render("No pipeline runs found"))

		return err
	}

	// JSON always returns the full struct.
	if output.ResolveFormat(opts.OutputFormat) == output.FormatJSON {
		return output.PrintJSON(opts.IO.Out, result)
	}

	// --reason: pipeline info + task tree + failure details (no summary table).
	if opts.IncludeReason {
		if len(result.Tasks) > 0 {
			return output.RenderReason(opts.IO.Out, result)
		}

		if err := output.RenderNoTaskData(opts.IO.Out, result.PipelineRuns[0].Status); err != nil {
			return err
		}
	}

	// Summary table (default).
	if err := renderSummaryTable(opts, result); err != nil {
		return err
	}

	// --logs: append logs for the most recent run.
	if result.Logs != "" {
		pr := result.PipelineRuns[0]
		header := fmt.Sprintf("Logs: %s (%s)", pr.Name, pr.Status)

		if _, err := fmt.Fprintln(opts.IO.Out); err != nil {
			return err
		}

		if _, err := lipgloss.Fprintln(opts.IO.Out, output.SectionStyle.Render(header)); err != nil {
			return err
		}

		if _, err := fmt.Fprintln(opts.IO.Out, result.Logs); err != nil {
			return err
		}
	}

	return nil
}

// Column width limits for the summary table.
const (
	colWidthName    = 30
	colWidthProject = 14
	colWidthAuthor  = 14
)

// renderSummaryTable renders the pipeline run list as a styled/plain table.
func renderSummaryTable(opts *ListOptions, result *portal.PipelineRunListResult) error {
	return output.RenderList(opts.IO, opts.OutputFormat, result, func(isTTY bool) ([]string, [][]string) {
		headers := []string{"NAME", "STATUS", "PROJECT", "PR", "AUTHOR", "TYPE", "STARTED", "DURATION"}
		rows := make([][]string, 0, len(result.PipelineRuns))

		for _, pr := range result.PipelineRuns {
			status := pr.Status
			prCol := pr.PRNumber

			nameCol := output.Truncate(pr.Name, colWidthName)

			if isTTY {
				status = output.PipelineStatusColor(pr.Status)

				if pr.PRURL != "" && pr.PRNumber != "" {
					prCol = output.Hyperlink(pr.PRNumber, pr.PRURL)
				}

				if pr.PortalURL != "" {
					nameCol = output.Hyperlink(nameCol, pr.PortalURL)
				}
			}

			rows = append(rows, []string{
				nameCol,
				status,
				output.Truncate(pr.Project, colWidthProject),
				prCol,
				output.Truncate(pr.Author, colWidthAuthor),
				pr.Type,
				output.FormatRelativeTime(pr.StartTime),
				pr.Duration,
			})
		}

		return headers, rows
	})
}
