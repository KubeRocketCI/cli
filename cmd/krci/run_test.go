package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
)

// Not parallel: configure initialises the global viper, and the tests set
// HOME and KRCI_* through t.Setenv.

// isolate points HOME at an empty directory, clears every KRCI_* variable and
// resets the global viper, so the run sees no configuration.
func isolate(t *testing.T) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())

	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "KRCI_") {
			t.Setenv(name, "")
		}
	}

	viper.Reset()
	t.Cleanup(viper.Reset)
}

type matcher struct {
	desc string
	ok   func(string) bool
}

func empty() matcher { return matcher{"empty", func(s string) bool { return s == "" }} }

func equal(want string) matcher {
	return matcher{"== " + want, func(s string) bool { return s == want }}
}

func prefix(p string) matcher {
	return matcher{"prefix " + p, func(s string) bool { return strings.HasPrefix(s, p) }}
}

func contains(p string) matcher {
	return matcher{"contains " + p, func(s string) bool { return strings.Contains(s, p) }}
}

func newFactory(out, errOut *bytes.Buffer) *cmdutil.Factory {
	return cmdutil.New(iostreams.New(nil, out, errOut, false))
}

type runCase struct {
	args       []string
	wantCode   int
	wantOut    matcher
	wantErrOut matcher
}

func (tc runCase) check(t *testing.T, code int, out, errOut string) {
	t.Helper()

	if code != tc.wantCode {
		t.Errorf("exit code = %d, want %d", code, tc.wantCode)
	}

	if !tc.wantOut.ok(out) {
		t.Errorf("stdout = %q, want %s", out, tc.wantOut.desc)
	}

	if !tc.wantErrOut.ok(errOut) {
		t.Errorf("stderr = %q, want %s", errOut, tc.wantErrOut.desc)
	}
}

func TestRunWith(t *testing.T) {
	cases := map[string]runCase{
		"version": {
			args:       []string{"version"},
			wantOut:    prefix("krci version "),
			wantErrOut: empty(),
		},
		"version flag": {
			args:       []string{"--version"},
			wantOut:    prefix("krci version "),
			wantErrOut: empty(),
		},
		"help flag": {
			args:       []string{"--help"},
			wantOut:    contains("Usage:"),
			wantErrOut: empty(),
		},
		"help command": {
			args:       []string{"help", "version"},
			wantOut:    contains("Usage:"),
			wantErrOut: empty(),
		},
		"shell completion": {
			args:       []string{"__complete", "help", ""},
			wantOut:    contains("pipelinerun"),
			wantErrOut: contains("Completion ended with directive"),
		},
		"unknown help topic": {
			args:       []string{"help", "nosuch"},
			wantOut:    empty(),
			wantErrOut: prefix("Unknown help topic"),
		},
		"unknown command": {
			args:       []string{"nosuch"},
			wantCode:   1,
			wantOut:    empty(),
			wantErrOut: equal("Error: unknown command \"nosuch\" for \"krci\"\n"),
		},
		"missing argument": {
			args:       []string{"pipelinerun", "get"},
			wantCode:   1,
			wantOut:    empty(),
			wantErrOut: prefix("Error: requires a pipeline run name"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			isolate(t)

			var out, errb bytes.Buffer

			code := runWith(context.Background(), tc.args, newFactory(&out, &errb))
			tc.check(t, code, out.String(), errb.String())
		})
	}
}

func TestRunWith_NilArgsIgnoreProcessArgs(t *testing.T) {
	isolate(t)

	procArgs := os.Args
	os.Args = []string{"krci", "nosuch"}

	t.Cleanup(func() { os.Args = procArgs })

	var out, errb bytes.Buffer

	code := runWith(context.Background(), nil, newFactory(&out, &errb))

	if code != 0 || !strings.Contains(out.String(), "Usage:") || errb.Len() != 0 {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q; want 0, root help, empty", code, out.String(), errb.String())
	}
}

func TestRun(t *testing.T) {
	cases := map[string]runCase{
		"success": {
			args:       []string{"version"},
			wantOut:    prefix("krci version "),
			wantErrOut: empty(),
		},
		"final error line": {
			args:       []string{"nosuch"},
			wantCode:   1,
			wantOut:    empty(),
			wantErrOut: equal("Error: unknown command \"nosuch\" for \"krci\"\n"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			isolate(t)

			var out, errb bytes.Buffer

			code := run(context.Background(), tc.args, &out, &errb)
			tc.check(t, code, out.String(), errb.String())
		})
	}
}
