package iostreams

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/output"
)

// policy has one row per rule of the terminalWriter policy.
var policy = []struct {
	name, in, want string
}{
	{"text passes", "hello, світ\n", "hello, світ\n"},
	{"tab passes", "a\tb", "a\tb"},
	{"crlf is one newline", "a\r\nb", "a\nb"},
	{"lone cr is a newline", "10%\r20%", "10%\n20%"},
	{"other C0 and DEL are dropped", "a\x00\x07\x08\x0b\x0c\x7fb", "ab"},
	{"C1 runes are dropped", "a\u009b2J\u009d0;t\u0085b", "a2J0;tb"},
	{"raw 8-bit C1 becomes U+FFFD", "a\x9b2Jb", "a\ufffd2Jb"},
	{"invalid UTF-8 becomes U+FFFD", "a\xffb", "a\ufffdb"},
	{"incomplete rune at the end is dropped", "ab\xc3", "ab"},

	{"SGR bold and 256-colour pass", "\x1b[1;38;5;245mX\x1b[m", "\x1b[1;38;5;245mX\x1b[m"},
	{"SGR truecolour passes, 0 becomes the plain reset", "\x1b[38;2;1;2;3mX\x1b[0m", "\x1b[38;2;1;2;3mX\x1b[m"},
	{"SGR 16 colours pass", "\x1b[32mok\x1b[97m \x1b[39m", "\x1b[32mok\x1b[97m \x1b[39m"},
	{"SGR ending in reset is a reset", "\x1b[1;mX", "\x1b[mX"},
	{"SGR underline with colour passes", "\x1b[1;4;31mX\x1b[22;24;39m", "\x1b[1;4;31mX\x1b[22;24;39m"},
	{"SGR reverse is dropped", "\x1b[7mX", "X"},
	{"SGR conceal is dropped", "\x1b[8mX", "X"},
	{"SGR blink is dropped", "\x1b[5mX", "X"},
	{"SGR background is dropped", "\x1b[41mX\x1b[48;5;1mY", "XY"},
	{"SGR colon parameters are dropped", "\x1b[38:5:1mX", "X"},
	{"SGR with too many parameters is dropped", "\x1b[1;1;1;1;1;1;1;1;1mX", "X"},
	{"SGR 38 without arguments is dropped", "\x1b[38mX\x1b[38;5;300mY", "XY"},
	{"SGR signed parameter is dropped", "\x1b[+1mX", "X"},

	{"CSI clear screen is dropped", "a\x1b[2Jb", "ab"},
	{"CSI cursor, private and report are dropped", "\x1b[H\x1b[?25l\x1b[21tX", "X"},
	{"CSI over the limit is dropped whole", "\x1b[" + strings.Repeat("0", maxCSILen) + "1mX", "X"},
	{"CAN aborts a CSI", "\x1b[2\x18JX", "JX"},

	{"OSC title with BEL is dropped", "a\x1b]0;pwned\x07b", "ab"},
	{"OSC title with ST is dropped", "a\x1b]2;pwned\x1b\\b", "ab"},
	{"OSC 52 clipboard is dropped", "\x1b]52;c;aGk=\x07X", "X"},
	{"OSC aborts at newline and keeps the line", "\x1b]0;title\nnext", "\nnext"},
	{"OSC aborts at a high byte", "\x1b]8;;http://h/\xc9\x9c\x07x", "ɜx"},
	{"SUB aborts an OSC", "\x1b]0;t\x1aX", "X"},

	{"OSC 8 http link passes with ST and no params",
		"\x1b]8;id=1;https://example.com/x\x07T\x1b]8;;\x07",
		"\x1b]8;;https://example.com/x\x1b\\T\x1b]8;;\x1b\\"},
	{"OSC 8 uppercase scheme passes", "\x1b]8;;HTTP://h/\x1b\\x\x1b]8;;\x1b\\", "\x1b]8;;HTTP://h/\x1b\\x\x1b]8;;\x1b\\"},
	{"OSC 8 javascript is dropped", "\x1b]8;;javascript:alert(1)\x1b\\click\x1b]8;;\x1b\\", "click"},
	{"OSC 8 file is dropped", "\x1b]8;;file:///etc/passwd\x07x\x1b]8;;\x07", "x"},
	{"OSC 8 without host is dropped", "\x1b]8;;http:///p\x07x\x1b]8;;\x07", "x"},
	{"OSC 8 with a space is encoded", "\x1b]8;;http://h/a b\x07x", "\x1b]8;;http://h/a%20b\x1b\\x"},
	{"SGR reset with an off code passes", "\x1b[0;27mX", "\x1b[0;27mX"},
	{"OSC 8 over the limit is dropped whole", "\x1b]8;;http://h/" + strings.Repeat("a", maxOSCLen) + "\x07x", "x"},
	{"OSC 8 ends at a BEL inside the URL", "\x1b]8;;http://h/\x07\x07x", "\x1b]8;;http://h/\x1b\\x"},
	{"stray link close is dropped", "x\x1b]8;;\x07y", "xy"},

	{"DCS is dropped", "\x1bP$q\"p\x1b\\X", "X"},
	{"APC is dropped", "\x1b_Gf=1\x1b\\X", "X"},
	{"string ends at BEL", "\x1bPdata\x07X", "X"},
	{"ESC two-character sequences are dropped", "\x1bc\x1b(BX", "X"},
	{"ESC before a control drops only the ESC", "\x1b\nX", "\nX"},
	{"ESC inside a string starts a new sequence", "\x1bPdata\x1b[2JX", "X"},
	{"bare ESC at the end is dropped", "a\x1b", "a"},
	{"unterminated OSC at the end is dropped", "a\x1b]0;title", "a"},

	{"open style is closed at newline", "\x1b[31mred\nplain", "\x1b[31mred\x1b[m\nplain"},
	{"open link is closed at newline", "\x1b]8;;http://h/\x07t\nplain", "\x1b]8;;http://h/\x1b\\t\x1b]8;;\x1b\\\nplain"},
	{"reset before newline leaves nothing open", "\x1b[31mred\x1b[m\nplain", "\x1b[31mred\x1b[m\nplain"},
	{"cr before newline closes the style once", "\x1b[31mred\r\nplain", "\x1b[31mred\x1b[m\nplain"},
}

