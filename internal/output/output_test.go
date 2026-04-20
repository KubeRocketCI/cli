package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/KubeRocketCI/cli/internal/portal"
)

func TestPrintJSONEnvelope(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	payload := map[string]int{"bugs": 3}

	if err := PrintJSONEnvelope(&buf, "1", payload); err != nil {
		t.Fatalf("PrintJSONEnvelope returned error: %v", err)
	}

	var got struct {
		SchemaVersion string         `json:"schemaVersion"`
		Data          map[string]int `json:"data"`
	}

	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got.SchemaVersion != "1" {
		t.Errorf("schemaVersion = %q, want 1", got.SchemaVersion)
	}

	if got.Data["bugs"] != 3 {
		t.Errorf("data.bugs = %d, want 3", got.Data["bugs"])
	}
}

func TestPrintJSONErrorEnvelope(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	if err := PrintJSONErrorEnvelope(&buf, "1", errors.New("boom")); err != nil {
		t.Fatalf("PrintJSONErrorEnvelope returned error: %v", err)
	}

	var got struct {
		SchemaVersion string `json:"schemaVersion"`
		Error         struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got.SchemaVersion != "1" {
		t.Errorf("schemaVersion = %q, want 1", got.SchemaVersion)
	}

	if got.Error.Message != "boom" {
		t.Errorf("error.message = %q, want boom", got.Error.Message)
	}
}

func TestSonarGateStatusColor_PassthroughWhenUnknown(t *testing.T) {
	t.Parallel()

	// Unknown statuses must pass through without ANSI codes.
	got := SonarGateStatusColor(portal.QualityGateStatus("UNKNOWN_STATUS"))
	if got != "UNKNOWN_STATUS" {
		t.Errorf("SonarGateStatusColor returned %q, want %q", got, "UNKNOWN_STATUS")
	}
}

func TestSonarGateStatusColor_RecognizesKnownStatuses(t *testing.T) {
	t.Parallel()

	// For known statuses, the returned string should at least still contain the status text.
	for _, s := range []portal.QualityGateStatus{portal.QualityGateOK, portal.QualityGateWarn, portal.QualityGateError, portal.QualityGateNone} {
		got := SonarGateStatusColor(s)
		if !strings.Contains(got, string(s)) {
			t.Errorf("SonarGateStatusColor(%q) returned %q, missing status text", s, got)
		}
	}
}

func TestSonarGateStatusColor_EmptyStatusReturnsEmpty(t *testing.T) {
	t.Parallel()

	// An empty QualityGateStatus hits the default branch and must return "".
	got := SonarGateStatusColor(portal.QualityGateStatus(""))
	if got != "" {
		t.Errorf("SonarGateStatusColor(%q) = %q, want %q", "", got, "")
	}
}

func TestRelativeTime(t *testing.T) {
	t.Parallel()

	const justNow = "just now"

	now := time.Now()

	t.Run("just now", func(t *testing.T) {
		t.Parallel()

		got := RelativeTime(now)
		if got != justNow {
			t.Errorf("RelativeTime(now) = %q, want %q", got, justNow)
		}
	})

	t.Run("seconds ago is still just now", func(t *testing.T) {
		t.Parallel()

		got := RelativeTime(now.Add(-30 * time.Second))
		if got != justNow {
			t.Errorf("RelativeTime(30s ago) = %q, want %q", got, justNow)
		}
	})

	t.Run("minutes ago", func(t *testing.T) {
		t.Parallel()

		got := RelativeTime(now.Add(-30 * time.Minute))
		// Allow 29m or 30m due to slight wall-clock drift during test execution.
		if !strings.HasSuffix(got, "m ago") {
			t.Errorf("RelativeTime(30m ago) = %q, want something like '29m ago' or '30m ago'", got)
		}
	})

	t.Run("hours ago", func(t *testing.T) {
		t.Parallel()

		got := RelativeTime(now.Add(-3 * time.Hour))
		if !strings.HasSuffix(got, "h ago") {
			t.Errorf("RelativeTime(3h ago) = %q, want something like '3h ago'", got)
		}
	})

	t.Run("days ago", func(t *testing.T) {
		t.Parallel()

		got := RelativeTime(now.Add(-3 * 24 * time.Hour))
		if !strings.HasSuffix(got, "d ago") {
			t.Errorf("RelativeTime(3d ago) = %q, want something like '3d ago'", got)
		}
	})

	t.Run("over a week formats as date", func(t *testing.T) {
		t.Parallel()

		past := now.Add(-8 * 24 * time.Hour)
		got := RelativeTime(past)
		want := past.Format("Jan 02 15:04")
		if got != want {
			t.Errorf("RelativeTime(8d ago) = %q, want %q", got, want)
		}
	})
}

func TestFormatRelativeTime_ParseFailureReturnsOriginal(t *testing.T) {
	t.Parallel()

	garbage := "not-a-timestamp"
	got := FormatRelativeTime(garbage)
	if got != garbage {
		t.Errorf("FormatRelativeTime(%q) = %q, want original string", garbage, got)
	}
}

func TestFormatRelativeTime_ValidRFC3339(t *testing.T) {
	t.Parallel()

	// A timestamp from 2 hours ago should produce an "h ago" string.
	ts := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	got := FormatRelativeTime(ts)
	if !strings.HasSuffix(got, "h ago") {
		t.Errorf("FormatRelativeTime(2h ago RFC3339) = %q, want something like '2h ago'", got)
	}
}

func TestPrintJSONEnvelope_StructuredOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	payload := map[string]string{"project": "my-app", "status": "OK"}

	if err := PrintJSONEnvelope(&buf, "1", payload); err != nil {
		t.Fatalf("PrintJSONEnvelope returned error: %v", err)
	}

	var got struct {
		SchemaVersion string            `json:"schemaVersion"`
		Data          map[string]string `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v (raw: %q)", err, buf.String())
	}
	if got.SchemaVersion != "1" {
		t.Errorf("schemaVersion = %q, want %q", got.SchemaVersion, "1")
	}
	if got.Data["project"] != "my-app" {
		t.Errorf("data.project = %q, want %q", got.Data["project"], "my-app")
	}
}

func TestPrintJSONErrorEnvelope_WrappedError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	inner := errors.New("y")
	wrapped := fmt.Errorf("x: %w", inner)

	if err := PrintJSONErrorEnvelope(&buf, "1", wrapped); err != nil {
		t.Fatalf("PrintJSONErrorEnvelope returned error: %v", err)
	}

	var got struct {
		SchemaVersion string `json:"schemaVersion"`
		Error         struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v (raw: %q)", err, buf.String())
	}
	if got.Error.Message != "x: y" {
		t.Errorf("error.message = %q, want %q", got.Error.Message, "x: y")
	}
}
