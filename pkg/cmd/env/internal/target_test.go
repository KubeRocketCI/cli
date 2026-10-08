package envinternal

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/portal"
)

func TestTargetArgs(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "pods <deployment> <env>"}
	check := TargetArgs("pods")

	if err := check(cmd, []string{"my-pipeline", "dev"}); err != nil {
		t.Errorf("two positionals must pass, got %v", err)
	}

	for _, args := range [][]string{{}, {"only-one"}, {"a", "b", "c"}} {
		err := check(cmd, args)
		if err == nil {
			t.Fatalf("expected an error for args=%v", args)
		}

		for _, want := range []string{"requires a deployment and an env", "krci env pods my-pipeline prod", "krci env list"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("args=%v: error %q misses %q", args, err, want)
			}
		}
	}
}

func TestValidateTarget(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		format, deployment, env string
		want                    string
	}{
		"valid":              {format: "json", deployment: "my-pipeline", env: "dev"},
		"default format":     {deployment: "my-pipeline", env: "dev"},
		"unknown format":     {format: "yaml", deployment: "my-pipeline", env: "dev", want: "unknown output format"},
		"invalid deployment": {deployment: "BAD_NAME", env: "dev", want: "<deployment> must be a valid DNS-1123 name"},
		"invalid env":        {deployment: "my-pipeline", env: "Bad_Env", want: "<env> must be a valid DNS-1123 name"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := ValidateTarget(tc.format, tc.deployment, tc.env)

			switch {
			case tc.want == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestMapNotFound(t *testing.T) {
	t.Parallel()

	other := errors.New("portal returned HTTP 500")

	cases := map[string]struct {
		err  error
		want string
	}{
		"deployment":         {err: portal.ErrDeploymentNotFound, want: `deployment "my-pipeline" not found`},
		"environment":        {err: portal.ErrEnvNotFound, want: `environment "dev" not found in deployment "my-pipeline"`},
		"wrapped deployment": {err: fmt.Errorf("resolving: %w", portal.ErrDeploymentNotFound), want: `deployment "my-pipeline" not found`},
		"anything else":      {err: other, want: other.Error()},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := MapNotFound(tc.err, "my-pipeline", "dev"); got.Error() != tc.want {
				t.Errorf("MapNotFound = %q, want %q", got, tc.want)
			}
		})
	}
}
