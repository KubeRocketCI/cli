package pipelineruninternal

import (
	"errors"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// HandleAuthError maps portal.ErrUnauthorized to the auth-required remediation
// hint; other errors pass through unchanged.
func HandleAuthError(err error) error {
	if errors.Is(err, portal.ErrUnauthorized) {
		return cmdutil.ErrAuthRequired(err)
	}

	return err
}
