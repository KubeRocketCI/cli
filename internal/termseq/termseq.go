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

// maxParams bounds the parameters of one SGR sequence.
const maxParams = 8

const del = 0x7f

// SGR validates SGR parameters, the text between "ESC [" and "m", against the
// allow list: reset, bold, foreground 30-37 and 90-97, 38;5;n, 38;2;r;g;b and
// 39, at most maxParams fields. It returns the canonical sequence and whether
// it leaves a style open. A sequence whose net effect is a reset is returned
// as Reset.
func SGR(params string) (canon string, open, ok bool) {
	if params == "" {
		return Reset, false, true
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
		case v == 1 || v == 39 || (v >= 30 && v <= 37) || (v >= 90 && v <= 97):
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

// Link reports whether target is an http(s) URL with a host and printable
// ASCII without spaces.
func Link(target string) bool {
	if target == "" || !printableASCII(target) || strings.Contains(target, " ") {
		return false
	}

	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return false
	}

	return u.Scheme == "http" || u.Scheme == "https"
}

// OSC8 reads an OSC 8 payload, the text between "ESC ]" and the terminator,
// and returns its target. ok is false unless the payload is printable ASCII
// of the form "8;params;target" and the target is empty, which closes a link,
// or passes Link. Params are ignored.
func OSC8(payload string) (target string, ok bool) {
	if !printableASCII(payload) {
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

	return target, target == "" || Link(target)
}

func printableASCII(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r < ' ' || r >= del })
}
