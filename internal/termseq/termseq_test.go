package termseq

import "testing"

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

	cases := map[string]bool{
		"http://h/":           true,
		"HTTPS://H/p?q=1#f":   true,
		"":                    false,
		"javascript:alert(1)": false,
		"file:///etc/passwd":  false,
		"http:///p":           false,
		"http://h/a b":        false,
		"http://h/\n":         false,
		"http://h/\u202e":     false,
		"http://h/\x7f":       false,
		"//h/":                false,
		"h/":                  false,
	}

	for target, want := range cases {
		if got := Link(target); got != want {
			t.Errorf("Link(%q) = %v, want %v", target, got, want)
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
		"8;;javascript:alert(1)": {target: "javascript:alert(1)"},
		"0;title":                {target: ""},
		"8;;http://h/\n":         {target: ""},
		"52;c;aGk=":              {target: ""},
	}

	for payload, tc := range cases {
		target, ok := OSC8(payload)
		if target != tc.target || ok != tc.ok {
			t.Errorf("OSC8(%q) = %q, %v; want %q, %v", payload, target, ok, tc.target, tc.ok)
		}
	}
}
