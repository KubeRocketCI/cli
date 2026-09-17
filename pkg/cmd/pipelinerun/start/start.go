// Package start implements the "krci pipelinerun start" command.
package start

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/pipelinerun"
)

type StartOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Pipeline     string
	Params       []string
	Labels       []string
	OutputFormat string
	DryRun       bool

	parsedParams map[string]string
	parsedLabels map[string]string
}

// NewCmdStart returns the "pipelinerun start" cobra.Command.
// runF is the business logic function; pass nil to use the default startRun.
func NewCmdStart(f *cmdutil.Factory, runF func(*StartOptions) error) *cobra.Command {
	opts := &StartOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "start <pipeline>",
		Short: "Start any Tekton pipeline by name",
		Long: `Start a Tekton pipeline by name — the "expert escape hatch": the user
names the pipeline and supplies any params they want to override.

Pipeline params without a default are submitted with an empty value
(strings as "", arrays as []). The CLI does not pre-validate required
params client-side — pass them with --param k=v if your pipeline needs them.

The new run is created with Kubernetes 'metadata.generateName' so the
apiserver assigns the random suffix; the resolved name is read back and
returned in the output.`,
		Args: cmdutil.ExactArgs(1, "a pipeline name", "to list available pipelines: krci pipelinerun list"),
		Example: `  # Start a pipeline with no params
  krci pipelinerun start foo-build

  # Start with a single param
  krci pipelinerun start foo-build --param git-revision=main

  # Start with multiple params and a discoverability label
  krci pipelinerun start foo-build --param k=v --param k2=v2 --label app.edp.epam.com/codebase=my-app

  # Render the would-be PipelineRun without creating it
  krci pipelinerun start foo-build --param k=v --dry-run

  # Output as JSON (for AI agents / scripting)
  krci pipelinerun start foo-build -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Pipeline = args[0]

			if err := opts.validate(); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return startRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringArrayVar(&opts.Params, "param", nil,
		"Pipeline parameter as key=value (repeatable; split on first '=')")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil,
		"Label to attach to the resulting PipelineRun as key=value (repeatable)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false,
		"Render the would-be PipelineRun without creating it (requires -o json or -o yaml)")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json, yaml (yaml only with --dry-run)")

	return cmd
}

func (opts *StartOptions) validate() error {
	if err := cmdutil.ValidateK8sName("<pipeline>", opts.Pipeline); err != nil {
		return err
	}

	if err := pipelinerun.ValidateOutputAndDryRun(opts.OutputFormat, opts.DryRun); err != nil {
		return err
	}

	params, err := pipelinerun.ParseKeyValueList(opts.Params, pipelinerun.KindParameter)
	if err != nil {
		return err
	}

	opts.parsedParams = params

	labels, err := pipelinerun.ParseKeyValueList(opts.Labels, pipelinerun.KindLabel)
	if err != nil {
		return err
	}

	opts.parsedLabels = labels

	return nil
}

func startRun(ctx context.Context, opts *StartOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	client, err := opts.RestClient()
	if err != nil {
		return err
	}

	svc := portal.NewPipelineRunStartService(client, cfg.Namespace)

	result, err := svc.Start(ctx, portal.StartInput{
		Pipeline: opts.Pipeline,
		Params:   opts.parsedParams,
		Labels:   opts.parsedLabels,
		DryRun:   opts.DryRun,
	})
	if err != nil {
		return pipelinerun.HandleAuthError(err)
	}

	return pipelinerun.PresentResult(opts.IO, opts.OutputFormat, opts.DryRun, result)
}
