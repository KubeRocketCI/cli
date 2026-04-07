package portal

import (
	"fmt"
	"net/http"
)

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
		return fmt.Errorf("portal returned HTTP %d: %s", statusCode, truncate(body, 200))
	}
}

func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}

	return string(b[:max]) + "..."
}
