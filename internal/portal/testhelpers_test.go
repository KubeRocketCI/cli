package portal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient creates a Client pointing at the given httptest TLS server.
// The server must be created with httptest.NewTLSServer.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()

	client, err := NewClient(srv.URL, func(_ context.Context) (string, error) {
		return "test-token", nil
	})
	if err != nil {
		t.Fatalf("newTestClient: %v", err)
	}

	// Use the TLS server's own client so self-signed certs are trusted.
	client.http = srv.Client()

	return client
}

func tRPCResponse(data any) map[string]any {
	return map[string]any{
		"result": map[string]any{
			"data": data,
		},
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
