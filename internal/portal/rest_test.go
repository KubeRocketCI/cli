package portal

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

const fastifyRouteMissing = `{"message":"Route POST:/rest/v1/pipelineruns/build not found","error":"Not Found","statusCode":404}`

func TestRouteMissingError(t *testing.T) {
	t.Parallel()

	err := routeMissingError([]byte(fastifyRouteMissing))
	if !errors.Is(err, ErrPortalUnsupported) {
		t.Fatalf("expected ErrPortalUnsupported, got %v", err)
	}

	if !strings.Contains(err.Error(), "Route POST:/rest/v1/pipelineruns/build not found") ||
		!strings.Contains(err.Error(), "upgrade the portal") {
		t.Fatalf("message should name the route and the remedy: %q", err.Error())
	}

	for name, body := range map[string]string{
		"portal envelope": `{"error":{"code":"NOT_FOUND","reason":"codebase_not_found","message":"Not Found"}}`,
		"plain text":      `not found`,
		"other message":   `{"message":"gone","error":"Not Found","statusCode":404}`,
	} {
		if err := routeMissingError([]byte(body)); err != nil {
			t.Errorf("%s: expected nil, got %v", name, err)
		}
	}
}

func TestCheckResponse_RouteMissingIsPortalUnsupported(t *testing.T) {
	t.Parallel()

	if err := checkResponse(http.StatusNotFound, []byte(fastifyRouteMissing)); !errors.Is(err, ErrPortalUnsupported) {
		t.Fatalf("expected ErrPortalUnsupported, got %v", err)
	}

	if err := checkResponse(http.StatusNotFound, []byte(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCheckResponse_AuthStatuses(t *testing.T) {
	t.Parallel()

	if err := checkResponse(http.StatusUnauthorized, nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401: expected ErrUnauthorized, got %v", err)
	}

	if err := checkResponse(http.StatusForbidden, nil); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("403: expected ErrPermissionDenied, got %v", err)
	}
}

func TestPlatformStatusError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status   int
		body     string
		sentinel error
		contains string
	}{
		{http.StatusBadRequest, `{"error":{"code":"BAD_REQUEST","message":"Bad Request"}}`, ErrPlatformReject, "Bad Request"},
		{http.StatusConflict, ``, ErrPlatformReject, http.StatusText(http.StatusConflict)},
		{http.StatusUnprocessableEntity, ``, ErrPlatformReject, http.StatusText(http.StatusUnprocessableEntity)},
		{http.StatusInternalServerError, `boom`, ErrUpstreamUnavailable, "boom"},
		{http.StatusBadGateway, ``, ErrUpstreamUnavailable, ""},
	}

	for _, tc := range cases {
		err := platformStatusError(tc.status, []byte(tc.body))
		if !errors.Is(err, tc.sentinel) || !strings.Contains(err.Error(), tc.contains) {
			t.Errorf("status %d: got %v", tc.status, err)
		}
	}

	for _, status := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		if err := platformStatusError(status, nil); err != nil {
			t.Errorf("status %d: expected nil, got %v", status, err)
		}
	}
}
