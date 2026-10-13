package portal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

func TestValidatePortalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "https", raw: "https://portal.example.com"},
		{name: "https with port and path", raw: "https://portal.example.com:8443/krci"},
		{name: "upper-case scheme", raw: "HTTPS://portal.example.com"},
		{name: "http IPv4 loopback with port", raw: "http://127.0.0.1:3001"},
		{name: "http IPv4 loopback range", raw: "http://127.0.0.2"},
		{name: "http IPv6 loopback with port", raw: "http://[::1]:3001"},
		{name: "http localhost", raw: "http://localhost:3001"},
		{name: "http LOCALHOST", raw: "http://LOCALHOST"},
		{name: "path", raw: "https://portal.example.com/krci/"},
		{
			name:    "http remote",
			raw:     "http://portal.example.com",
			wantErr: `invalid portal URL: must use https, or http on localhost or a loopback IP: http://portal.example.com`,
		},
		{
			name:    "http localhost subdomain",
			raw:     "http://localhost.example.com",
			wantErr: `invalid portal URL: must use https, or http on localhost or a loopback IP: http://localhost.example.com`,
		},
		{
			name:    "http unspecified address",
			raw:     "http://0.0.0.0:3001",
			wantErr: `invalid portal URL: must use https, or http on localhost or a loopback IP: http://0.0.0.0:3001`,
		},
		{
			name:    "other scheme",
			raw:     "ftp://portal.example.com",
			wantErr: `invalid portal URL: must use https, or http on localhost or a loopback IP: ftp://portal.example.com`,
		},
		{
			name:    "user info",
			raw:     "https://user@portal.example.com",
			wantErr: `invalid portal URL: user info is not allowed: https://user@portal.example.com`,
		},
		{
			name:    "user info with password is redacted",
			raw:     "http://user:secret@127.0.0.1",
			wantErr: `invalid portal URL: user info is not allowed: http://user:xxxxx@127.0.0.1`,
		},
		{
			name:    "user info on a remote http host is redacted",
			raw:     "http://user:secret@portal.example.com",
			wantErr: `invalid portal URL: user info is not allowed: http://user:xxxxx@portal.example.com`,
		},
		{
			name:    "password with invalid port is not echoed",
			raw:     "https://user:secret@portal.example.com:abc",
			wantErr: `invalid portal URL: cannot be parsed`,
		},
		{
			name:    "password without scheme is not echoed",
			raw:     "user:secret@portal.example.com",
			wantErr: `invalid portal URL: a host is required`,
		},
		{
			name:    "query",
			raw:     "https://portal.example.com?x=1",
			wantErr: `invalid portal URL: query and fragment are not allowed`,
		},
		{
			name:    "fragment",
			raw:     "https://portal.example.com#f",
			wantErr: `invalid portal URL: query and fragment are not allowed`,
		},
		{
			name:    "empty query",
			raw:     "https://portal.example.com?",
			wantErr: `invalid portal URL: query and fragment are not allowed`,
		},
		{
			name:    "empty fragment",
			raw:     "https://portal.example.com#",
			wantErr: `invalid portal URL: query and fragment are not allowed`,
		},
		{
			name:    "query token is not echoed",
			raw:     "http://portal.example.com/krci?access_token=secret",
			wantErr: `invalid portal URL: query and fragment are not allowed`,
		},
		{
			name:    "query token with user info is not echoed",
			raw:     "https://user@portal.example.com?access_token=secret",
			wantErr: `invalid portal URL: query and fragment are not allowed`,
		},
		{
			name:    "empty host",
			raw:     "https://",
			wantErr: `invalid portal URL: a host is required`,
		},
		{
			name:    "port without host",
			raw:     "https://:443",
			wantErr: `invalid portal URL: a host is required`,
		},
		{
			name:    "no scheme",
			raw:     "portal.example.com",
			wantErr: `invalid portal URL: a host is required`,
		},
		{
			name:    "empty",
			raw:     "",
			wantErr: `invalid portal URL: a host is required`,
		},
		{
			name:    "garbage",
			raw:     "https://portal example.com/%zz",
			wantErr: `invalid portal URL: cannot be parsed`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePortalURL(tt.raw)

			if tt.wantErr == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, ErrInvalidPortalURL)
			assert.EqualError(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestCheckRedirect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		via     []string
		target  string
		wantErr string
	}{
		{
			name:   "same host https",
			via:    []string{"https://portal.example.com/rest/v1/config"},
			target: "https://portal.example.com/rest/v2/config",
		},
		{
			name:   "same host, case differs",
			via:    []string{"https://portal.example.com/a"},
			target: "https://Portal.Example.com/b",
		},
		{
			name:   "same loopback host http",
			via:    []string{"http://127.0.0.1:3001/a"},
			target: "http://127.0.0.1:3001/b",
		},
		{
			name:    "cross host",
			via:     []string{"https://portal.example.com/a"},
			target:  "https://evil.example.com/a",
			wantErr: "redirect refused from https://portal.example.com/a to https://evil.example.com/a",
		},
		{
			name:   "explicit default https port to implicit",
			via:    []string{"https://portal.example.com:443/a"},
			target: "https://portal.example.com/b",
		},
		{
			name:   "implicit default http port to explicit",
			via:    []string{"http://localhost/a"},
			target: "http://localhost:80/b",
		},
		{
			name:    "default ports differ by scheme",
			via:     []string{"http://localhost/a"},
			target:  "https://localhost/a",
			wantErr: "redirect refused from http://localhost/a to https://localhost/a",
		},
		{
			name:   "loopback upgrade to https",
			via:    []string{"http://127.0.0.1:3001/a"},
			target: "https://127.0.0.1:3001/b",
		},
		{
			name:    "loopback downgrade after an upgrade",
			via:     []string{"http://127.0.0.1:3001/a", "https://127.0.0.1:3001/b"},
			target:  "http://127.0.0.1:3001/c",
			wantErr: "redirect refused from https://127.0.0.1:3001/b to http://127.0.0.1:3001/c",
		},
		{
			name:   "IPv6 loopback same port",
			via:    []string{"http://[::1]:3001/a"},
			target: "http://[::1]:3001/b",
		},
		{
			name:    "same host, other port",
			via:     []string{"https://portal.example.com/a"},
			target:  "https://portal.example.com:8443/a",
			wantErr: "redirect refused from https://portal.example.com/a to https://portal.example.com:8443/a",
		},
		{
			name:    "scheme downgrade",
			via:     []string{"https://portal.example.com/a"},
			target:  "http://portal.example.com/a",
			wantErr: "redirect refused from https://portal.example.com/a to http://portal.example.com/a",
		},
		{
			name:    "downgrade after an allowed hop names the last hop",
			via:     []string{"https://portal.example.com/a", "https://portal.example.com/b"},
			target:  "http://portal.example.com/c",
			wantErr: "redirect refused from https://portal.example.com/b to http://portal.example.com/c",
		},
		{
			name:    "user info is redacted",
			via:     []string{"https://portal.example.com/a"},
			target:  "https://user:secret@evil.example.com/a",
			wantErr: "redirect refused from https://portal.example.com/a to https://user:xxxxx@evil.example.com/a",
		},
		{
			name:    "hop limit",
			via:     strings.Fields(strings.Repeat("https://portal.example.com/a ", maxRedirects)),
			target:  "https://portal.example.com/b",
			wantErr: "stopped after 10 redirects",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			via := make([]*http.Request, 0, len(tt.via))
			for _, raw := range tt.via {
				via = append(via, newRequest(t, raw))
			}

			err := checkRedirect(newRequest(t, tt.target), via)

			if tt.wantErr == "" {
				require.NoError(t, err)

				return
			}

			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestHTTPClient_Redirects(t *testing.T) {
	t.Parallel()

	t.Run("same host is followed", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/start" {
				http.Redirect(w, r, "/end", http.StatusFound)

				return
			}

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(srv.Close)

		resp := get(t, newHTTPClient(5*time.Second), srv.URL+"/start")
		require.NoError(t, resp.err)
		assert.Equal(t, http.StatusNoContent, resp.status)
		assert.Equal(t, "/end", resp.finalPath)
	})

	t.Run("cross host is refused", func(t *testing.T) {
		t.Parallel()

		target, hits := countingServer(t)
		origin := redirectingServer(t, target.URL+"/end")

		resp := get(t, newHTTPClient(5*time.Second), origin.URL+"/start")
		require.ErrorIs(t, resp.err, ErrRedirectRefused)
		assert.Contains(t, resp.err.Error(), "from "+origin.URL+"/start to "+target.URL+"/end")
		assert.Zero(t, hits.Load(), "redirect target was requested")
	})

	t.Run("https to http on the same host is refused", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://"+r.Host+"/end", http.StatusFound)
		}))
		t.Cleanup(srv.Close)

		client := newHTTPClient(5 * time.Second)
		client.Transport = srv.Client().Transport

		resp := get(t, client, srv.URL+"/start")
		require.ErrorIs(t, resp.err, ErrRedirectRefused)
		assert.Contains(t, resp.err.Error(),
			"from "+srv.URL+"/start to http://"+strings.TrimPrefix(srv.URL, "https://")+"/end")
	})
}

