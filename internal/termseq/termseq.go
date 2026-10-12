// Package termseq is the terminal sequence policy shared by the process
// streams and the output primitives: the SGR parameters and OSC 8 targets the
// CLI lets through to a terminal. Everything else is dropped by the stream
// and shown in caret notation by the primitives.
package termseq

import (
	"net/url"
	"strconv"
	"strings"
)

// Reset is the canonical SGR reset.
const Reset = "\x1b[m"

// Limits on one sequence. A longer one is rejected whole.
const (
	MaxSGRLen = 64   // bytes of SGR parameters
	MaxOSCLen = 2048 // bytes of an OSC payload
	maxParams = 8    // SGR parameter fields
)

const del = 0x7f

// SGR validates SGR parameters, the text between "ESC [" and "m", against the
// allow list: reset; bold, faint, italic, underline, double underline and
// strike-through; every off code 22-29, which can only remove styling;
// foreground 30-37, 90-97, 38;5;n, 38;2;r;g;b and 39; default background 49.
// At most maxParams fields and MaxSGRLen bytes. It returns the canonical
// sequence and whether it leaves a style open. A sequence whose net effect is
// a reset is returned as Reset.
func SGR(params string) (canon string, open, ok bool) {
	if params == "" {
		return Reset, false, true
	}

	if len(params) > MaxSGRLen {
		return "", false, false
	}

	fields := strings.Split(params, ";")
	if len(fields) > maxParams {
		return "", false, false
	}

	values := make([]int, 0, len(fields))

	for _, f := range fields {
		if f == "" {
			values = append(values, 0)

			continue
		}

		v, err := strconv.Atoi(f)
		if err != nil || strings.Trim(f, "0123456789") != "" {
			return "", false, false
		}

		values = append(values, v)
	}

	parts := make([]string, 0, len(values))

	for i := 0; i < len(values); i++ {
		v := values[i]

		switch {
		case v == 0:
			open = false
		case attribute(v) || off(v):
			open = true
		case v == 38:
			n := colorArgs(values[i+1:])
			if n == 0 {
				return "", false, false
			}

			parts = appendInts(parts, values[i:i+1+n])
			i += n
			open = true

			continue
		default:
			return "", false, false
		}

		parts = append(parts, strconv.Itoa(v))
	}

	if !open {
		return Reset, false, true
	}

	return "\x1b[" + strings.Join(parts, ";") + "m", true, true
}

// attribute reports an allowed SGR parameter that turns styling on.
func attribute(v int) bool {
	switch v {
	case 1, 2, 3, 4, 9, 21:
		return true
	}

	return (v >= 30 && v <= 37) || (v >= 90 && v <= 97)
}

// off reports an SGR parameter that only removes styling: the off codes
// 22-29 and the default foreground and background.
func off(v int) bool {
	return (v >= 22 && v <= 29) || v == 39 || v == 49
}

// colorArgs returns the number of parameters that follow 38: 2 for 5;n and 4
// for 2;r;g;b, or 0 when they are missing or out of range.
func colorArgs(rest []int) int {
	switch {
	case len(rest) >= 2 && rest[0] == 5 && rest[1] <= 255:
		return 2
	case len(rest) >= 4 && rest[0] == 2 && rest[1] <= 255 && rest[2] <= 255 && rest[3] <= 255:
		return 4
	default:
		return 0
	}
}

func appendInts(parts []string, values []int) []string {
	for _, v := range values {
		parts = append(parts, strconv.Itoa(v))
	}

	return parts
}

// Link returns target as an OSC 8 target: an http(s) URL with an ASCII host,
// with every byte outside 0x21-0x7E in the path, query and fragment
// percent-encoded. ok is false for an empty target, a control character, a
// missing or non-ASCII host, another scheme, or an encoded form that would
// put the OSC 8 payload over MaxOSCLen. A non-ASCII host is not encoded yet;
// it needs IDNA.
func Link(target string) (encoded string, ok bool) {
	if target == "" || strings.ContainsFunc(target, func(r rune) bool { return r < ' ' || r == del }) {
		return "", false
	}

	u, err := url.Parse(target)
	if err != nil || u.Host == "" || !printableASCII(u.Host) || strings.Contains(u.Host, " ") {
		return "", false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}

	encoded = percentEncode(target)
	if len("8;;")+len(encoded) > MaxOSCLen {
		return "", false
	}

	return encoded, true
}

// percentEncode replaces every byte outside 0x21-0x7E with its %XX form.
func percentEncode(s string) string {
	const hex = "0123456789ABCDEF"

	var b strings.Builder

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c > ' ' && c < del {
			b.WriteByte(c)

			continue
		}

		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0xf])
	}

	return b.String()
}

// OSC8 reads an OSC 8 payload, the text between "ESC ]" and the terminator,
// and returns its target as Link encodes it. ok is false unless the payload
// is printable ASCII within MaxOSCLen of the form "8;params;target" and the
// target is empty, which closes a link, or passes Link. Params are ignored.
func OSC8(payload string) (target string, ok bool) {
	if len(payload) > MaxOSCLen || !printableASCII(payload) {
		return "", false
	}

	rest, ok := strings.CutPrefix(payload, "8;")
	if !ok {
		return "", false
	}

	_, target, ok = strings.Cut(rest, ";")
	if !ok {
		return "", false
	}

	if target == "" {
		return "", true
	}

	return Link(target)
}

func printableASCII(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r < ' ' || r >= del })
}
