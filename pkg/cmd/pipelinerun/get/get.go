// Package get implements the "krci pipelinerun get" command.
package get

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
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// GetOptions holds all inputs for the pipelinerun get command.
type GetOptions struct {
	IO            *iostreams.IOStreams
	RestClient    func() (*restapi.ClientWithResponses, error)
	Config        func() (*config.Config, error)
	Name          string
	OutputFormat  string
	IncludeLogs   bool
	IncludeReason bool
}

// NewCmdGet returns the "pipelinerun get" cobra.Command.
func NewCmdGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get pipeline run details",
		Args:  cmdutil.ExactArgs(1, "a pipeline run name", "to find run names: krci pipelinerun list"),
		Example: `  # Get pipeline run info
  krci pipelinerun get review-my-app-main-a1b2c3

  # Get failure diagnosis (task tree + failed step + logs)
  krci pipelinerun get review-my-app-main-a1b2c3 --reason

  # Get full logs
  krci pipelinerun get review-my-app-main-a1b2c3 --logs

  # JSON for agents
  krci pipelinerun get review-my-app-main-a1b2c3 --reason -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]

			if runF != nil {
				return runF(opts)
			}

			return getRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.IncludeLogs, "logs", false, "Include pipeline run logs")
	cmd.Flags().BoolVar(&opts.IncludeReason, "reason", false, "Show task tree and failure diagnosis")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "", "Output format: table, json (default: auto-detect)")

	return cmd
}

func getRun(ctx context.Context, opts *GetOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	client, err := opts.RestClient()
	if err != nil {
		return err
	}

	svc := portal.NewPipelineRunService(client, cfg.PortalURL, cfg.ClusterName, cfg.Namespace)

	result, err := svc.UnifiedGet(ctx, opts.Name, portal.PipelineRunGetOptions{
		IncludeLogs:   opts.IncludeLogs,
		IncludeReason: opts.IncludeReason,
	})
	if err != nil {
		if errors.Is(err, portal.ErrNotFound) {
			return fmt.Errorf("pipeline run %q not found", opts.Name)
		}

		if errors.Is(err, portal.ErrUnauthorized) {
			return cmdutil.ErrAuthRequired(err)
		}

		return err
	}

	if opts.IncludeReason {
		output.TruncateTaskLogs(result)
	}

	if output.ResolveFormat(opts.OutputFormat) == output.FormatJSON {
		return output.PrintJSON(opts.IO.Out, result)
	}

	run := &result.PipelineRuns[0]

	// --reason: full diagnosis view (shows task tree even for succeeded runs).
	if opts.IncludeReason {
		if len(result.Tasks) > 0 {
			return output.RenderReason(opts.IO.Out, result)
		}

		if err := output.RenderNoTaskData(opts.IO.Out, run.Status); err != nil {
			return err
		}
	}

	// Default / --logs: pipeline info + optional logs.
	if err := output.RenderRunInfo(opts.IO.Out, run); err != nil {
		return err
	}

	if result.Logs != "" {
		if _, err := fmt.Fprintln(opts.IO.Out); err != nil {
			return err
		}

		header := fmt.Sprintf("Logs: %s", run.Name)

		if _, err := lipgloss.Fprintln(opts.IO.Out, output.SectionStyle.Render(header)); err != nil {
			return err
		}

		if _, err := fmt.Fprintln(opts.IO.Out, result.Logs); err != nil {
			return err
		}
	}

	return nil
}
