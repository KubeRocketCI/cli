// Package envinternal holds what the `krci env` verbs that take
// <deployment> <env> share: the positionals, their validation, and the
// not-found messages.
package envinternal

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
)

// TargetHelp explains the two positionals in the long help of a verb.
const TargetHelp = `<deployment> is the parent CDPipeline name. <env> is Stage.spec.name (the
short user-facing identifier such as "dev", "stage", "prod").`

// TargetArgs requires the two positionals of `krci env <verb> <deployment> <env>`.
func TargetArgs(verb string) cobra.PositionalArgs {
	return cmdutil.ExactArgs(2,
		fmt.Sprintf("a deployment and an env (e.g. krci env %s my-pipeline prod)", verb),
		"to see available environments: krci env list")
}

// ValidateTarget checks the output format and both positionals, before any
// Portal call.
func ValidateTarget(outputFormat, deployment, env string) error {
	if err := discovery.ValidateOutputFormat(outputFormat); err != nil {
		return err
	}

	if err := cmdutil.ValidateK8sName("<deployment>", deployment); err != nil {
		return err
	}

	return cmdutil.ValidateK8sName("<env>", env)
}

// MapNotFound rewrites the not-found sentinels of portal.EnvService into the
// message that names the deployment and the environment. Other errors pass
// through unchanged.
func MapNotFound(err error, deployment, env string) error {
	switch {
	case errors.Is(err, portal.ErrEnvNotFound):
		return fmt.Errorf("environment %q not found in deployment %q", env, deployment)
	case errors.Is(err, portal.ErrDeploymentNotFound):
		return fmt.Errorf("deployment %q not found", deployment)
	default:
		return err
	}
}
