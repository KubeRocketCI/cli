// Package sonarinternal holds helpers shared by `krci sonar` verbs
// (validation, enum sets, render/format dispatch, error-envelope plumbing).
// Placed under an `internal/` directory so Go enforces package visibility:
// only packages below `pkg/cmd/sonar/` may import it.
package sonarinternal

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/output"
)

// MaxPageSize is the upper bound on `--page-size` for every paginated sonar
// verb, mirroring the shared Zod schema cap on portal-side.
const MaxPageSize = 500

// ValidateProjectKey returns an error when project does not match DNS-1123.
// Codebase names (by Portal convention, also the SonarQube projectKey) follow
// the same shape as Kubernetes namespaces — see internal/cmdutil/validate.go.
func ValidateProjectKey(project string) error {
	if project == "" {
		return fmt.Errorf("<project> must not be empty")
	}

	if len(project) > cmdutil.DNS1123SubdomainMaxLength {
		return fmt.Errorf("<project> must be at most %d characters", cmdutil.DNS1123SubdomainMaxLength)
	}

	if !cmdutil.IsValidDNS1123Label(project) {
		return fmt.Errorf("<project> must be a valid DNS-1123 name")
	}

	return nil
}

// ValidatePullRequest rejects an explicitly supplied empty --pull-request value.
// changed is true when the flag was set on the command line (regardless of value).
func ValidatePullRequest(changed bool, value string) error {
	if changed && value == "" {
		return fmt.Errorf("--pull-request requires a non-empty value")
	}

	return nil
}

// ValidateOutputFormat rejects `-o` values other than "", "table", or "json".
// An empty string means "use default" (table) and is always valid.
func ValidateOutputFormat(format string) error {
	switch format {
	case "", output.FormatTable, output.FormatJSON:
		return nil
	default:
		return fmt.Errorf("unknown output format: %s (use 'json' or 'table')", format)
	}
}

// ValidateProjectCommand runs the standard validator triple used by the three
// project-scoped verbs (get, gate, issues): output format → DNS-1123 project
// key → non-empty --pull-request (when supplied).
func ValidateProjectCommand(cmd *cobra.Command, outputFormat, project, pullRequest string) error {
	if err := ValidateOutputFormat(outputFormat); err != nil {
		return err
	}

	if err := ValidateProjectKey(project); err != nil {
		return err
	}

	return ValidatePullRequest(cmd.Flags().Changed("pull-request"), pullRequest)
}

// EnumSet pairs the canonical list and lookup map for a SonarQube enum.
// Callers treat both fields as read-only.
type EnumSet struct {
	Values []string
	set    map[string]struct{}
}

func newEnumSet(values ...string) EnumSet {
	s := make(map[string]struct{}, len(values))
	for _, v := range values {
		s[v] = struct{}{}
	}

	return EnumSet{Values: values, set: s}
}

// Canonical SonarQube enum sets. Keep in sync with the Portal's shared
// SonarQube schema.
var (
	IssueTypes      = newEnumSet("BUG", "VULNERABILITY", "CODE_SMELL")
	IssueSeverities = newEnumSet("BLOCKER", "CRITICAL", "MAJOR", "MINOR", "INFO")
	IssueStatuses   = newEnumSet("OPEN", "CONFIRMED", "REOPENED", "RESOLVED", "CLOSED")
)

// ValidateEnumCSV checks every comma-separated/repeated value in values
// against allowed. flagName is used in error messages. Returns the flattened
// list of validated items in input order with duplicates collapsed (empty
// when values is empty).
func ValidateEnumCSV(flagName string, values []string, allowed EnumSet) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	expanded := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, v := range values {
		for part := range strings.SplitSeq(v, ",") {
			p := strings.TrimSpace(part)
			if p == "" {
				continue
			}

			if _, ok := allowed.set[p]; !ok {
				return nil, fmt.Errorf(
					"invalid --%s=%s; must be one of %s",
					flagName, p, strings.Join(allowed.Values, ", "),
				)
			}

			if _, dup := seen[p]; dup {
				continue
			}
			seen[p] = struct{}{}

			expanded = append(expanded, p)
		}
	}

	return expanded, nil
}
