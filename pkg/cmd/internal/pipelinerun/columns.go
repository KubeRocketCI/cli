// Package pipelinerun holds helpers shared by the `krci pipelinerun` verbs and
// `krci project build` (validation, shared column headers, key=value parsing,
// row and dry-run rendering).
package pipelinerun

// SchemaVersion is the JSON envelope schema tag for pipelinerun command output.
// Matches the per-group pattern used by sonar, sca, and discovery.
const SchemaVersion = "1"

// Headers is the shared column set for `pipelinerun list`, `pipelinerun
// start` and `project build`.
var Headers = []string{"NAME", "STATUS", "PROJECT", "PR", "AUTHOR", "TYPE", "STARTED", "DURATION"}
