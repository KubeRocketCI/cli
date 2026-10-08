package root

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/config"
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
		"stray argument, auth login":            {"auth login extra", `unknown command "extra" for "krci auth login"`},
		"stray argument, auth logout":           {"auth logout extra", `unknown command "extra" for "krci auth logout"`},
		"stray argument, auth status":           {"auth status extra", `unknown command "extra" for "krci auth status"`},
		"stray argument, project list":          {"project list extra", `unknown command "extra" for "krci project list"`},
		"stray argument, deployment list":       {"deployment list extra", `unknown command "extra" for "krci deployment list"`},
		"stray argument, version":               {"version extra", `unknown command "extra" for "krci version"`},
		"mistyped subcommand":                   {"project lst", `unknown command "lst" for "krci project"`},
		"mistyped subcommand, auth":             {"auth logot", `unknown command "logot" for "krci auth"`},
		"mistyped subcommand, completion":       {"completion zssh", `unknown command "zssh" for "krci completion"`},
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

// Config resolves only in commands that use it: help and version work while
// the config file is broken; a portal command reports the config error.
func TestRoot_ConfigErrorOnlyWhereUsed(t *testing.T) {
	cases := map[string]struct {
		argv    string
		wantOut string
		wantErr bool
	}{
		"bare group":       {argv: "project", wantOut: "Available Commands:"},
		"bare group, auth": {argv: "auth", wantOut: "Available Commands:"},
		"bare completion":  {argv: "completion", wantOut: "Available Commands:"},
		"help command":     {argv: "help project", wantOut: "Available Commands:"},
		"version":          {argv: "version", wantOut: "krci version test"},
		"portal command":   {argv: "project list", wantErr: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := cmdtest.NewFactory()
			f.Config = func() (*config.Config, error) {
				return nil, errors.New("loading config: parsing config: bad portal-url")
			}

			out := &bytes.Buffer{}
			f.IOStreams.Out = out
			cmd := NewCmdRoot(f, "test", "", "")
			cmd.SetArgs(strings.Fields(tc.argv))
			cmd.SetOut(out)
			cmd.SetErr(&bytes.Buffer{})

			err := cmd.Execute()
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "loading config") {
					t.Fatalf("Execute() = %v, want the config error", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Execute() = %v, want nil", err)
			}

			if !strings.Contains(out.String(), tc.wantOut) {
				t.Errorf("stdout = %q, want %q", out.String(), tc.wantOut)
			}
		})
	}
}
