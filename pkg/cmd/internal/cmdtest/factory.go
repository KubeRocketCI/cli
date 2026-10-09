// Package cmdtest provides shared helpers for verb-level command tests.
// Lives under pkg/cmd/internal/ so any package below pkg/cmd/ can import it.
package cmdtest

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KubeRocketCI/cli/internal/auth"
	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

// NewFactory returns cmdutil.New over in-memory I/O buffers for verb-level
// Cobra tests. The config resolver returns cluster in-cluster and namespace ns.
// RestClient returns nil: inject runF to bypass the network or replace it.
// TokenProvider returns an error: replace it to drive a verb that needs a token.
// Other fields keep the cmdutil.New defaults.
func NewFactory() *cmdutil.Factory {
	f := cmdutil.New(iostreams.New(nil, &bytes.Buffer{}, &bytes.Buffer{}, false))

	f.SetConfigResolver(func() (*config.Config, error) {
		return &config.Config{ClusterName: "in-cluster", Namespace: "ns"}, nil
	})
	f.RestClient = func() (*restapi.ClientWithResponses, error) {
		return nil, nil
	}
	f.TokenProvider = func() (auth.TokenProvider, error) {
		return nil, errors.New("cmdtest: TokenProvider is not stubbed")
	}

	return f
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

// Stderr returns what a verb wrote to the stderr buffer of a factory built by
// NewFactory or NewPortalFactory.
func Stderr(f *cmdutil.Factory) string {
	return f.IOStreams.ErrOut.(*bytes.Buffer).String()
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
