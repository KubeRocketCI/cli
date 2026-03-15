package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"time"
)

type callbackResult struct {
	Code string
}

const successHTML = `<!DOCTYPE html>
<html><body style="font-family:system-ui;text-align:center;padding:60px">
<h2>Authentication successful!</h2>
<p>You can close this tab and return to the terminal.</p>
</body></html>`

const errorHTML = `<!DOCTYPE html>
<html><body style="font-family:system-ui;text-align:center;padding:60px">
<h2>Authentication failed</h2>
<p><strong>%s</strong>: %s</p>
<p>Please try again.</p>
</body></html>`

// waitForCallback starts a single-use HTTP server that receives the OAuth2 callback.
// It validates the state parameter and returns the authorization code.
// Invalid requests (wrong state, missing code) are rejected with HTTP errors but
// do NOT terminate the server — only a valid success or an IdP error response is terminal.
func waitForCallback(listener net.Listener, expectedState string, timeout time.Duration) (*callbackResult, error) {
	resultCh := make(chan *callbackResult, 1)
	errCh := make(chan error, 1)

	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/callback" {
				http.NotFound(w, r)
				return
			}

			// Check for OAuth error response from the IdP (terminal).
			if errParam := r.URL.Query().Get("error"); errParam != "" {
				desc := r.URL.Query().Get("error_description")
				w.Header().Set("Content-Type", "text/html")

				if _, writeErr := fmt.Fprintf(w, errorHTML,
					html.EscapeString(errParam), html.EscapeString(desc)); writeErr != nil {
					http.Error(w, "write error", http.StatusInternalServerError)
				}

				errCh <- fmt.Errorf("%s: %s", errParam, desc)

				return
			}

			// Validate state parameter to prevent CSRF (constant-time comparison).
			// Invalid state is NOT terminal — keep listening for the legitimate callback.
			stateParam := r.URL.Query().Get("state")
			if subtle.ConstantTimeCompare([]byte(stateParam), []byte(expectedState)) != 1 {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}

			code := r.URL.Query().Get("code")
			if code == "" {
				http.Error(w, "missing authorization code", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "text/html")
			if _, err := fmt.Fprint(w, successHTML); err != nil {
				http.Error(w, "write error", http.StatusInternalServerError)
			}
			resultCh <- &callbackResult{Code: code}
		}),
	}

	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("callback server: %w", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case result := <-resultCh:
		_ = srv.Shutdown(ctx)
		return result, nil
	case err := <-errCh:
		_ = srv.Shutdown(ctx)
		return nil, fmt.Errorf("auth callback: %w", err)
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return nil, fmt.Errorf("authentication timed out after %v", timeout)
	}
}

// generateState creates a cryptographically random state parameter (32 bytes, base64url).
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
