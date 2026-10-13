package sonarinternal

import (
	"testing"

	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

func TestGateStatusColor(t *testing.T) {
	t.Parallel()

	cases := map[portal.QualityGateStatus]output.Cell{
		portal.QualityGateOK:    output.Green("OK"),
		portal.QualityGateWarn:  output.Yellow("WARN"),
		portal.QualityGateError: output.Red("ERROR"),
		portal.QualityGateNone:  output.Gray("NONE"),
		"ok":                    output.Text("ok"),
		"UNKNOWN\x1b[2J":        output.Text("UNKNOWN\x1b[2J"),
		"":                      {},
	}

	for status, want := range cases {
		if got := GateStatusColor(status); got != want {
			t.Errorf("GateStatusColor(%q) = %#v, want %#v", status, got, want)
		}
	}
}
