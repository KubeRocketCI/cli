// Package scainternal holds helpers shared by `krci sca` verbs
// (validation, enum sets, render/format dispatch, error-envelope plumbing).
// Structure mirrors `pkg/cmd/sonar/internal/`.
package scainternal

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/output"
)

// MaxPageSize is the upper bound on `--page-size` for every paginated sca
// verb. Matches the OpenAPI ceiling.
const MaxPageSize = 500

// BranchFlagUsage is the verbatim help text for `--branch` across every
// per-codebase sca verb. Spec Requirement "Codebase + Branch Addressing"
// mandates this exact string so users always see the same Dep-Track `version`
// field explanation.
const BranchFlagUsage = "branch name (maps to the Dep-Track project 'version' field). " +
	"Defaults to the codebase's spec.defaultBranch. " +
	"Run 'krci sca list --search=<codebase>' to discover all recorded versions."

// SeverityFlagUsage is the verbatim help text for `--severity` across the
// verbs that expose it. Spec design §D4 mandates this exact string: it
// enumerates the allowed values and explains the inclusive semantics.
const SeverityFlagUsage = "minimum severity to include (inclusive). " +
	"One of: critical, high, medium, low, info. Case-insensitive. " +
	"'high' returns high+critical; 'medium' returns medium+high+critical; etc. " +
	"INFO includes UNASSIGNED."

// ValidateCodebaseKey mirrors sonarinternal.ValidateProjectKey — codebase
// names by platform convention follow DNS-1123.
func ValidateCodebaseKey(codebase string) error {
	if codebase == "" {
		return fmt.Errorf("<codebase> must not be empty")
	}

	if len(codebase) > cmdutil.DNS1123SubdomainMaxLength {
		return fmt.Errorf("<codebase> must be at most %d characters", cmdutil.DNS1123SubdomainMaxLength)
	}

	if !cmdutil.IsValidDNS1123Label(codebase) {
		return fmt.Errorf("<codebase> must be a valid DNS-1123 name")
	}

	return nil
}

// ValidateNonEmptyFlag rejects a flag that was explicitly supplied with an
// empty value. changed is true when the flag was set on the command line.
func ValidateNonEmptyFlag(name string, changed bool, value string) error {
	if changed && value == "" {
		return fmt.Errorf("--%s requires a non-empty value", name)
	}

	return nil
}

// ValidatePageBounds rejects out-of-range --page / --page-size values. Shared
// between every paginated sca verb so the ceiling is enforced uniformly.
func ValidatePageBounds(page, pageSize int) error {
	if pageSize < 1 || pageSize > MaxPageSize {
		return fmt.Errorf("--page-size must be between 1 and %d", MaxPageSize)
	}
	if page < 1 {
		return fmt.Errorf("--page must be >= 1")
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

// ValidateCodebaseCommand runs the validator chain used by every per-codebase
// sca verb (get, components, findings): output format → DNS-1123 codebase
// → non-empty --branch (when explicitly supplied). Unlike sonar, sca never
// takes --pr, so there is no scope-mutex step.
func ValidateCodebaseCommand(cmd *cobra.Command, outputFormat, codebase, branch string) error {
	if err := ValidateOutputFormat(outputFormat); err != nil {
		return err
	}

	if err := ValidateCodebaseKey(codebase); err != nil {
		return err
	}

	return ValidateNonEmptyFlag("branch", cmd.Flags().Changed("branch"), branch)
}

// Canonical Dep-Track severity values in descending severity order.
// UNASSIGNED is filtered together with INFO per design §D4 — see
// InclusiveSeverities.
var severityOrder = []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "INFO", "UNASSIGNED"}

// SeverityEnum is the set of values accepted by the `--severity` flag.
// Lowercase "unassigned" is intentionally omitted from the user-facing list
// (it is reachable via `--severity=info`) but is still accepted if typed
// explicitly, for scripting symmetry with the upstream payload values.
var allowedSeverityInputs = map[string]string{
	"critical":   "CRITICAL",
	"high":       "HIGH",
	"medium":     "MEDIUM",
	"low":        "LOW",
	"info":       "INFO",
	"unassigned": "UNASSIGNED",
}

// ValidateSeverityCSV canonicalises the comma-separated / repeatable values
// in values to upper-case Dep-Track severities. Empty input returns nil. An
// unknown value returns an error listing the accepted values.
//
// The function returns validated values in input order with duplicates
// collapsed — this makes the test expectations stable.
func ValidateSeverityCSV(values []string) ([]string, error) {
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

			upper, ok := allowedSeverityInputs[strings.ToLower(p)]
			if !ok {
				return nil, fmt.Errorf(
					"invalid --severity=%s; must be one of %s",
					p, strings.Join(severityOrder, ", "),
				)
			}

			if _, dup := seen[upper]; dup {
				continue
			}
			seen[upper] = struct{}{}

			expanded = append(expanded, upper)
		}
	}

	return expanded, nil
}

