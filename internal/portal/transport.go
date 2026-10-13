package portal

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

const (
	// maxRedirects caps requests per call; same value as the net/http default.
	maxRedirects = 10

	// maxResponseBodyBytes caps a portal response body read through a doer.
	maxResponseBodyBytes = 64 << 20

	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// ValidatePortalURL accepts an absolute URL with a host, no user info, no query
// and no fragment, whose scheme is https, or http when the host is localhost or
// a loopback IP. A path is allowed.
// Failure wraps ErrInvalidPortalURL. The message carries the URL, password
// redacted, only when it parsed with a host and has no query or fragment.
func ValidatePortalURL(raw string) error {
	u, err := url.Parse(raw)

	switch {
	case err != nil:
		return fmt.Errorf("%w: cannot be parsed", ErrInvalidPortalURL)
	case u.Hostname() == "":
		return fmt.Errorf("%w: a host is required", ErrInvalidPortalURL)
	case strings.ContainsAny(raw, "?#"):
		return fmt.Errorf("%w: query and fragment are not allowed", ErrInvalidPortalURL)
	case u.User != nil:
		return fmt.Errorf("%w: user info is not allowed: %s", ErrInvalidPortalURL, u.Redacted())
	case !allowedScheme(u):
		return fmt.Errorf("%w: must use https, or http on localhost or a loopback IP: %s",
			ErrInvalidPortalURL, u.Redacted())
	}

	return nil
}

func allowedScheme(u *url.URL) bool {
	switch u.Scheme {
	case schemeHTTPS:
		return true
	case schemeHTTP:
		return isLoopbackHost(u.Hostname())
	default:
		return false
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// newHTTPClient returns a client for portal requests: timeout caps each
// request, and redirects follow checkRedirect.
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: checkRedirect}
}

// doer sends portal REST requests and enforces the response contract every
// endpoint shares:
//   - a 200 must carry a Content-Type the generated parsers decode as JSON,
//     else ErrUnexpectedResponse;
//   - any other status has its Content-Type removed: the error body stays raw
//     for checkResponse, and the generated parsers do not decode it;
//   - reading a body past the cap fails with ErrResponseTooLarge.
type doer struct {
	client       *http.Client
	maxBodyBytes int64
}

// NewRestClient returns the generated portal client sending through a doer:
// requests time out after timeout, bodies are capped at maxResponseBodyBytes.
// The doer replaces any HTTP client in opts.
func NewRestClient(
	baseURL string, timeout time.Duration, opts ...restapi.ClientOption,
) (*restapi.ClientWithResponses, error) {
	opts = slices.Concat(opts, []restapi.ClientOption{restapi.WithHTTPClient(newDoer(timeout, maxResponseBodyBytes))})

	return restapi.NewClientWithResponses(baseURL, opts...)
}

func newDoer(timeout time.Duration, maxBodyBytes int64) *doer {
	return &doer{client: newHTTPClient(timeout), maxBodyBytes: maxBodyBytes}
}

// Do sends req; see doer for the response contract.
func (d *doer) Do(req *http.Request) (*http.Response, error) {
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}

	resp.Body = cappedBody{http.MaxBytesReader(nil, resp.Body, d.maxBodyBytes)}

	if resp.StatusCode != http.StatusOK {
		resp.Header.Del("Content-Type")

		return resp, nil
	}

	// Same test as the generated parsers apply before decoding JSON200.
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "json") {
		_ = resp.Body.Close()

		return nil, fmt.Errorf("%w: HTTP 200 with Content-Type %q, not JSON",
			ErrUnexpectedResponse, truncateBody([]byte(ct)))
	}

	return resp, nil
}

// cappedBody reports the http.MaxBytesReader limit as ErrResponseTooLarge.
type cappedBody struct {
	io.ReadCloser
}

func (b cappedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err == nil {
		return n, nil
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return n, fmt.Errorf("%w: more than %d bytes", ErrResponseTooLarge, tooLarge.Limit)
	}

	return n, err
}

// checkRedirect stops the chain once maxRedirects requests have been made.
// Every hop stays on the host and effective port of the first request. A hop
// from https to http is refused.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}

	first, prev := via[0].URL, via[len(via)-1].URL

	if !sameHostPort(req.URL, first) || (prev.Scheme == schemeHTTPS && req.URL.Scheme != schemeHTTPS) {
		return fmt.Errorf("%w from %s to %s", ErrRedirectRefused, prev.Redacted(), req.URL.Redacted())
	}

	return nil
}

func sameHostPort(a, b *url.URL) bool {
	return strings.EqualFold(a.Hostname(), b.Hostname()) && effectivePort(a) == effectivePort(b)
}

// effectivePort is the explicit port, else 443 for https and 80 for http.
func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}

	switch u.Scheme {
	case schemeHTTPS:
		return "443"
	case schemeHTTP:
		return "80"
	default:
		return ""
	}
}
