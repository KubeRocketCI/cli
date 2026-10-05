// Package cmdtest provides shared helpers for verb-level command tests.
// Lives under pkg/cmd/internal/ so any package below pkg/cmd/ can import it.
package cmdtest

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

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

// NewPortalFactory returns NewFactory with a RestClient that talks to a mock
// portal served by handler, and the buffer stdout is written to. Use it to
// drive a verb's own run function.
func NewPortalFactory(t *testing.T, handler http.Handler) (*cmdutil.Factory, *bytes.Buffer) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	out := &bytes.Buffer{}

	f := NewFactory()
	f.IOStreams.Out = out
	f.RestClient = func() (*restapi.ClientWithResponses, error) {
		return restapi.NewClientWithResponses(srv.URL)
	}

	return f, out
}

// PortalReply returns a mock portal that answers every request with status
// and the JSON body.
func PortalReply(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}
