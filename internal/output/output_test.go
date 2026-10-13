package output

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/KubeRocketCI/cli/internal/portal"
)

// sgr matches the SGR sequences Style.Render emits; tests strip them before
// comparing bytes.
var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestSonarGateStatusColor_PassthroughWhenUnknown(t *testing.T) {
	t.Parallel()

	// Unknown statuses must pass through without ANSI codes.
	got := SonarGateStatusColor(portal.QualityGateStatus("UNKNOWN_STATUS"))
	if got != Text("UNKNOWN_STATUS") {
		t.Errorf("SonarGateStatusColor returned %q, want %q", got, "UNKNOWN_STATUS")
	}
}

func TestSonarGateStatusColor_RecognizesKnownStatuses(t *testing.T) {
	t.Parallel()

	// For known statuses, the returned string should at least still contain the status text.
	for _, s := range []portal.QualityGateStatus{portal.QualityGateOK, portal.QualityGateWarn, portal.QualityGateError, portal.QualityGateNone} {
		got := SonarGateStatusColor(s).Render(true)
		if !strings.Contains(got, string(s)) {
			t.Errorf("SonarGateStatusColor(%q) returned %q, missing status text", s, got)
		}
	}
}

func TestSonarGateStatusColor_EmptyStatusReturnsEmpty(t *testing.T) {
	t.Parallel()

	// An empty QualityGateStatus hits the default branch and must return "".
	got := SonarGateStatusColor(portal.QualityGateStatus("")).Render(true)
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
		"control characters become visible": {
			in:   "boom \x1b]0;title\x07\x1b[2J done\u009b31m\x7f",
			want: "boom ^[]0;title^G^[[2J done^[[31m^?",
		},
		"bidi overrides drop, separators break": {in: "a\u202eb\u2066c\u2028d", want: "abc d"},
		"invalid UTF-8 becomes U+FFFD":          {in: "a\xffb", want: "a\ufffdb"},
		"text beyond ASCII stays":               {in: "помилка —  кінець", want: "помилка — кінець"},
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
	rows := [][]Cell{Texts("foo", "OK"), {Text("bar"), Red("ERROR")}}

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

	if err := PrintTable(&buf, true, []string{"NAME", "STATUS"}, [][]Cell{{Text("foo"), Green("OK")}}); err != nil {
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
		{Label: "Status", Value: "created", Styled: Green("[created-styled]")},
		{Label: "Available", Value: "true", Styled: AvailableText(true)},
	}

	cases := []struct {
		name  string
		isTTY bool
		want  string
	}{
		{"plain pads the label to 14 columns and ignores Styled", false,
			"Name:         foo\nStatus:       created\nAvailable:    true\n"},
		{"styled pads through LabelStyle and prefers a non-zero Styled", true,
			"Name:          foo\nStatus:        [created-styled]\nAvailable:     Yes\n"},
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
		if err := PrintAligned(&buf, isTTY, nil, "  ", 0); err != nil {
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
		{Label: "A longer label", Value: "3", Styled: Concat(Text("3 "), Red("CRITICAL"))},
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
			want:   "  Short            val1\n  A longer label   3\n",
		},
		{
			name:  "styled renders labels and prefers Styled",
			isTTY: true,
			want: alignedLabelStyle.Render("Short") + "            val1\n" +
				alignedLabelStyle.Render("A longer label") + "   3 " + redStyle.Render("CRITICAL") + "\n",
		},
		{
			name:   "styled keeps the indent outside the label style",
			isTTY:  true,
			indent: "  ",
			want: "  " + alignedLabelStyle.Render("Short") + "            val1\n" +
				"  " + alignedLabelStyle.Render("A longer label") + "   3 " + redStyle.Render("CRITICAL") + "\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			if err := PrintAligned(&buf, tc.isTTY, lines, tc.indent, 0); err != nil {
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

func TestPrintJSON_EscapesC1AndDEL(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := PrintJSON(&buf, map[string]string{"m": "a\u009bb\x7fc\x1bdé"}); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}

	want := "{\n  \"m\": \"a\\u009bb\\u007fc\\u001bdé\"\n}\n"
	if got := buf.String(); got != want {
		t.Errorf("PrintJSON = %q, want %q", got, want)
	}

	var back map[string]string
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil || back["m"] != "a\u009bb\x7fc\x1bdé" {
		t.Errorf("round trip = %q, %v", back["m"], err)
	}
}

func TestPrintJSON_PlainBytesUnchanged(t *testing.T) {
	t.Parallel()

	var got, want bytes.Buffer
	if err := PrintJSON(&got, []string{"plain", "текст"}); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}

	enc := json.NewEncoder(&want)
	enc.SetIndent("", "  ")
	_ = enc.Encode([]string{"plain", "текст"})

	if got.String() != want.String() {
		t.Errorf("PrintJSON = %q, want %q", got.String(), want.String())
	}
}

func TestPrintable(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ in, want string }{
		"plain is unchanged":          {in: "plain текст\n\tok\r", want: "plain текст\n\tok\r"},
		"C0 in caret notation":        {in: "\x00\x07\x1b[2J", want: "^@^G^[[2J"},
		"DEL":                         {in: "a\x7fb", want: "a^?b"},
		"C1 as ESC plus letter":       {in: "\u009b31m\u009d0;t\u0085", want: "^[[31m^[]0;t^[E"},
		"bidi drop, separators break": {in: "a\u202a\u202e\u2066\u2069b\u2028c\u2029d", want: "ab\nc\nd"},
		"invalid UTF-8":               {in: "a\xff\xc3b", want: "a\ufffd\ufffdb"},
	}

	for name, tc := range cases {
		if got := printable(tc.in); got != tc.want {
			t.Errorf("%s: printable = %q, want %q", name, got, tc.want)
		}
	}
}

