// Package pipelineruninternal holds helpers shared by `krci pipelinerun` verbs
// (validation, shared column headers, key=value parsing).
package pipelineruninternal

// SchemaVersion is the JSON envelope schema tag for pipelinerun command output.
// Matches the per-group pattern used by sonar, sca, and discovery.
const SchemaVersion = "1"

// Headers is the shared column set for `pipelinerun list` and `pipelinerun
// start`. The two verbs must emit byte-identical headers; a header-equality
// regression test enforces parity.
var Headers = []string{"NAME", "STATUS", "PROJECT", "PR", "AUTHOR", "TYPE", "STARTED", "DURATION"}