func TestDoer_ResponseContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		contentType string
		wantErr     error
		wantErrMsg  string
		wantCT      string
	}{
		{name: "200 JSON", status: http.StatusOK, contentType: "application/json; charset=utf-8",
			wantCT: "application/json; charset=utf-8"},
		{name: "200 web page", status: http.StatusOK, contentType: "text/html; charset=utf-8",
			wantErr: ErrUnexpectedResponse},
		{name: "200 without Content-Type", status: http.StatusOK, wantErr: ErrUnexpectedResponse},
		{name: "200 long Content-Type", status: http.StatusOK, contentType: strings.Repeat("t", 1000),
			wantErr: ErrUnexpectedResponse, wantErrMsg: `Content-Type "` + strings.Repeat("t", maxErrorBodyLen) + `...", not JSON`},
		{name: "401 JSON keeps the body raw", status: http.StatusUnauthorized, contentType: "application/json"},
		{name: "502 page keeps the body raw", status: http.StatusBadGateway, contentType: "text/html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const body = `{"statusCode":401,"error":"Unauthorized"}`

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header()["Content-Type"] = []string{tt.contentType}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, body)
			}))
			t.Cleanup(srv.Close)

			resp, err := newDoer(5*time.Second, maxResponseBodyBytes).Do(newRequest(t, srv.URL))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.ErrorContains(t, err, tt.wantErrMsg)
				assert.Nil(t, resp)

				return
			}

			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.status, resp.StatusCode)
			assert.Equal(t, tt.wantCT, resp.Header.Get("Content-Type"))
			assert.Equal(t, body, string(got))
		})
	}
}

