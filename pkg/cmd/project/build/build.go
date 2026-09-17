// Package build implements the "krci project build <name>" command.
package build

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/pipelinerun"
)

// BuildOptions holds all inputs for `krci project build <name>`.
type BuildOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Project      string
	Branch       string
	Params       []string
	OutputFormat string
	DryRun       bool

	parsedParams map[string]string
}

// NewCmdBuild returns the "project build <name>" cobra.Command.
// runF is the business-logic function; pass nil to use the default run.
func NewCmdBuild(f *cmdutil.Factory, runF func(*BuildOptions) error) *cobra.Command {
	opts := &BuildOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "build <name>",
		Short: "Build a project branch the way the Portal's Build button does",
		Long: fmt.Sprintf(`Start the build pipeline of a project branch.

The build pipeline, its params, labels and service account are resolved from
the project and the branch — you never name the pipeline, the CodebaseBranch,
or the TriggerTemplate. --branch defaults to the project's default branch.

Params derived from the project and branch cannot be overridden:
%s
--param is for everything else (COMMIT_MESSAGE and pipeline-specific params);
use 'krci pipelinerun start' when you need raw control over a run.

A build already running for the branch is rejected: every build run of the
branch is checked, not only the latest one, so an older run that is still
active blocks too. The check is list-then-create, not a platform guarantee,
so two callers firing at once can both get a run.`,
			strings.Join(portal.BuildManagedParams, ", ")),
		Args: cmdutil.ExactArgs(1, "a project name", "to list projects: krci project list"),
		Example: `  # Build the project's default branch
  krci project build my-app

  # Build a specific branch
  krci project build my-app --branch feat/x

  # Override a non-managed param
  krci project build my-app --param COMMIT_MESSAGE="rebuild after config change"

  # Render the would-be PipelineRun without creating it
  krci project build my-app --dry-run -o yaml

  # JSON output (for AI agents / scripting)
  krci project build my-app -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Project = args[0]

			if err := opts.validate(); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return buildRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Branch, "branch", "",
		"Git branch to build (default: the project's default branch)")
	cmd.Flags().StringArrayVar(&opts.Params, "param", nil,
		"Pipeline parameter as key=value (repeatable; split on first '=')")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false,
		"Render the would-be PipelineRun without creating it (requires -o json or -o yaml)")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json, yaml (yaml only with --dry-run)")

	return cmd
}

func (opts *BuildOptions) validate() error {
	if err := cmdutil.ValidateK8sName("<name>", opts.Project); err != nil {
		return err
	}

	if err := opts.validateBranch(); err != nil {
		return err
	}

	if err := pipelinerun.ValidateOutputAndDryRun(opts.OutputFormat, opts.DryRun); err != nil {
		return err
	}

	params, err := pipelinerun.ParseKeyValueList(opts.Params, pipelinerun.KindParameter)
	if err != nil {
		return err
	}

	if err := portal.ValidateBuildParams(params); err != nil {
		return err
	}

	opts.parsedParams = params

	return nil
}

// validateBranch mirrors the portal's branch bound. An empty value means the
// server resolves codebase.spec.defaultBranch.
func (opts *BuildOptions) validateBranch() error {
	if len(opts.Branch) > portal.MaxBranchLength {
		return fmt.Errorf("--branch must be at most %d characters", portal.MaxBranchLength)
	}

	return nil
}

func buildRun(ctx context.Context, opts *BuildOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	client, err := opts.RestClient()
	if err != nil {
		return err
	}

	svc := portal.NewProjectBuildService(client, cfg.Namespace)

	result, err := svc.Build(ctx, portal.BuildInput{
		Codebase: opts.Project,
		Branch:   opts.Branch,
		Params:   opts.parsedParams,
		DryRun:   opts.DryRun,
	})
	if err != nil {
		return pipelinerun.HandleAuthError(err)
	}

	return pipelinerun.PresentResult(opts.IO, opts.OutputFormat, opts.DryRun, result)
}
