// Package cmdtest provides shared helpers for verb-level command tests.
// Lives under pkg/cmd/internal/ so any package below pkg/cmd/ can import it.
package cmdtest

import (
	"bytes"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// NewFactory returns a *cmdutil.Factory wired with in-memory I/O buffers and
// stub Config/RestClient functions suitable for verb-level Cobra tests.
// Config returns the supplied cluster + namespace; RestClient returns nil
// (callers either inject runF to bypass the network or replace RestClient).
func NewFactory() *cmdutil.Factory {
	return &cmdutil.Factory{
		IOStreams: &iostreams.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
		Config: func() (*config.Config, error) {
			return &config.Config{ClusterName: "in-cluster", Namespace: "ns"}, nil
		},
		RestClient: func() (*restapi.ClientWithResponses, error) {
			return nil, nil
		},
	}
}
