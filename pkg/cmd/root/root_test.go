package root

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

func TestRoot_ArgsValidation(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
	t.Parallel()

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
			t.Parallel()

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

// SetOut sends cobra's command and flag deprecation notices to stdout
// (spf13/cobra#1708); redirectUnknownHelpTopic covers only the help-topic path.
func TestRoot_NoDeprecations(t *testing.T) {
	t.Parallel()

	if got := deprecations(NewCmdRoot(cmdtest.NewFactory(), "test", "", "")); len(got) > 0 {
		t.Errorf("deprecations = %v, want none: route their notices to stderr as redirectUnknownHelpTopic does", got)
	}
}

func TestDeprecations_FindsCommandsAndFlags(t *testing.T) {
	t.Parallel()

	root := NewCmdRoot(cmdtest.NewFactory(), "test", "", "")
	list, _, _ := root.Find([]string{"project", "list"})
	list.Deprecated = "use x"

	if err := root.PersistentFlags().MarkDeprecated("portal-url", "use x"); err != nil {
		t.Fatal(err)
	}

	if err := list.Flags().MarkShorthandDeprecated("output", "use --output"); err != nil {
		t.Fatal(err)
	}

	want := []string{"krci --portal-url", "krci project list", "krci project list -o"}
	if got := deprecations(root); !slices.Equal(got, want) {
		t.Errorf("deprecations = %v, want %v", got, want)
	}
}

// deprecations lists every command, flag and flag shorthand in the tree marked deprecated.
func deprecations(c *cobra.Command) []string {
	var found []string

	if c.Deprecated != "" {
		found = append(found, c.CommandPath())
	}

	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Deprecated != "" {
			found = append(found, c.CommandPath()+" --"+f.Name)
		}

		if f.ShorthandDeprecated != "" {
			found = append(found, c.CommandPath()+" -"+f.Shorthand)
		}
	})

	for _, sub := range c.Commands() {
		found = append(found, deprecations(sub)...)
	}

	return found
}