// InclusiveSeverities returns the severities that should be included when the
// user supplies `--severity=<min>`. The return set is "min and above": e.g.
// `HIGH` → `{CRITICAL, HIGH}`; `INFO` → `{CRITICAL, HIGH, MEDIUM, LOW, INFO,
// UNASSIGNED}`. An unknown min returns nil so callers can treat that as
// "no filter" (already validated upstream).
//
// severityOrder is in descending severity (CRITICAL first); we walk from the
// start through `min` inclusive.
func InclusiveSeverities(min string) []string {
	if min == "" {
		return nil
	}
	upper := strings.ToUpper(min)
	out := make([]string, 0, len(severityOrder))
	found := false
	for _, s := range severityOrder {
		out = append(out, s)
		if s == upper {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	// "INFO" implicitly includes UNASSIGNED — design §D4. UNASSIGNED already
	// sits beyond INFO in severityOrder; append it when the selected min is
	// INFO so the filter set matches the design intent.
	if upper == "INFO" && !slices.Contains(out, "UNASSIGNED") {
		out = append(out, "UNASSIGNED")
	}
	return out
}

// SeverityMatches returns true if severity is in the allowed set. Empty allowed
// means "no filter" — callers expected to short-circuit before calling.
// Severity is normalised to upper-case so upstream casing variations don't
// silently drop rows (allowed is always canonical upper-case).
func SeverityMatches(severity string, allowed []string) bool {
	return slices.Contains(allowed, strings.ToUpper(severity))
}

// severityRank ranks Dep-Track severities ascending (UNASSIGNED=0 is least
// severe). Used by InclusiveFromSet to pick the lowest-rank threshold.
var severityRank = map[string]int{
	"CRITICAL":   5,
	"HIGH":       4,
	"MEDIUM":     3,
	"LOW":        2,
	"INFO":       1,
	"UNASSIGNED": 0,
}

// ExpandSeverityFlag is the composite helper for run functions: it validates
// the raw `--severity` flag values (silently ignoring the error — callers are
// expected to run ValidateSeverityCSV in RunE first) and returns the inclusive
// "min and above" expansion. nil input returns nil so the caller can skip
// filtering without a branch on slice length.
func ExpandSeverityFlag(raw []string) []string {
	validated, _ := ValidateSeverityCSV(raw)
	return InclusiveFromSet(validated)
}

// InclusiveFromSet returns the inclusive severity set derived from an
// already-validated `--severity` value list. When several thresholds are
// supplied, it takes the least-severe one ("medium" when "critical,medium" is
// supplied) so the expansion pulls everything at or above medium — matching
// the intent of "include all of these severities plus higher ones".
func InclusiveFromSet(set []string) []string {
	if len(set) == 0 {
		return nil
	}
	min := set[0]
	for _, s := range set[1:] {
		if severityRank[s] < severityRank[min] {
			min = s
		}
	}
	return InclusiveSeverities(min)
}

// ComponentMatchesSeverity returns true if the component's metrics contain at
// least one vulnerability of a severity present in `allowed` (or any
// severity when `allowed` is empty). Separated so callers can invoke it
// without reaching into the scainternal types.
func ComponentMatchesSeverity(hasCounts func(severity string) int, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, sev := range allowed {
		if hasCounts(sev) > 0 {
			return true
		}
	}
	return false
}