// TestDoer_GeneratedParsers pins the Doer to the generated parsers: every 200
// it passes decodes into JSON200, and an error body of any shape reaches the
// caller undecoded.
func TestDoer_GeneratedParsers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
	}{
		{name: "200 JSON", status: http.StatusOK, contentType: "application/json; charset=utf-8",
			body: `{"clusterName":"c","defaultNamespace":"ns"}`},
		{name: "200 JSON suffix type", status: http.StatusOK, contentType: "application/problem+json",
			body: `{"clusterName":"c","defaultNamespace":"ns"}`},
		{name: "401 Fastify body", status: http.StatusUnauthorized, contentType: "application/json",
			body: `{"statusCode":401,"error":"Unauthorized","message":"Unauthorized"}`},
		{name: "403 not JSON", status: http.StatusForbidden, contentType: "application/json", body: `denied`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(srv.Close)

			client, err := NewRestClient(srv.URL, 5*time.Second)
			require.NoError(t, err)

			resp, err := client.ConfigGetWithResponse(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.body, string(resp.Body))

			if tt.status == http.StatusOK {
				require.NotNil(t, resp.JSON200)
				assert.Equal(t, "ns", resp.JSON200.DefaultNamespace)
			}
		})
	}
}

func TestDoer_BodyCap(t *testing.T) {
	t.Parallel()

	const limit = 8

	tests := []struct {
		name    string
		status  int
		size    int
		wantErr bool
	}{
		{name: "at the cap", status: http.StatusOK, size: limit},
		{name: "past the cap", status: http.StatusOK, size: limit + 1, wantErr: true},
		{name: "error body past the cap", status: http.StatusInternalServerError, size: limit + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, strings.Repeat("x", tt.size))
			}))
			t.Cleanup(srv.Close)

			d := newDoer(5*time.Second, limit)

			resp, err := d.Do(newRequest(t, srv.URL))
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			got, err := io.ReadAll(resp.Body)
			if !tt.wantErr {
				require.NoError(t, err)
				assert.Len(t, got, tt.size)

				return
			}

			require.ErrorIs(t, err, ErrResponseTooLarge)
			assert.Len(t, got, limit)
		})
	}
}

type getResult struct {
	status    int
	finalPath string
	err       error
}

func get(t *testing.T, client *http.Client, rawURL string) getResult {
	t.Helper()

	req := newRequest(t, rawURL)
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := client.Do(req)
	if err != nil {
		return getResult{err: err}
	}

	_ = resp.Body.Close()

	return getResult{status: resp.StatusCode, finalPath: resp.Request.URL.Path}
}

func newRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	require.NoError(t, err)

	return req
}

// countingServer answers 204 and counts the requests it receives.
func countingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	return srv, &hits
}

// redirectingServer answers every request with a 302 to location.
func redirectingServer(t *testing.T, location string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, location, http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestNewRestClient_DoerReplacesCallerHTTPClient(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html></html>")
	}))
	t.Cleanup(srv.Close)

	client, err := NewRestClient(srv.URL, 5*time.Second, restapi.WithHTTPClient(http.DefaultClient))
	require.NoError(t, err)

	_, err = client.ConfigGetWithResponse(t.Context())
	require.ErrorIs(t, err, ErrUnexpectedResponse)
}
