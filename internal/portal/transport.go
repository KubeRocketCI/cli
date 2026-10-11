package portal

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// maxRedirects caps requests per call; same value as the net/http default.
	maxRedirects = 10

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

// NewHTTPClient returns a client for portal requests: timeout caps each
// request, and redirects follow checkRedirect.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: checkRedirect}
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
