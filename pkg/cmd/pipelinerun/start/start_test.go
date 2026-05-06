package start

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

var newFactory = cmdtest.NewFactory

// runCmd executes the command with argv and returns the captured opts so
// tests can assert post-validation state without hitting the network.
func runCmd(t *testing.T, argv []string) (*StartOptions, error) {
	t.Helper()

	var captured *StartOptions

	cmd := NewCmdStart(newFactory(), func(o *StartOptions) error {
		captured = o

		return nil
	})

	cmd.SetArgs(argv)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		return captured, err
	}

	return captured, nil
}

func TestStart_RejectsMissingPositional(t *testing.T) {
	t.Parallel()

	_, err := runCmd(t, []string{})
	if err == nil {
		t.Fatal("expected error for missing <pipeline> positional")
	}

	if !strings.Contains(err.Error(), "requires a pipeline name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_RejectsInvalidDNS1123(t *testing.T) {
	t.Parallel()

	_, err := runCmd(t, []string{"Foo_Build"})
	if err == nil || !strings.Contains(err.Error(), "DNS-1123") {
		t.Fatalf("expected DNS-1123 error, got: %v", err)
	}
}

func TestStart_AcceptsValidName(t *testing.T) {
	t.Parallel()

	opts, err := runCmd(t, []string{"foo-build"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts == nil || opts.Pipeline != "foo-build" {
		t.Fatalf("opts not captured properly: %+v", opts)
	}
}

func TestStart_RejectsUnknownOutputFormat(t *testing.T) {
	t.Parallel()

	_, err := runCmd(t, []string{"foo-build", "-o", "xml"})
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Fatalf("expected unknown-format error, got: %v", err)
	}
}

func TestStart_RejectsDryRunPlusTable(t *testing.T) {
	t.Parallel()

	_, err := runCmd(t, []string{"foo-build", "--dry-run", "-o", "table"})
	if err == nil || !strings.Contains(err.Error(), "--dry-run cannot use -o table") {
		t.Fatalf("expected dry-run+table mutex error, got: %v", err)
	}
}

func TestStart_RejectsYAMLWithoutDryRun(t *testing.T) {
	t.Parallel()

	_, err := runCmd(t, []string{"foo-build", "-o", "yaml"})
	if err == nil || !strings.Contains(err.Error(), "-o yaml requires --dry-run") {
		t.Fatalf("expected yaml-without-dry-run error, got: %v", err)
	}
}

func TestStart_DryRunYAMLOutputFormat(t *testing.T) {
	t.Parallel()

	opts, err := runCmd(t, []string{"foo-build", "--dry-run", "-o", "yaml"})
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

	_, err := runCmd(t, []string{"foo-build", "--param", "keywithoutvalue"})
	if err == nil || !strings.Contains(err.Error(), "parameter must be key=value") {
		t.Fatalf("expected parser error, got: %v", err)
	}
}

func TestStart_RejectsParamEmptyKey(t *testing.T) {
	t.Parallel()

	_, err := runCmd(t, []string{"foo-build", "--param", "=value"})
	if err == nil || !strings.Contains(err.Error(), "parameter key must not be empty") {
		t.Fatalf("expected empty-key error, got: %v", err)
	}
}

func TestStart_RejectsDuplicateParam(t *testing.T) {
	t.Parallel()

	_, err := runCmd(t, []string{"foo-build", "--param", "k=v1", "--param", "k=v2"})
	if err == nil || !strings.Contains(err.Error(), "duplicate parameter") {
		t.Fatalf("expected duplicate-param error, got: %v", err)
	}
}

func TestStart_RejectsDuplicateLabel(t *testing.T) {
	t.Parallel()

	_, err := runCmd(t, []string{"foo-build", "--label", "k=v1", "--label", "k=v2"})
	if err == nil || !strings.Contains(err.Error(), "duplicate label") {
		t.Fatalf("expected duplicate-label error, got: %v", err)
	}
}

func TestStart_AcceptsParamValueWithEquals(t *testing.T) {
	t.Parallel()

	opts, err := runCmd(t, []string{"foo-build", "--param", "token=abc=def=="})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.parsedParams["token"] != "abc=def==" {
		t.Errorf("expected value preserved, got: %q", opts.parsedParams["token"])
	}
}

func TestStart_AcceptsCommaSeparatedParam(t *testing.T) {
	t.Parallel()

	opts, err := runCmd(t, []string{"foo-build", "--param", "items=v1,v2"})
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

	opts, err := runCmd(t, []string{"foo-build", "--label", "app.edp.epam.com/codebase=my-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.parsedLabels["app.edp.epam.com/codebase"] != "my-app" {
		t.Errorf("label not parsed: %+v", opts.parsedLabels)
	}
}

// portalManifest is what the Portal procedure returns for `dryRun=true`:
// the rendered PipelineRun draft as a JSON object (parsed at the transport
// boundary). The CLI re-encodes per requested output format.
func portalManifest() map[string]any {
	return map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata": map[string]any{
			"generateName": "foo-build-run-",
			"labels":       map[string]any{"app.edp.epam.com/codebase": "my-app"},
		},
		"spec": map[string]any{
			"params": []any{map[string]any{"name": "git-revision", "value": "main"}},
		},
	}
}

func newDryRunOpts(t *testing.T, format string) (*StartOptions, *bytes.Buffer) {
	t.Helper()

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	return &StartOptions{
		IO:           &iostreams.IOStreams{Out: out, ErrOut: errOut},
		OutputFormat: format,
		DryRun:       true,
	}, out
}

func TestRenderDryRun_DefaultEmitsYAML(t *testing.T) {
	t.Parallel()

	opts, out := newDryRunOpts(t, "")

	if err := renderDryRun(opts, &portal.StartResult{DryRunManifest: portalManifest()}); err != nil {
		t.Fatalf("renderDryRun: %v", err)
	}

	got := out.String()

	// YAML markers: bare keys, no quoted JSON braces wrapping the doc.
	for _, want := range []string{
		"apiVersion: tekton.dev/v1\n",
		"kind: PipelineRun\n",
		"generateName: foo-build-run-\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing YAML fragment %q in output:\n%s", want, got)
		}
	}

	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("default dry-run output should be YAML, got JSON-looking output:\n%s", got)
	}
}

