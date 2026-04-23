package portal

import (
	"fmt"
	"net/http"
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
	case http.StatusNotFound:
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
