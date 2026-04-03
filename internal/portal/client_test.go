package portal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dummyTokenFunc returns a no-op token function for tests that only exercise URL validation.
func dummyTokenFunc(_ context.Context) (string, error) { return "dummy-token", nil }

func TestNewClient_RequiresHTTPS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		portalURL string
	}{
		{name: "http scheme", portalURL: "http://portal.example.com"},
		{name: "no scheme", portalURL: "portal.example.com"},
		{name: "empty host", portalURL: "https://"},
		{name: "empty string", portalURL: ""},
		{name: "ftp scheme", portalURL: "ftp://portal.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(tt.portalURL, dummyTokenFunc)
			require.Error(t, err)
			assert.Nil(t, client)
			assert.True(t, errors.Is(err, ErrHTTPSRequired), "error should wrap ErrHTTPSRequired")
			assert.Contains(t, err.Error(), "portal URL must use HTTPS")
		})
	}
}

func TestNewClient_AcceptsHTTPS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		portalURL string
	}{
		{name: "basic https", portalURL: "https://portal.example.com"},
		{name: "https with port", portalURL: "https://portal.example.com:8443"},
		{name: "https with path", portalURL: "https://portal.example.com/subpath"},
		{name: "https with trailing slash", portalURL: "https://portal.example.com/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(tt.portalURL, dummyTokenFunc)
			require.NoError(t, err)
			assert.NotNil(t, client)
		})
	}
}

func TestNewClient_TrimTrailingSlash(t *testing.T) {
	t.Parallel()

	client, err := NewClient("https://portal.example.com/", dummyTokenFunc)
	require.NoError(t, err)
	assert.Equal(t, "https://portal.example.com", client.baseURL)
}

func TestQuery_SendsBearerToken(t *testing.T) {
	t.Parallel()

	var gotAuth string

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeJSON(w, tRPCResponse(map[string]string{"key": "value"}))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)

	var result map[string]string

	err := client.Query(context.Background(), "test.proc", nil, &result)
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token", gotAuth)
}
