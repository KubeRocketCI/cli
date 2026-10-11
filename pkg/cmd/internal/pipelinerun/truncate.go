package pipelinerun

import (
	"fmt"
	"slices"
	"strings"

	"github.com/KubeRocketCI/cli/internal/portal"
)

const reasonMaxLogLines = 25

// TruncateTaskLogs returns result with every task log cut to its last
// reasonMaxLogLines lines. Tasks is cloned; PipelineRuns is shared with the
// input.
func TruncateTaskLogs(result portal.PipelineRunListResult) portal.PipelineRunListResult {
	tasks := slices.Clone(result.Tasks)
	for i := range tasks {
		if tasks[i].Logs != "" {
			tasks[i].Logs = tailLines(tasks[i].Logs, reasonMaxLogLines)
		}
	}

	result.Tasks = tasks

	return result
}

func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if len(lines) <= n {
		return s
	}

	return fmt.Sprintf("... (%d lines truncated)\n", len(lines)-n) +
		strings.Join(lines[len(lines)-n:], "\n") + "\n"
}
