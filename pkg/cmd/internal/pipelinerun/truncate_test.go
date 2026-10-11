package pipelinerun

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal"
)

func numberedLog(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}

	return b.String()
}

func TestTruncateTaskLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		logs string
		want string
	}{
		{
			name: "empty log stays empty",
			logs: "",
			want: "",
		},
		{
			name: "short log unchanged",
			logs: numberedLog(3),
			want: numberedLog(3),
		},
		{
			name: "log at the limit unchanged",
			logs: numberedLog(reasonMaxLogLines),
			want: numberedLog(reasonMaxLogLines),
		},
		{
			name: "long log keeps the last lines with a header",
			logs: numberedLog(reasonMaxLogLines + 5),
			want: "... (5 lines truncated)\n" + strings.Join(lineRange(6, reasonMaxLogLines+5), "\n") + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := portal.PipelineRunListResult{Tasks: []portal.TaskRunInfo{{Name: "build", Logs: tt.logs}}}

			got := TruncateTaskLogs(in)
			if got.Tasks[0].Logs != tt.want {
				t.Errorf("Logs = %q, want %q", got.Tasks[0].Logs, tt.want)
			}
		})
	}
}

func TestTruncateTaskLogs_NoTasksKeepsNil(t *testing.T) {
	t.Parallel()

	got := TruncateTaskLogs(portal.PipelineRunListResult{})
	if got.Tasks != nil {
		t.Errorf("Tasks = %#v, want nil", got.Tasks)
	}
}

func lineRange(from, to int) []string {
	out := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("line %d", i))
	}

	return out
}

func newLongResult() portal.PipelineRunListResult {
	return portal.PipelineRunListResult{
		PipelineRuns: []portal.PipelineRunInfo{{Name: "my-app-build-run-1", Status: portal.StatusFailed}},
		Logs:         numberedLog(3),
		Tasks: []portal.TaskRunInfo{
			{Name: "clone", Status: portal.StatusSucceeded, Logs: numberedLog(2)},
			{Name: "test", Status: portal.StatusFailed, FailedStep: "unit", ExitCode: 1, Logs: numberedLog(2 * reasonMaxLogLines)},
		},
	}
}

func TestTruncateTaskLogs_LeavesInputUnchanged(t *testing.T) {
	t.Parallel()

	result := newLongResult()
	want := newLongResult()

	truncated := TruncateTaskLogs(result)

	if !reflect.DeepEqual(result, want) {
		t.Errorf("input differs from its fixture after truncation")
	}

	truncated.Tasks[1].Logs = "overwritten"
	truncated.Tasks[1].Status = portal.StatusSucceeded

	if !reflect.DeepEqual(result, want) {
		t.Errorf("writing to the copy changed the input; Tasks slice is shared")
	}
}
