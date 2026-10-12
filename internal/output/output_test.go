package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/KubeRocketCI/cli/internal/portal"
)

// sgr matches the colour codes lipgloss emits into a buffer when CLICOLOR_FORCE
// is set; tests strip them before comparing bytes.
var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

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

func TestSingleLine(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		in   string
		want string
	}{
		"whitespace runs": {
			in:   "  rpc error:\n\tcode = Unknown  desc = chart\r\nfailed \n",
			want: "rpc error: code = Unknown desc = chart failed",
		},
		"escape sequences lose their control characters": {
			in:   "boom \x1b]0;title\x07\x1b[2J done\u009b31m",
			want: "boom ]0;title[2J done31m",
		},
		"text beyond ASCII stays": {in: "помилка —  кінець", want: "помилка — кінець"},
	}

	for name, tc := range cases {
		if got := SingleLine(tc.in); got != tc.want {
			t.Errorf("%s: SingleLine = %q, want %q", name, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		in    string
		width int
		want  string
	}{
		"fits":                       {in: "short", width: 5, want: "short"},
		"no limit":                   {in: "anything goes", width: 0, want: "anything goes"},
		"cut with an ellipsis":       {in: "a long message", width: 9, want: "a long..."},
		"no room for an ellipsis":    {in: "abcdef", width: 3, want: "abc"},
		"counts characters":          {in: "помилка", width: 7, want: "помилка"},
		"cuts between characters":    {in: "помилка запуску", width: 10, want: "помилка..."},
		"a short width of non-ASCII": {in: "помилка", width: 2, want: "по"},
	}

	for name, tc := range cases {
		if got := Truncate(tc.in, tc.width); got != tc.want {
			t.Errorf("%s: Truncate = %q, want %q", name, got, tc.want)
		}
	}
}

func TestPrintTable_Plain(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	headers := []string{"NAME", "STATUS"}
	rows := [][]string{{"foo", "OK"}, {"bar", "ERROR"}}

	if err := PrintTable(&buf, false, headers, rows); err != nil {
		t.Fatalf("PrintTable(isTTY=false) returned error: %v", err)
	}

	want := "NAME   STATUS\nfoo    OK\nbar    ERROR\n"
	if got := buf.String(); got != want {
		t.Errorf("PrintTable(isTTY=false) = %q, want %q", got, want)
	}
}

func TestPrintTable_Styled(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	if err := PrintTable(&buf, true, []string{"NAME", "STATUS"}, [][]string{{"foo", "OK"}}); err != nil {
		t.Fatalf("PrintTable(isTTY=true) returned error: %v", err)
	}

	// The hidden border pads every row and adds a border line above and below.
	want := "                 \n  NAME   STATUS  \n                 \n  foo    OK      \n                 \n"
	if got := sgr.ReplaceAllString(buf.String(), ""); got != want {
		t.Errorf("PrintTable(isTTY=true) = %q, want %q", got, want)
	}
}

func TestPrintHeading(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		isTTY bool
		want  string
	}{
		{"plain appends a colon", false, "Reliability:\n"},
		{"styled uses HeaderStyle", true, HeaderStyle.Render("Reliability") + "\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			if err := PrintHeading(&buf, tc.isTTY, "Reliability"); err != nil {
				t.Fatalf("PrintHeading: %v", err)
			}

			if got := buf.String(); got != tc.want {
				t.Errorf("PrintHeading(isTTY=%v) = %q, want %q", tc.isTTY, got, tc.want)
			}
		})
	}
}

func TestPrintDetailLines(t *testing.T) {
	t.Parallel()

	lines := []DetailLine{
		{Label: "Name", Value: "foo"},
		{Label: "Status", Value: "created", Styled: "[created-styled]"},
	}

	cases := []struct {
		name  string
		isTTY bool
		want  string
	}{
		{"plain pads the label to 14 columns and ignores Styled", false,
			"Name:         foo\nStatus:       created\n"},
		{"styled pads through LabelStyle and prefers Styled", true,
			"Name:          foo\nStatus:        [created-styled]\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			if err := PrintDetailLines(&buf, tc.isTTY, lines); err != nil {
				t.Fatalf("PrintDetailLines: %v", err)
			}

			if got := sgr.ReplaceAllString(buf.String(), ""); got != tc.want {
				t.Errorf("PrintDetailLines(isTTY=%v) = %q, want %q", tc.isTTY, got, tc.want)
			}
		})
	}
}

func TestPrintAligned_Empty(t *testing.T) {
	t.Parallel()

	for _, isTTY := range []bool{false, true} {
		var buf bytes.Buffer
		if err := PrintAligned(&buf, isTTY, nil, "  "); err != nil {
			t.Fatalf("PrintAligned(isTTY=%v, nil): %v", isTTY, err)
		}

		if buf.Len() != 0 {
			t.Errorf("PrintAligned(isTTY=%v, nil) wrote %q, want nothing", isTTY, buf.String())
		}
	}
}

func TestPrintAligned(t *testing.T) {
	t.Parallel()

	lines := []DetailLine{
		{Label: "Short", Value: "val1"},
		{Label: "A longer label", Value: "OK", Styled: "[OK-styled]"},
	}

	cases := []struct {
		name   string
		isTTY  bool
		indent string
		want   string
	}{
		{
			name:   "plain pads to the longest label and ignores Styled",
			indent: "  ",
			want:   "  Short            val1\n  A longer label   OK\n",
		},
		{
			name:  "styled renders labels and prefers Styled",
			isTTY: true,
			want: alignedLabelStyle.Render("Short") + "            val1\n" +
				alignedLabelStyle.Render("A longer label") + "   [OK-styled]\n",
		},
		{
			name:   "styled keeps the indent outside the label style",
			isTTY:  true,
			indent: "  ",
			want: "  " + alignedLabelStyle.Render("Short") + "            val1\n" +
				"  " + alignedLabelStyle.Render("A longer label") + "   [OK-styled]\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			if err := PrintAligned(&buf, tc.isTTY, lines, tc.indent); err != nil {
				t.Fatalf("PrintAligned: %v", err)
			}

			if got := buf.String(); got != tc.want {
				t.Errorf("PrintAligned(isTTY=%v) = %q, want %q", tc.isTTY, got, tc.want)
			}
		})
	}
}

func TestPageCount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		total, pageSize, want int
	}{
		{0, 50, 1},
		{1, 50, 1},
		{50, 50, 1},
		{51, 50, 2},
		{100, 25, 4},
		{101, 25, 5},
		{0, 0, 1},
		{100, 0, 1},
		{100, -5, 1},
	}

	for _, tc := range cases {
		if got := PageCount(tc.total, tc.pageSize); got != tc.want {
			t.Errorf("PageCount(%d,%d) = %d, want %d", tc.total, tc.pageSize, got, tc.want)
		}
	}
}

func TestOrDash(t *testing.T) {
	t.Parallel()

	if got := OrDash(""); got != EmptyCell {
		t.Errorf("OrDash(\"\") = %q, want %q", got, EmptyCell)
	}

	if got := OrDash("x"); got != "x" {
		t.Errorf("OrDash(\"x\") = %q, want %q", got, "x")
	}
}