func sanitize(t *testing.T, chunks ...string) string {
	t.Helper()

	var buf bytes.Buffer

	w := newTerminalWriter(&buf)

	for _, c := range chunks {
		n, err := w.Write([]byte(c))
		if err != nil || n != len(c) {
			t.Fatalf("Write(%q) = %d, %v; want %d, nil", c, n, err, len(c))
		}
	}

	return buf.String()
}

func TestTerminalWriter_Policy(t *testing.T) {
	t.Parallel()

	for _, tc := range policy {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := sanitize(t, tc.in); got != tc.want {
				t.Errorf("one write:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// Every row gives the same output when split at any byte and when written one
// byte at a time.
func TestTerminalWriter_SplitWrites(t *testing.T) {
	t.Parallel()

	for _, tc := range policy {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for i := 1; i < len(tc.in); i++ {
				if got := sanitize(t, tc.in[:i], tc.in[i:]); got != tc.want {
					t.Fatalf("split at %d:\n got %q\nwant %q", i, got, tc.want)
				}
			}

			chunks := make([]string, 0, len(tc.in))
			for i := range len(tc.in) {
				chunks = append(chunks, tc.in[i:i+1])
			}

			if got := sanitize(t, chunks...); got != tc.want {
				t.Errorf("byte at a time:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestTerminalWriter_ForwardsWriteError(t *testing.T) {
	t.Parallel()

	w := newTerminalWriter(failingWriter{})

	if _, err := w.Write([]byte("x")); err == nil {
		t.Fatal("Write returned nil, want the next stage's error")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// styled is our own output: a bold 256-colour label, a green value and a link.
const styled = "\x1b[1;38;5;245mLabel:\x1b[m \x1b[32mok\x1b[m \x1b]8;;http://h/\x07t\x1b]8;;\x07\n"

func TestFromWriters_Profiles(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"a buffer is piped: no sequence survives",
			map[string]string{"TERM": "xterm-256color"},
			"Label: ok t\n"},
		{"CLICOLOR_FORCE keeps colour when piped",
			map[string]string{"TERM": "xterm-256color", "CLICOLOR_FORCE": "1", "COLORTERM": "truecolor"},
			styledST},
		{"NO_COLOR on a terminal keeps bold and links, drops colour",
			map[string]string{"TERM": "xterm-256color", "TTY_FORCE": "1", "NO_COLOR": "1"},
			"\x1b[1mLabel:\x1b[m \x1b[mok\x1b[m \x1b]8;;http://h/\x1b\\t\x1b]8;;\x1b\\\n"},
		{"a dumb terminal gets plain text",
			map[string]string{"TERM": "dumb", "TTY_FORCE": "1"},
			"Label: ok t\n"},
		{"a 16-colour terminal gets downsampled colour",
			map[string]string{"TERM": "xterm", "TTY_FORCE": "1"},
			"\x1b[1;37mLabel:\x1b[m \x1b[32mok\x1b[m \x1b]8;;http://h/\x1b\\t\x1b]8;;\x1b\\\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			environ := make([]string, 0, len(tc.env))
			for k, v := range tc.env {
				environ = append(environ, k+"="+v)
			}

			var out, errOut bytes.Buffer

			ios := fromWriters(nil, &out, &errOut, environ)
			if ios.IsStdoutTTY() {
				t.Fatal("IsStdoutTTY() = true for a buffer")
			}

			for _, w := range []struct {
				name string
				w    io.Writer
				buf  *bytes.Buffer
			}{{"Out", ios.Out, &out}, {"ErrOut", ios.ErrOut, &errOut}} {
				if _, err := w.w.Write([]byte(styled + "\x1b]0;pwned\x07")); err != nil {
					t.Fatal(err)
				}

				if got := w.buf.String(); got != tc.want {
					t.Errorf("%s:\n got %q\nwant %q", w.name, got, tc.want)
				}
			}
		})
	}
}

// styledST is styled as the terminalWriter re-emits it: links end with ST.
var styledST = strings.ReplaceAll(styled, "\x07", "\x1b\\")

// The styles output renders with pass the stream unchanged, apart from the
// link terminator, which the stream rewrites to ST.
func TestFromWriters_KeepsOutputStyles(t *testing.T) {
	t.Parallel()

	in := output.Hyperlink("t", "https://h/p?q=1").Render(true) + output.Hyperlink("t", "https://h/p?q=1#f").Render(true) +
		output.HeaderStyle.Render("h") +
		output.LabelStyle.Render("l") + output.DimStyle.Render("d") +
		output.EvenRowStyle.Render("e") + output.OddRowStyle.Render("o")
	for _, c := range []output.Cell{
		output.Green("g"), output.Yellow("y"), output.Red("r"), output.Blue("b"), output.Gray("x"),
	} {
		in += c.Render(true)
	}
	want := strings.ReplaceAll(in, "\x07", "\x1b\\")

	var out bytes.Buffer

	ios := fromWriters(nil, &out, &bytes.Buffer{}, []string{"TERM=xterm-256color", "COLORTERM=truecolor", "CLICOLOR_FORCE=1"})
	if _, err := ios.Out.Write([]byte(in)); err != nil {
		t.Fatal(err)
	}

	if out.String() != want {
		t.Errorf("stream changed output's own styles:\n got %q\nwant %q", out.String(), want)
	}
}