func TestPrintTable_HostileCells(t *testing.T) {
	t.Parallel()

	headers := []string{"NAME\nX"}
	rows := [][]Cell{{Text("a\x1b[2J\nb\tc")}}

	var plain bytes.Buffer
	if err := PrintTable(&plain, false, headers, rows); err != nil {
		t.Fatal(err)
	}

	if want := "NAME X\na^[[2J b c\n"; plain.String() != want {
		t.Errorf("plain = %q, want %q", plain.String(), want)
	}

	// Every control that lipgloss measures as zero width but a terminal acts
	// on, inside and outside the CLI's own sequences, link payloads that carry
	// a line break, and cluster text holding sequences the stream keeps.
	link := Hyperlink("t", "http://h/")
	hostile := [][]Cell{
		{Concat(Text("legit\f\f\fpay\x7f\v\x1b[2J\u202ements "), Green("ok\fnot"), Text(" "), link)},
		{Text("x\x1b]8;\nFORGED\x07")},
		{Text("x\x1b]8;;http://h/\rFORGED\x07")},
		{Hyperlink("a", "http://a/\nb")},
		{Concat(Text("a\nb\t"), Green("ok"))},
		{Text(clusterSeqs)},
		{Green("\x1b[31mx")},
	}
	visible := [][]Cell{
		{Concat(Text("legit^L^L^Lpay^?^K^[[2Jments "), Green("ok^Lnot"), Text(" "), link)},
		{Text("x^[]8; FORGED^G")},
		{Text("x^[]8;;http://h/ FORGED^G")},
		{Text("a")},
		{Concat(Text("a b "), Green("ok"))},
		{Text(clusterSeqsVisible)},
		{Green("^[[31mx")},
	}

	var styled bytes.Buffer
	if err := PrintTable(&styled, true, headers, hostile); err != nil {
		t.Fatal(err)
	}

	var clean bytes.Buffer
	if err := PrintTable(&clean, true, []string{"NAME X"}, visible); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{greenStyle.Render("ok^Lnot"), link.Render(true), "x^[]8; FORGED^G", clusterSeqsVisible} {
		if !strings.Contains(styled.String(), want) {
			t.Errorf("styled table is missing %q:\n%q", want, styled.String())
		}
	}

	if got, want := strings.Count(styled.String(), "\n"), strings.Count(clean.String(), "\n"); got != want {
		t.Errorf("styled table has %d lines, want %d:\n%q", got, want, styled.String())
	}

	if styled.String() != clean.String() {
		t.Errorf("styled = %q, want the render of the visible input %q", styled.String(), clean.String())
	}
}

// clusterSeqs holds an SGR sequence and an OSC 8 link that termseq accepts;
// as cluster text they print in caret notation.
const (
	clusterSeqs        = "\x1b[31mred\x1b[m \x1b]8;;http://h/\x1b\\t\x1b]8;;\x1b\\"
	clusterSeqsVisible = "^[[31mred^[[m ^[]8;;http://h/^[\\t^[]8;;^[\\"
)

func TestPrintDetailLines_HostileValues(t *testing.T) {
	t.Parallel()

	lines := []DetailLine{
		{Label: "Na\nme", Value: "v\x1b]0;t\x07\nw", Styled: Concat(Green("x\fy"), Text("\nz"))},
		{Label: "Seq", Value: clusterSeqs},
		{Label: "Styled", Value: "v", Styled: Green(clusterSeqs)},
	}

	var plain bytes.Buffer
	if err := PrintDetailLines(&plain, false, lines); err != nil {
		t.Fatal(err)
	}

	if want := "Na me:        v^[]0;t^G w\nSeq:          " + clusterSeqsVisible + "\nStyled:       v\n"; plain.String() != want {
		t.Errorf("plain = %q, want %q", plain.String(), want)
	}

	var styled bytes.Buffer
	if err := PrintDetailLines(&styled, true, lines); err != nil {
		t.Fatal(err)
	}

	want := "Na me:         x^Ly z\nSeq:           " + clusterSeqsVisible + "\nStyled:        " + clusterSeqsVisible + "\n"
	if got := sgr.ReplaceAllString(styled.String(), ""); got != want {
		t.Errorf("styled = %q, want %q", got, want)
	}
}

