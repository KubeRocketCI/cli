package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
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
// Terminal: a state-matched request carrying a code or an IdP error.
// Rejected with 400, wait continues: wrong or missing state (error callbacks included), missing code.
// The wait ends after timeout or when ctx is cancelled; cancellation returns an
// error wrapping ctx.Err(). An empty expectedState is an error; the listener is closed.
func waitForCallback(
	ctx context.Context, listener net.Listener, expectedState string, timeout time.Duration,
) (*callbackResult, error) {
	if expectedState == "" {
		_ = listener.Close()
		return nil, errors.New("auth callback: empty expected state")
	}

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

			q := r.URL.Query()

			// State is compared before any other parameter; a forged code or error callback never ends the wait.
			if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(expectedState)) != 1 {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}

			// OAuth error response from the IdP (terminal).
			if errParam := q.Get("error"); errParam != "" {
				desc := q.Get("error_description")
				w.Header().Set("Content-Type", "text/html")

				if _, writeErr := fmt.Fprintf(w, errorHTML,
					html.EscapeString(errParam), html.EscapeString(desc)); writeErr != nil {
					http.Error(w, "write error", http.StatusInternalServerError)
				}

				errCh <- fmt.Errorf("%s: %s", errParam, desc)

				return
			}

			code := q.Get("code")
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

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case result := <-resultCh:
		_ = srv.Shutdown(waitCtx)
		return result, nil
	case err := <-errCh:
		_ = srv.Shutdown(waitCtx)
		return nil, fmt.Errorf("auth callback: %w", err)
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			// Close drops idle browser preconnects; Shutdown waits on them for 5 s.
			_ = srv.Close()
			return nil, fmt.Errorf("authentication cancelled: %w", ctx.Err())
		}

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
