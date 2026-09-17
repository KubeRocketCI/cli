package build

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
)

var newFactory = cmdtest.NewFactory

func TestBuild_RejectsInvalidInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		argv    []string
		wantErr string
	}{
		{"missing positional", nil, "requires a project name"},
		{"overlong branch", []string{"my-app", "--branch", strings.Repeat("b", 254)}, "--branch must be at most 253 characters"},
		{"dry-run with table", []string{"my-app", "--dry-run", "-o", "table"}, "--dry-run cannot use -o table"},
		{"yaml without dry-run", []string{"my-app", "-o", "yaml"}, "-o yaml requires --dry-run"},
		{"unknown output format", []string{"my-app", "-o", "xml"}, "unknown output format"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := cmdtest.RunCmd(t, NewCmdBuild, tc.argv)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestBuild_EmptyBranchMeansDefault(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdBuild, []string{"my-app", "--branch", ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.Branch != "" {
		t.Fatalf("branch should stay empty: %q", opts.Branch)
	}
}

func TestBuild_RejectsInvalidName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		arg  string
	}{
		{"dots and uppercase", "My.App"},
		{"over 253 chars", strings.Repeat("a", 254)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := cmdtest.RunCmd(t, NewCmdBuild, []string{tc.arg})
			if err == nil || !strings.Contains(err.Error(), "<name>") {
				t.Fatalf("expected <name> validation error, got: %v", err)
			}
		})
	}
}

func TestBuild_AcceptsMaxLengthName(t *testing.T) {
	t.Parallel()

	name := strings.Repeat("a", 253)

	opts, err := cmdtest.RunCmd(t, NewCmdBuild, []string{name})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.Project != name {
		t.Fatalf("project not captured: %q", opts.Project)
	}
}

func TestBuild_AcceptsBranchWithSlash(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdBuild, []string{"my-app", "--branch", "feat/x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.Branch != "feat/x" {
		t.Fatalf("branch not captured: %q", opts.Branch)
	}
}

func TestBuild_RejectsManagedParam(t *testing.T) {
	t.Parallel()

	_, err := cmdtest.RunCmd(t, NewCmdBuild, []string{"my-app", "--param", "git-source-url=x"})
	if err == nil {
		t.Fatal("expected managed-param rejection")
	}

	want := portal.ValidateBuildParams(map[string]string{"git-source-url": "x"}).Error()
	if err.Error() != want {
		t.Fatalf("message:\n got  %q\n want %q", err.Error(), want)
	}
}

func TestBuild_RejectsEveryManagedParam(t *testing.T) {
	t.Parallel()

	for _, param := range portal.BuildManagedParams {
		t.Run(param, func(t *testing.T) {
			t.Parallel()

			_, err := cmdtest.RunCmd(t, NewCmdBuild, []string{"my-app", "--param", param + "=x"})
			if err == nil || !strings.Contains(err.Error(), "'"+param+"'") {
				t.Fatalf("expected rejection of %q, got: %v", param, err)
			}
		})
	}
}

func TestBuild_AcceptsNonManagedParams(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdBuild, []string{"my-app", "--param", "COMMIT_MESSAGE=hi", "--param", "k2=v2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.parsedParams["COMMIT_MESSAGE"] != "hi" || opts.parsedParams["k2"] != "v2" {
		t.Fatalf("params not parsed: %+v", opts.parsedParams)
	}
}

func TestBuild_Defaults(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdBuild, []string{"my-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if opts.Branch != "" {
		t.Errorf("branch default must be empty (server resolves it), got %q", opts.Branch)
	}

	if opts.DryRun {
		t.Error("dry-run must default to false")
	}

	if opts.OutputFormat != "" {
		t.Errorf("output default must be empty (table), got %q", opts.OutputFormat)
	}

	if opts.parsedParams != nil {
		t.Errorf("params default must be nil, got %+v", opts.parsedParams)
	}
}

func TestBuild_DryRunYAML(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdBuild, []string{"my-app", "--dry-run", "-o", "yaml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !opts.DryRun || opts.OutputFormat != "yaml" {
		t.Fatalf("dry-run/output not captured: %+v", opts)
	}
}

// TestBuild_DoesNotExposeLabel checks that --label, the raw escape hatch of
// `pipelinerun start`, is absent: `project build` resolves labels itself.
func TestBuild_DoesNotExposeLabel(t *testing.T) {
	t.Parallel()

	cmd := NewCmdBuild(newFactory(), nil)
	if cmd.Flags().Lookup("label") != nil {
		t.Error("--label flag must NOT be exposed")
	}
}

func TestBuild_HelpListsManagedParamsAndFlags(t *testing.T) {
	t.Parallel()

	cmd := NewCmdBuild(newFactory(), nil)
	help := cmd.Long + "\n" + cmd.Example

	for _, fragment := range []string{
		"--branch", "--param", "--dry-run", "-o json",
		"git-source-url", "gitfullrepositoryname", "krci pipelinerun start",
	} {
		if !strings.Contains(help, fragment) {
			t.Errorf("help text missing %q", fragment)
		}
	}
}

// TestBuildInput_MatchSpec pins the local mirror of POST /v1/pipelineruns/build
// to the vendored OpenAPI document: the managed-param set, the codebase name
// rule, and the branch bounds.
func TestBuildInput_MatchSpec(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../internal/portal/openapi/spec.json")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}

	var doc struct {
		Paths map[string]struct {
			Post struct {
				RequestBody struct {
					Content map[string]struct {
						Schema struct {
							Properties struct {
								Codebase struct {
									Pattern   string `json:"pattern"`
									MaxLength int    `json:"maxLength"`
								} `json:"codebase"`
								Branch struct {
									MinLength int `json:"minLength"`
									MaxLength int `json:"maxLength"`
								} `json:"branch"`
								Params struct {
									Managed []string `json:"x-krci-managed-params"`
								} `json:"params"`
							} `json:"properties"`
						} `json:"schema"`
					} `json:"content"`
				} `json:"requestBody"`
			} `json:"post"`
		} `json:"paths"`
	}

	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	props := doc.Paths["/v1/pipelineruns/build"].Post.RequestBody.Content["application/json"].Schema.Properties

	if len(props.Params.Managed) == 0 {
		t.Fatal("x-krci-managed-params missing from the vendored spec")
	}

	got := slices.Clone(portal.BuildManagedParams)
	want := slices.Clone(props.Params.Managed)
	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Errorf("BuildManagedParams drifted from the spec:\n go   %v\n spec %v", got, want)
	}

	if props.Codebase.Pattern != cmdutil.K8sNamePattern {
		t.Errorf("codebase pattern: spec %q, cmdutil %q", props.Codebase.Pattern, cmdutil.K8sNamePattern)
	}

	if props.Codebase.MaxLength != cmdutil.DNS1123SubdomainMaxLength {
		t.Errorf("codebase maxLength: spec %d, cmdutil %d", props.Codebase.MaxLength, cmdutil.DNS1123SubdomainMaxLength)
	}

	if props.Branch.MinLength != 1 || props.Branch.MaxLength != portal.MaxBranchLength {
		t.Errorf("branch bounds: spec [%d, %d], portal [1, %d]",
			props.Branch.MinLength, props.Branch.MaxLength, portal.MaxBranchLength)
	}
}
