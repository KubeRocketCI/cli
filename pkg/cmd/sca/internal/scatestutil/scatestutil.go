// Package scatestutil exposes helpers shared by every `krci sca <verb>` test
// package — primarily a Factory stub wired to in-memory streams and a nil
// portal client suitable for RunE plumbing tests.
package scatestutil

import (
	"bytes"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// NewFactory returns a Factory with buffered stdout/stderr and a RestClient
// closure that yields (nil, nil). Tests that exercise the pre-network
// validation path — flag parsing, arg checks, bounds, severity canonicalisation
// — can drive a cobra.Command through Execute without touching the network.
func NewFactory() *cmdutil.Factory {
	return &cmdutil.Factory{
		IOStreams: &iostreams.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
		RestClient: func() (*restapi.ClientWithResponses, error) {
			return nil, nil
		},
	}
}
