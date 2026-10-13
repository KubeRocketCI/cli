// Package pipelinerun holds helpers shared by the `krci pipelinerun` verbs and
// `krci project build` (validation, shared column headers, key=value parsing,
// row and dry-run rendering, run info and --reason rendering, the status color,
// --reason log truncation).
package pipelinerun

// Headers is the shared column set for `pipelinerun list`, `pipelinerun
// start` and `project build`.
var Headers = []string{"NAME", "STATUS", "PROJECT", "PR", "AUTHOR", "TYPE", "STARTED", "DURATION"}
