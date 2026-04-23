package sca

import (
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
)

func TestNewCmdSca(t *testing.T) {
	t.Parallel()

	f := &cmdutil.Factory{IOStreams: &iostreams.IOStreams{}}
	cmd := NewCmdSca(f)

	if cmd.Use != "sca" {
		t.Errorf("Use = %q", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short must not be empty")
	}
	if cmd.Long == "" {
		t.Error("Long must not be empty")
	}

	want := map[string]bool{"list": false, "get": false, "components": false, "findings": false}
	for _, sub := range cmd.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("missing subcommand %q", name)
		}
	}
	if got := len(cmd.Commands()); got != 4 {
		t.Errorf("want 4 subcommands, got %d", got)
	}
}
