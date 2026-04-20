package sonar

import (
	"bytes"
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

func newFactory() *cmdutil.Factory {
	return &cmdutil.Factory{
		IOStreams: &iostreams.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
		RestClient: func() (*restapi.ClientWithResponses, error) {
			return nil, nil
		},
	}
}

func TestNewCmdSonar_UseAndDescription(t *testing.T) {
	t.Parallel()

	cmd := NewCmdSonar(newFactory())

	if cmd.Use != "sonar" {
		t.Errorf("Use = %q, want %q", cmd.Use, "sonar")
	}

	if cmd.Short == "" {
		t.Error("Short must not be empty")
	}

	if cmd.Long == "" {
		t.Error("Long must not be empty")
	}
}

func TestNewCmdSonar_Subcommands(t *testing.T) {
	t.Parallel()

	cmd := NewCmdSonar(newFactory())

	want := []string{"list", "get", "gate", "issues"}

	subNames := make(map[string]bool, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}

	for _, name := range want {
		t.Run(name, func(t *testing.T) {
			if !subNames[name] {
				t.Errorf("subcommand %q not registered; registered: %v", name, subNames)
			}
		})
	}
}

func TestNewCmdSonar_SubcommandCount(t *testing.T) {
	t.Parallel()

	cmd := NewCmdSonar(newFactory())

	if got := len(cmd.Commands()); got != 4 {
		t.Errorf("expected 4 subcommands, got %d", got)
	}
}
