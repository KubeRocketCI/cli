package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
)

// Not parallel: the tests set HOME and KRCI_* through t.Setenv.

// isolatedConfigDir points HOME and USERPROFILE at an empty directory and
// blanks every KRCI_* variable (viper ignores empty env values), so the run
// sees no configuration. It returns the config directory, which is not created.
func isolatedConfigDir(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "KRCI_") {
			t.Setenv(name, "")
		}
	}

	return config.DefaultConfigDir()
}

func writeConfigFile(t *testing.T, dir, content string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
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

func all(ms ...matcher) matcher {
	return matcher{
		desc: "all of " + describe(ms),
		ok: func(s string) bool {
			for _, m := range ms {
				if !m.ok(s) {
					return false
				}
			}

			return true
		},
	}
}

func describe(ms []matcher) string {
	descs := make([]string, 0, len(ms))
	for _, m := range ms {
		descs = append(descs, m.desc)
	}

	return strings.Join(descs, ", ")
}

func newFactory(out, errOut *bytes.Buffer) *cmdutil.Factory {
	return cmdutil.New(iostreams.New(nil, out, errOut, false))
}

// portalURLAfter runs args through runWith, requires exit code 0, and returns
// the PortalURL the run's Factory resolves.
func portalURLAfter(t *testing.T, args ...string) string {
	t.Helper()

	var out, errb bytes.Buffer

	f := newFactory(&out, &errb)

	if code := runWith(context.Background(), args, f); code != 0 {
		t.Fatalf("runWith(%v) = %d, stderr = %q", args, code, errb.String())
	}

	cfg, err := f.Config()
	if err != nil {
		t.Fatal(err)
	}

	return cfg.PortalURL
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
			isolatedConfigDir(t)

			var out, errb bytes.Buffer

			code := runWith(context.Background(), tc.args, newFactory(&out, &errb))
			tc.check(t, code, out.String(), errb.String())
		})
	}
}

func TestRunWith_NilArgsIgnoreProcessArgs(t *testing.T) {
	isolatedConfigDir(t)

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
			isolatedConfigDir(t)

			var out, errb bytes.Buffer

			code := run(context.Background(), tc.args, &out, &errb)
			tc.check(t, code, out.String(), errb.String())
		})
	}
}

func TestRunWith_ConfigReachesLeaves(t *testing.T) {
	cases := map[string]struct {
		args []string
		want error
	}{
		"portal URL gate": {
			args: []string{"project", "list"},
			want: cmdutil.ConfigNotSetError("portal URL", "", cmdutil.PortalURLOption, cmdutil.LoginHint),
		},
		"flag reaches the leaf, cluster name gate": {
			args: []string{"project", "list", "--portal-url", "https://p.example"},
			want: cmdutil.ConfigNotSetError("cluster name", "", cmdutil.ClusterNameOption, cmdutil.LoginHint),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			isolatedConfigDir(t)

			var out, errb bytes.Buffer

			code := runWith(context.Background(), tc.args, newFactory(&out, &errb))
			runCase{
				wantCode:   1,
				wantOut:    empty(),
				wantErrOut: equal("Error: " + tc.want.Error() + "\n"),
			}.check(t, code, out.String(), errb.String())
		})
	}
}

func TestRunWith_ConfigWarningOnInjectedStderr(t *testing.T) {
	writeConfigFile(t, isolatedConfigDir(t), "portal-url: [broken\n")

	var out, errb bytes.Buffer

	code := runWith(context.Background(), []string{"nosuch"}, newFactory(&out, &errb))
	runCase{
		wantCode: 1,
		wantOut:  empty(),
		wantErrOut: all(
			prefix("Warning: error reading config file: "),
			contains("\nError: unknown command \"nosuch\" for \"krci\"\n"),
		),
	}.check(t, code, out.String(), errb.String())
}

func TestRunWith_NoStateBetweenRuns(t *testing.T) {
	isolatedConfigDir(t)

	runs := []struct {
		args []string
		want string
	}{
		{[]string{"version", "--portal-url", "https://x.example"}, "https://x.example"},
		{[]string{"version"}, ""},
	}

	for _, r := range runs {
		if got := portalURLAfter(t, r.args...); got != r.want {
			t.Errorf("after %v: PortalURL = %q, want %q", r.args, got, r.want)
		}
	}
}

func TestRunWith_PortalURLPrecedence(t *testing.T) {
	cases := map[string]struct {
		args []string
		env  string
		file string
		want string
	}{
		"flag over env": {
			args: []string{"--portal-url", "https://flag.example"},
			env:  "https://env.example",
			file: "https://file.example",
			want: "https://flag.example",
		},
		"env over file": {
			env:  "https://env.example",
			file: "https://file.example",
			want: "https://env.example",
		},
		"file over default": {
			file: "https://file.example",
			want: "https://file.example",
		},
		"default": {},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := isolatedConfigDir(t)

			if tc.file != "" {
				writeConfigFile(t, dir, "portal-url: "+tc.file+"\n")
			}

			if tc.env != "" {
				t.Setenv("KRCI_PORTAL_URL", tc.env)
			}

			if got := portalURLAfter(t, append([]string{"version"}, tc.args...)...); got != tc.want {
				t.Errorf("PortalURL = %q, want %q", got, tc.want)
			}
		})
	}
}
