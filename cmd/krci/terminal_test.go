package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hostileDescription is a CDPipeline description, as JSON text, that carries a
// title change, a javascript: link, a clear-screen, a C1 CSI and a newline.
const hostileDescription = `Demo\u001b]0;pwned\u0007\nFORGED: Status: created` +
	`\u001b]8;;javascript:alert(1)\u001b\\click\u001b]8;;\u001b\\ \u001b[2J \u009b2J tail`

// TestHostileDescription: cluster text reaches the terminal without any escape
// sequence or control character, through the real process streams.
func TestHostileDescription(t *testing.T) {
	tables, _ := fixtures(t)
	routes := tables["env"]

	for i := range routes {
		if routes[i].kind == kindCDPipeline {
			routes[i].body = strings.Replace(routes[i].body, `"deploymentType": "container"`,
				`"deploymentType": "container", "description": "`+hostileDescription+`"`, 1)
		}
	}

	res := runStub(t, routes, nil, "", []string{"deployment", "get", envDeployment})
	require.Equal(t, 0, res.code, res.stderr)

	got := res.stdout

	for _, bad := range []string{"\x1b", "\x07", "\x7f", "\u009b", "\nFORGED"} {
		assert.NotContains(t, got, bad)
	}

	assert.Contains(t, got, "Description:  Demo^[]0;pwned^G FORGED: Status: created"+
		"^[]8;;javascript:alert(1)^[\\click^[]8;;^[\\ ^[[2J ^[[2J tail\n")
}
