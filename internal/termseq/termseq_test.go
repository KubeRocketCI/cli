package termseq

import (
	"strings"
	"testing"
)

func TestSGR(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		params string
		canon  string
		open   bool
		ok     bool
	}{
		"empty is reset":             {params: "", canon: Reset, ok: true},
		"0 is reset":                 {params: "0", canon: Reset, ok: true},
		"bold and 256-colour":        {params: "1;38;5;245", canon: "\x1b[1;38;5;245m", open: true, ok: true},
		"truecolour":                 {params: "38;2;1;2;3", canon: "\x1b[38;2;1;2;3m", open: true, ok: true},
		"16 colours":                 {params: "32", canon: "\x1b[32m", open: true, ok: true},
		"bright":                     {params: "97", canon: "\x1b[97m", open: true, ok: true},
		"default foreground":         {params: "39", canon: "\x1b[39m", open: true, ok: true},
		"ending in reset is reset":   {params: "1;", canon: Reset, ok: true},
		"reset then bold is open":    {params: "0;1", canon: "\x1b[0;1m", open: true, ok: true},
		"underline and bold off":     {params: "1;4;31", canon: "\x1b[1;4;31m", open: true, ok: true},
		"bold off alone":             {params: "22", canon: "\x1b[22m", open: true, ok: true},
		"default background":         {params: "49", canon: "\x1b[49m", open: true, ok: true},
		"reset then reverse off":     {params: "0;27", canon: "\x1b[0;27m", open: true, ok: true},
		"double underline":           {params: "21", canon: "\x1b[21m", open: true, ok: true},
		"over the byte limit":        {params: strings.Repeat("1;", 32) + "1"},
		"index above 255":            {params: "38;5;999"},
		"component above 255":        {params: "38;2;1;2;300"},
		"too many fields":            {params: "1;1;1;1;1;1;1;1;1"},
		"colon sub-parameters":       {params: "38:5:1"},
		"signed":                     {params: "+1"},
		"reverse":                    {params: "7"},
		"conceal":                    {params: "8"},
		"blink":                      {params: "5"},
		"background":                 {params: "41"},
		"38 without arguments":       {params: "38"},
		"38 with the wrong selector": {params: "38;3;1"},
	}

	for name, tc := range cases {
		canon, open, ok := SGR(tc.params)
		if canon != tc.canon || open != tc.open || ok != tc.ok {
			t.Errorf("%s: SGR(%q) = %q, %v, %v; want %q, %v, %v", name, tc.params, canon, open, ok, tc.canon, tc.open, tc.ok)
		}
	}
}

func TestLink(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		encoded string
		ok      bool
	}{
		"http://h/":                                      {encoded: "http://h/", ok: true},
		"HTTPS://H/p?q=1#f":                              {encoded: "HTTPS://H/p?q=1#f", ok: true},
		"https://h/путь?q=я#я":                           {encoded: "https://h/%D0%BF%D1%83%D1%82%D1%8C?q=%D1%8F#%D1%8F", ok: true},
		"http://h/a b":                                   {encoded: "http://h/a%20b", ok: true},
		"http://h/\u202e":                                {encoded: "http://h/%E2%80%AE", ok: true},
		"https://münchen.de/":                            {},
		"https://h/" + strings.Repeat("п", 400):          {},
		"https://h/" + strings.Repeat("a", MaxOSCLen-13): {encoded: "https://h/" + strings.Repeat("a", MaxOSCLen-13), ok: true},
		"":                    {},
		"javascript:alert(1)": {},
		"file:///etc/passwd":  {},
		"http:///p":           {},
		"http://h h/":         {},
		"http://h/\n":         {},
		"http://h/\x7f":       {},
		"//h/":                {},
		"h/":                  {},
	}

	for target, want := range cases {
		encoded, ok := Link(target)
		if encoded != want.encoded || ok != want.ok {
			t.Errorf("Link(%q) = %q, %v; want %q, %v", target, encoded, ok, want.encoded, want.ok)
		}
	}
}

func TestOSC8(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		target string
		ok     bool
	}{
		"8;;http://h/":           {target: "http://h/", ok: true},
		"8;id=1:x;http://h/":     {target: "http://h/", ok: true},
		"8;;":                    {target: "", ok: true},
		"8;x":                    {target: ""},
		"8;;javascript:alert(1)": {},
		"0;title":                {target: ""},
		"8;;http://h/\n":         {target: ""},
		"52;c;aGk=":              {target: ""},
		"8;;http://h/" + strings.Repeat("a", MaxOSCLen): {target: ""},
	}

	for payload, tc := range cases {
		target, ok := OSC8(payload)
		if target != tc.target || ok != tc.ok {
			t.Errorf("OSC8(%q) = %q, %v; want %q, %v", payload, target, ok, tc.target, tc.ok)
		}
	}
}