func TestPrintAligned_HostileValues(t *testing.T) {
	t.Parallel()

	lines := []DetailLine{
		{Label: "Seq", Value: clusterSeqs},
		{Label: "Styled", Value: "v", Styled: Green(clusterSeqs)},
	}

	var plain bytes.Buffer
	if err := PrintAligned(&plain, false, lines, "", 0); err != nil {
		t.Fatal(err)
	}

	if want := "Seq      " + clusterSeqsVisible + "\nStyled   v\n"; plain.String() != want {
		t.Errorf("plain = %q, want %q", plain.String(), want)
	}

	var styled bytes.Buffer
	if err := PrintAligned(&styled, true, lines, "", 0); err != nil {
		t.Fatal(err)
	}

	want := "Seq      " + clusterSeqsVisible + "\nStyled   " + clusterSeqsVisible + "\n"
	if got := sgr.ReplaceAllString(styled.String(), ""); got != want {
		t.Errorf("styled = %q, want %q", got, want)
	}
}

func TestPrintAligned_MinimumWidth(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := PrintAligned(&buf, false, []DetailLine{{Label: "Ab:", Value: "v\nw"}}, "", 6); err != nil {
		t.Fatal(err)
	}

	if want := "Ab:      v w\n"; buf.String() != want {
		t.Errorf("PrintAligned = %q, want %q", buf.String(), want)
	}
}

func TestStatusColor_UnknownIsPrintable(t *testing.T) {
	t.Parallel()

	if got := StatusColor("odd\x1b[2J\nstate"); got != Text("odd^[[2J state") {
		t.Errorf("StatusColor = %#v", got)
	}
}

func TestLabelWidth(t *testing.T) {
	t.Parallel()

	got := LabelWidth([]DetailLine{{Label: "ab"}}, []DetailLine{{Label: "a\tbc"}, {Label: "Опис"}})
	if got != 4 {
		t.Errorf("LabelWidth = %d, want 4", got)
	}
}

func TestCell(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		c             Cell
		plain, styled string
	}{
		"text":                       {Text("a\nb"), "a b", "a b"},
		"green":                      {Green("ok"), "ok", greenStyle.Render("ok")},
		"a tab is one space":         {Green("a\tb"), "a b", greenStyle.Render("a b")},
		"CRLF is two spaces":         {Green("a\r\nb"), "a  b", greenStyle.Render("a  b")},
		"concat joins form by form":  {Concat(Text("3 "), Red("HIGH")), "3 HIGH", "3 " + redStyle.Render("HIGH")},
		"texts":                      {Concat(Texts("a", "\tb")...), "a b", "a b"},
		"an unknown status is plain": {PipelineStatusColor("odd"), "odd", "odd"},
		"empty green is zero":        {Green(""), "", ""},
		"empty link is zero":         {Hyperlink("", "http://h/"), "", ""},
	}

	for name, tc := range cases {
		if got := tc.c.Render(false); got != tc.plain {
			t.Errorf("%s: Render(false) = %q, want %q", name, got, tc.plain)
		}

		if got := tc.c.Render(true); got != tc.styled {
			t.Errorf("%s: Render(true) = %q, want %q", name, got, tc.styled)
		}
	}

	if !(Cell{}).IsZero() || !Green("").IsZero() || !Hyperlink("", "http://h/").IsZero() || Text("x").IsZero() {
		t.Error("IsZero: want true for empty text only")
	}
}

func TestHyperlink(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ text, target, want string }{
		"http link":              {"t", "http://h/", lipgloss.NewStyle().Hyperlink("http://h/").Render("t")},
		"no target":              {"t", "", "t"},
		"javascript":             {"t", "javascript:alert(1)", "t"},
		"no host":                {"t", "http:///p", "t"},
		"newline in target":      {"t", "http://h/\nb", "t"},
		"text stays on one row":  {"a\nb", "http://h/", lipgloss.NewStyle().Hyperlink("http://h/").Render("a b")},
		"unicode path encoded":   {"t", "https://h/путь", lipgloss.NewStyle().Hyperlink("https://h/%D0%BF%D1%83%D1%82%D1%8C").Render("t")},
		"unicode host":           {"t", "https://münchen.de/", "t"},
		"encoded over the limit": {"PR", "https://example.com/" + strings.Repeat("п", 400), "PR"},
	}

	for name, tc := range cases {
		c := Hyperlink(tc.text, tc.target)
		if got := c.Render(true); got != tc.want {
			t.Errorf("%s: Hyperlink = %q, want %q", name, got, tc.want)
		}

		if got := c.Render(false); got != cell(tc.text) {
			t.Errorf("%s: plain Hyperlink = %q, want %q", name, got, cell(tc.text))
		}
	}
}
