package portal

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// maxErrorBodyLen bounds the portion of a portal error body that is echoed back
// to the user. Keeps stderr readable and avoids dumping long HTML error pages.
const maxErrorBodyLen = 200

// checkResponse maps HTTP status codes to domain errors.
func checkResponse(statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrPermissionDenied
	case http.StatusNotFound:
		if err := routeMissingError(body); err != nil {
			return err
		}

		return ErrNotFound
	default:
		return fmt.Errorf("portal returned HTTP %d: %s", statusCode, truncateBody(body))
	}
}

// truncateBody shortens a server-provided byte body to maxErrorBodyLen ASCII
// characters, appending "..." when the body was cut. Shared by every caller
// that needs to surface a remote response body in an error message.
func truncateBody(b []byte) string {
	if len(b) <= maxErrorBodyLen {
		return string(b)
	}

	return string(b[:maxErrorBodyLen]) + "..."
}

// reasonError is one entry of a reasonTable: the sentinel callers match with
// errors.Is, and the message format. Format verbs are positional (%[1]s, …)
// over the args passed to checkReasonedResponse.
type reasonError struct {
	sentinel error
	format   string
}

func (r reasonError) err(args ...any) error {
	return newRichErr(fmt.Sprintf(r.format, args...), r.sentinel)
}

// reasonTable maps the stable `error.reason` tags one portal endpoint emits
// to the CLI error for each. Tags are unique across HTTP statuses; the
// status is not part of the key.
type reasonTable map[string]reasonError

// checkReasonedResponse maps a response from an endpoint whose error bodies
// carry `error.reason`. Order: 200 → nil; a known reason → its reasonError;
// 404 without a known reason → routeMissingError, else notFound;
// admission-class 4xx and 5xx → platformStatusError; anything else,
// including 401 and 403 → checkResponse. args feed every reasonError format.
func checkReasonedResponse(statusCode int, body []byte, reasons reasonTable, notFound reasonError, args ...any) error {
	if statusCode == http.StatusOK {
		return nil
	}

	reason, _ := parseErrorEnvelope(body)
	if r, ok := reasons[reason]; ok {
		return r.err(args...)
	}

	if statusCode == http.StatusNotFound {
		if err := routeMissingError(body); err != nil {
			return err
		}

		return notFound.err(args...)
	}

	if err := platformStatusError(statusCode, body); err != nil {
		return err
	}

	return checkResponse(statusCode, body)
}

// parseErrorEnvelope reads error.reason and the user-facing message from a
// Portal error body. Returns ("", "") on parse failure. message prefers
// error.message, falling back to top-level message.
func parseErrorEnvelope(body []byte) (reason, message string) {
	var env struct {
		Error struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &env); err != nil {
		return "", ""
	}

	message = env.Error.Message
	if message == "" {
		message = env.Message
	}

	return env.Error.Reason, message
}

// platformStatusError maps the statuses the portal forwards from Kubernetes
// without a stable reason tag: admission-class 4xx to ErrPlatformReject with
// the static phrase, 5xx to ErrUpstreamUnavailable with the truncated body.
// nil for every other status.
func platformStatusError(statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusBadRequest, http.StatusRequestTimeout, http.StatusConflict,
		http.StatusUnprocessableEntity, http.StatusTooManyRequests:
		_, message := parseErrorEnvelope(body)
		return fmt.Errorf("%w: %s", ErrPlatformReject, cmp.Or(message, http.StatusText(statusCode)))
	case http.StatusBadGateway, http.StatusServiceUnavailable,
		http.StatusInternalServerError, http.StatusGatewayTimeout:
		return fmt.Errorf("%w: %s", ErrUpstreamUnavailable, truncateBody(body))
	}

	return nil
}

// routeMissingError returns ErrPortalUnsupported when a 404 body is Fastify's
// route-not-found response, i.e. the portal predates the endpoint:
// {"message":"Route POST:/rest/v1/... not found","error":"Not Found","statusCode":404}.
// Portal application errors nest an object under "error"; here it is a string.
// nil for any other body.
func routeMissingError(body []byte) error {
	var b struct {
		Message    string `json:"message"`
		Error      string `json:"error"`
		StatusCode int    `json:"statusCode"`
	}

	if err := json.Unmarshal(body, &b); err != nil ||
		b.StatusCode != http.StatusNotFound || !strings.HasPrefix(b.Message, "Route ") {
		return nil
	}

	return newRichErr(
		fmt.Sprintf("portal has no endpoint for this command (%s); upgrade the portal", b.Message),
		ErrPortalUnsupported)
}