func TestRenderDryRun_OutputYAMLEmitsYAML(t *testing.T) {
	t.Parallel()

	opts, out := newDryRunOpts(t, "yaml")

	if err := renderDryRun(opts, &portal.StartResult{DryRunManifest: portalManifest()}); err != nil {
		t.Fatalf("renderDryRun: %v", err)
	}

	if !strings.Contains(out.String(), "apiVersion: tekton.dev/v1") {
		t.Errorf("-o yaml output missing YAML markers:\n%s", out.String())
	}
}

func TestRenderDryRun_OutputJSONEmbedsParsedObject(t *testing.T) {
	t.Parallel()

	opts, out := newDryRunOpts(t, "json")

	if err := renderDryRun(opts, &portal.StartResult{DryRunManifest: portalManifest()}); err != nil {
		t.Fatalf("renderDryRun: %v", err)
	}

	var envelope struct {
		SchemaVersion string         `json:"schemaVersion"`
		Data          map[string]any `json:"data"`
	}

	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("envelope is not parseable JSON: %v\noutput: %s", err, out.String())
	}

	if envelope.SchemaVersion != "1" {
		t.Errorf("schemaVersion = %q", envelope.SchemaVersion)
	}

	// data must be a parsed object — NOT a string-wrapped manifest.
	if envelope.Data["apiVersion"] != "tekton.dev/v1" {
		t.Errorf("data.apiVersion = %v (expected parsed object, got: %#v)", envelope.Data["apiVersion"], envelope.Data)
	}

	if envelope.Data["kind"] != "PipelineRun" {
		t.Errorf("data.kind = %v", envelope.Data["kind"])
	}
}

func TestRenderDryRun_RejectsEmptyManifest(t *testing.T) {
	t.Parallel()

	opts, _ := newDryRunOpts(t, "")

	err := renderDryRun(opts, &portal.StartResult{DryRunManifest: nil})
	if err == nil {
		t.Fatal("expected error on empty manifest")
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
