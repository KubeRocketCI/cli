package root

import (
	"bytes"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

// Not parallel: NewCmdRoot binds its persistent flags into the global viper.
func TestRoot_ArgsValidation(t *testing.T) {
	cases := map[string]struct {
		argv string
		want string
	}{
		"NoArgs verb, flag swallowed a flag":    {"pipelinerun list --project --pr 53", "flag needs an argument: --project"},
		"NoArgs sibling, flag swallowed a flag": {"env list --deployment --cluster x", "flag needs an argument: --deployment"},
		"ExactArgs verb, flag swallowed a flag": {"pipelinerun get run-1 -o --timeout 5m", "flag needs an argument: --output"},
		"slice flag swallowed a flag":           {"sca findings p --severity --branch main", "flag needs an argument: --severity"},
		"stray argument":                        {"env list extra", `unknown command "extra" for "krci env list"`},
		"ExactArgs count":                       {"pipelinerun get", "requires a pipeline run name"},
		"unknown subcommand":                    {"nosuch", `unknown command "nosuch" for "krci"`},
		"help, flag swallowed a flag":           {"help --portal-url -x", "flag needs an argument: --portal-url"},
		"completion, flag swallowed a flag":     {"completion zsh --portal-url -x", "flag needs an argument: --portal-url"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cmd := NewCmdRoot(cmdtest.NewFactory(), "test", "", "")
			cmd.SetArgs(strings.Fields(tc.argv))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error %q, got %v", tc.want, err)
			}
		})
	}
}
