package cmdutil

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
)

func newTestFactory() *Factory {
	return New(iostreams.New(nil, io.Discard, io.Discard, false))
}

func TestFactoryConfig_DefaultResolverFails(t *testing.T) {
	t.Parallel()

	_, err := newTestFactory().Config()

	require.Error(t, err)
	assert.EqualError(t, err, "loading config: config resolver not set")
}

func TestFactoryConfig_CopiedFuncSeesResolver(t *testing.T) {
	t.Parallel()

	f := newTestFactory()
	captured := f.Config

	f.SetConfigResolver(func() (*config.Config, error) {
		return &config.Config{PortalURL: "https://p.example"}, nil
	})

	cfg, err := captured()
	require.NoError(t, err)
	assert.Equal(t, "https://p.example", cfg.PortalURL)
}

func TestFactoryConfig_MemoizesSuccess(t *testing.T) {
	t.Parallel()

	f := newTestFactory()
	calls := 0

	f.SetConfigResolver(func() (*config.Config, error) {
		calls++

		return &config.Config{PortalURL: "https://first.example"}, nil
	})

	first, err := f.Config()
	require.NoError(t, err)

	f.SetConfigResolver(func() (*config.Config, error) {
		calls++

		return &config.Config{PortalURL: "https://second.example"}, nil
	})

	second, err := f.Config()
	require.NoError(t, err)

	assert.Same(t, first, second)
	assert.Equal(t, "https://first.example", second.PortalURL)
	assert.Equal(t, 1, calls)
}

func TestFactoryLazyFuncs_ReadConfigAtCallTime(t *testing.T) {
	t.Parallel()

	f := newTestFactory()
	tokenProvider, restClient := f.TokenProvider, f.RestClient
	cause := errors.New("resolver set after New")

	f.SetConfigResolver(func() (*config.Config, error) { return nil, cause })

	_, err := tokenProvider()
	require.ErrorIs(t, err, cause)

	_, err = restClient()
	require.ErrorIs(t, err, cause)
}

func TestFactoryConfig_DoesNotCacheFailure(t *testing.T) {
	t.Parallel()

	f := newTestFactory()
	cause := errors.New("parse failure")

	f.SetConfigResolver(func() (*config.Config, error) { return nil, cause })

	_, err := f.Config()
	require.ErrorIs(t, err, cause)
	assert.ErrorContains(t, err, "loading config: ")

	f.SetConfigResolver(func() (*config.Config, error) { return &config.Config{Namespace: "ns"}, nil })

	cfg, err := f.Config()
	require.NoError(t, err)
	assert.Equal(t, "ns", cfg.Namespace)
}
