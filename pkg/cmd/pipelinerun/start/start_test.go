package start

import (
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

var newFactory = cmdtest.NewFactory

func TestStart_RejectsMissingPositional(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdStart, []string{})
	if err == nil {
		t.Fatal("expected error for missing <pipeline> positional")
	}

	if !strings.Contains(err.Error(), "requires a pipeline name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_RejectsInvalidDNS1123(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdStart, []string{"Foo_Build"})
	if err == nil || !strings.Contains(err.Error(), "DNS-1123") {
		t.Fatalf("expected DNS-1123 error, got: %v", err)
	}
}

func TestStart_AcceptsValidName(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts == nil || opts.Pipeline != "foo-build" {
		t.Fatalf("opts not captured properly: %+v", opts)
	}
}

func TestStart_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "-o", "xml"})
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Fatalf("expected unknown-format error, got: %v", err)
	}
}

func TestStart_RejectsDryRunPlusTable(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "--dry-run", "-o", "table"})
	if err == nil || !strings.Contains(err.Error(), "--dry-run cannot use -o table") {
		t.Fatalf("expected dry-run+table mutex error, got: %v", err)
	}
}

func TestStart_RejectsYAMLWithoutDryRun(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "-o", "yaml"})
	if err == nil || !strings.Contains(err.Error(), "-o yaml requires --dry-run") {
		t.Fatalf("expected yaml-without-dry-run error, got: %v", err)
	}
}

func TestStart_DryRunYAMLOutputFormat(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "--dry-run", "-o", "yaml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !opts.DryRun {
		t.Errorf("DryRun not set")
	}

	if opts.OutputFormat != "yaml" {
		t.Errorf("OutputFormat = %q", opts.OutputFormat)
	}
}

func TestStart_RejectsParamWithoutEquals(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "--param", "keywithoutvalue"})
	if err == nil || !strings.Contains(err.Error(), "parameter must be key=value") {
		t.Fatalf("expected parser error, got: %v", err)
	}
}

func TestStart_RejectsParamEmptyKey(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "--param", "=value"})
	if err == nil || !strings.Contains(err.Error(), "parameter key must not be empty") {
		t.Fatalf("expected empty-key error, got: %v", err)
	}
}

func TestStart_RejectsDuplicateParam(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "--param", "k=v1", "--param", "k=v2"})
	if err == nil || !strings.Contains(err.Error(), "duplicate parameter") {
		t.Fatalf("expected duplicate-param error, got: %v", err)
	}
}

func TestStart_RejectsDuplicateLabel(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "--label", "k=v1", "--label", "k=v2"})
	if err == nil || !strings.Contains(err.Error(), "duplicate label") {
		t.Fatalf("expected duplicate-label error, got: %v", err)
	}
}

func TestStart_AcceptsParamValueWithEquals(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "--param", "token=abc=def=="})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.parsedParams["token"] != "abc=def==" {
		t.Errorf("expected value preserved, got: %q", opts.parsedParams["token"])
	}
}

func TestStart_AcceptsCommaSeparatedParam(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "--param", "items=v1,v2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.parsedParams["items"] != "v1,v2" {
		t.Errorf("comma value should be preserved verbatim, got: %q", opts.parsedParams["items"])
	}
}

func TestStart_DoesNotExposeWaitFollowTimeout(t *testing.T) {
	t.Parallel()

	cmd := NewCmdStart(newFactory(), nil)
	for _, name := range []string{"wait", "follow", "timeout"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Errorf("--%s flag must NOT be exposed (BR-wide non-goal)", name)
		}
	}
}

func TestStart_AcceptsValidLabel(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdStart, []string{"foo-build", "--label", "app.edp.epam.com/codebase=my-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.parsedLabels["app.edp.epam.com/codebase"] != "my-app" {
		t.Errorf("label not parsed: %+v", opts.parsedLabels)
	}
}

func TestStart_HelpHasExpectedExamples(t *testing.T) {
	t.Parallel()

	cmd := NewCmdStart(newFactory(), nil)
	help := cmd.Long + "\n" + cmd.Example

	for _, fragment := range []string{
		"--dry-run",
		"--param",
		"--label",
		"-o json",
	} {
		if !strings.Contains(help, fragment) {
			t.Errorf("help text missing %q", fragment)
		}
	}
}
