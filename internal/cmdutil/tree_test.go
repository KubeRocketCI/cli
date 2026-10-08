package cmdutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinalizeTree(t *testing.T) {
	t.Parallel()

	errCustom := errors.New("custom group ran")

	tests := []struct {
		name    string
		argv    string
		wantRun string
		wantOut string
		wantErr string
	}{
		{name: "leaf without Args runs", argv: "grp leaf", wantRun: "leaf"},
		{name: "leaf without Args rejects a stray argument", argv: "grp leaf extra", wantErr: `unknown command "extra" for "app grp leaf"`},
		{name: "declared Args kept", argv: "grp named x", wantRun: "named"},
		{name: "declared Args enforced", argv: "grp named", wantErr: "accepts 1 arg(s), received 0"},
		{name: "flag guard on a leaf", argv: "grp leaf --name --x", wantErr: "flag needs an argument: --name"},
		{name: "bare group prints help", argv: "grp", wantOut: "Available Commands:"},
		{name: "mistyped subcommand", argv: "grp lst", wantErr: `unknown command "lst" for "app grp"`},
		{name: "group RunE kept", argv: "custom", wantErr: errCustom.Error()},
		{name: "help takes a command path", argv: "help grp", wantOut: "Available Commands:"},
		{name: "root keeps cobra's unknown-command check", argv: "nosuch", wantErr: `unknown command "nosuch" for "app"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ran string

			record := func(name string) func(*cobra.Command, []string) error {
				return func(*cobra.Command, []string) error {
					ran = name
					return nil
				}
			}

			leaf := &cobra.Command{Use: "leaf", RunE: record("leaf")}
			leaf.Flags().String("name", "", "a string flag")

			grp := &cobra.Command{Use: "grp"}
			grp.AddCommand(leaf, &cobra.Command{Use: "named", Args: cobra.ExactArgs(1), RunE: record("named")})

			custom := &cobra.Command{Use: "custom", RunE: func(*cobra.Command, []string) error { return errCustom }}
			custom.AddCommand(&cobra.Command{Use: "sub", RunE: record("sub")})

			root := &cobra.Command{Use: "app", SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(grp, custom)
			root.InitDefaultHelpCmd()
			FinalizeTree(root)

			out := &strings.Builder{}
			root.SetOut(out)
			root.SetErr(&strings.Builder{})
			root.SetArgs(strings.Fields(tt.argv))

			err := root.Execute()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, ran, "run function must not be reached")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantRun, ran)
			assert.Contains(t, out.String(), tt.wantOut)
		})
	}
}
