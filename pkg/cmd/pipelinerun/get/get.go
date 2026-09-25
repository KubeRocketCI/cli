// Package get implements the "krci pipelinerun get" command.
package get

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/pipelinerun"
)

const (
	defaultWaitTimeout = time.Hour
	waitPollInterval   = 10 * time.Second
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
	Wait          bool
	Timeout       time.Duration

	pollInterval time.Duration // 0 → waitPollInterval
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

  # Wait until the run finishes (exit 1 unless it succeeded)
  krci pipelinerun get build-my-app-main-a1b2c3 --wait

  # JSON for agents
  krci pipelinerun get review-my-app-main-a1b2c3 --reason -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]

			if err := opts.validate(cmd.Flags().Changed("timeout")); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return getRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.IncludeLogs, "logs", false, "Include pipeline run logs")
	cmd.Flags().BoolVar(&opts.IncludeReason, "reason", false, "Show task tree and failure diagnosis")
	cmd.Flags().BoolVar(&opts.Wait, "wait", false, "Wait until the run finishes; exit 1 unless it succeeded")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", defaultWaitTimeout, "Maximum time to wait with --wait")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "", "Output format: table, json (default: auto-detect)")

	return cmd
}

func (opts *GetOptions) validate(timeoutSet bool) error {
	if timeoutSet && !opts.Wait {
		return errors.New("--timeout requires --wait")
	}

	if opts.Timeout <= 0 {
		return errors.New("--timeout must be greater than 0")
	}

	return nil
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

	result, err := fetchRun(ctx, svc, opts)
	if err != nil {
		return err
	}

	if err := renderRun(opts, result); err != nil {
		return err
	}

	if opts.Wait {
		return finishedError(opts, result.PipelineRuns[0].Status)
	}

	return nil
}

// fetchRun gets the run; with --wait it polls until the run finishes.
func fetchRun(
	ctx context.Context, svc *portal.PipelineRunService, opts *GetOptions,
) (*portal.PipelineRunListResult, error) {
	getOpts := portal.PipelineRunGetOptions{
		IncludeLogs:   opts.IncludeLogs,
		IncludeReason: opts.IncludeReason,
	}

	var (
		result *portal.PipelineRunListResult
		err    error
	)

	if opts.Wait {
		if opts.IO.IsStdoutTTY() {
			msg := fmt.Sprintf("waiting for pipeline run %s to finish (timeout %s)", opts.Name, opts.Timeout)
			_, _ = lipgloss.Fprintln(opts.IO.ErrOut, output.DimStyle.Render(msg))
		}

		waitCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		defer cancel()

		result, err = svc.Wait(waitCtx, opts.Name, getOpts, cmp.Or(opts.pollInterval, waitPollInterval))
		if err != nil && errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("timed out after %s waiting for pipeline run %q to finish", opts.Timeout, opts.Name)
		}
	} else {
		result, err = svc.Get(ctx, opts.Name, getOpts)
	}

	switch {
	case err == nil:
		return result, nil
	case errors.Is(err, portal.ErrNotFound):
		return nil, fmt.Errorf("pipeline run %q not found", opts.Name)
	default:
		return nil, pipelinerun.HandleAuthError(err)
	}
}

// finishedError is the --wait exit signal: nil only for a succeeded run.
func finishedError(opts *GetOptions, status string) error {
	if status == portal.StatusSucceeded {
		return nil
	}

	if opts.IncludeReason {
		return fmt.Errorf("pipeline run %q finished with status %s", opts.Name, status)
	}

	return fmt.Errorf("pipeline run %q finished with status %s; diagnose: krci pipelinerun get %s --reason",
		opts.Name, status, opts.Name)
}

func renderRun(opts *GetOptions, result *portal.PipelineRunListResult) error {
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
