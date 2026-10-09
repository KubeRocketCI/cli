package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallbackSuccess(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port
	state := "test-state-123"

	resultCh := make(chan *callbackResult, 1)
	errCh := make(chan error, 1)

	go func() {
		r, e := waitForCallback(t.Context(), listener, state, 5*time.Second)
		if e != nil {
			errCh <- e
			return
		}
		resultCh <- r
	}()

	// Simulate IdP callback.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?code=auth-code-xyz&state=%s", port, state))
	require.NoError(t, err)
	resp.Body.Close()

	select {
	case result := <-resultCh:
		assert.Equal(t, "auth-code-xyz", result.Code)
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for callback result")
	}
}

func TestCallbackStateMismatchKeepsListening(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port
	state := "correct-state"

	resultCh := make(chan *callbackResult, 1)
	errCh := make(chan error, 1)

	go func() {
		r, e := waitForCallback(t.Context(), listener, state, 5*time.Second)
		if e != nil {
			errCh <- e
			return
		}
		resultCh <- r
	}()

	// Send request with wrong state -- should be rejected but server continues.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?code=bad&state=wrong-state", port))
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Now send correct callback -- server should still be running.
	resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?code=good-code&state=%s", port, state))
	require.NoError(t, err)
	resp.Body.Close()

	select {
	case result := <-resultCh:
		assert.Equal(t, "good-code", result.Code)
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout -- server may have died on state mismatch")
	}
}

func TestCallbackOAuthError(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port

	errCh := make(chan error, 1)

	go func() {
		_, e := waitForCallback(t.Context(), listener, "state", 5*time.Second)
		errCh <- e
	}()

	// Simulate IdP error response.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?error=access_denied&error_description=user+cancelled", port))
	require.NoError(t, err)
	resp.Body.Close()

	select {
	case err := <-errCh:
		require.Error(t, err, "expected error for OAuth error response")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for error")
	}
}

func TestCallbackTimeout(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	_, err = waitForCallback(t.Context(), listener, "state", 100*time.Millisecond)
	require.EqualError(t, err, "authentication timed out after 100ms")
	assert.NotErrorIs(t, err, context.Canceled)
}

func TestCallbackCancel(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := listener.Addr().String()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)

	go func() {
		_, e := waitForCallback(ctx, listener, "state", time.Minute)
		errCh <- e
	}()

	// The server accepts connections in order: once the 404 arrives, the idle
	// connection dialed before it is tracked by the server.
	idle, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	defer idle.Close()

	resp, err := http.Get("http://" + addr + "/x")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
		assert.EqualError(t, err, "authentication cancelled: context canceled")
	case <-time.After(time.Second):
		t.Fatal("waitForCallback did not return within 1s of cancellation")
	}

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("listener still accepts connections after cancellation")
	}
}

func TestGenerateState(t *testing.T) {
	t.Parallel()

	s1, err := generateState()
	require.NoError(t, err)

	s2, err := generateState()
	require.NoError(t, err)

	assert.NotEmpty(t, s1)
	assert.NotEqual(t, s1, s2, "two calls produced identical state (randomness failure)")
}
