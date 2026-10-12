package cmdutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/portal"
)

func TestRewritef(t *testing.T) {
	t.Parallel()

	got := Rewritef(portal.ErrEnvNotFound, "environment %q not found", "dev")

	assert.EqualError(t, got, `environment "dev" not found`)
	require.ErrorIs(t, got, portal.ErrEnvNotFound)
	require.ErrorIs(t, got, portal.ErrNotFound)
	assert.NotContains(t, got.Error(), portal.ErrEnvNotFound.Error())
}
