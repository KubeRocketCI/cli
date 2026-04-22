package output

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/list"

	"github.com/KubeRocketCI/cli/internal/portal"
)

const reasonMaxLogLines = 25

// Pipeline run output styles.
var (
	SectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor))
	ReasonLabel  = LabelStyle.Width(12)
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Green)
	FailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	DimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// RenderReason renders pipeline info + task tree + failure diagnosis using lipgloss.
func RenderReason(w io.Writer, result *portal.PipelineRunListResult) error {
	if len(result.PipelineRuns) == 0 {
		return fmt.Errorf("no pipeline run data to render")
	}

	run := &result.PipelineRuns[0]

	if err := renderRunHeader(w, run); err != nil {
		return err
	}

	if len(result.Tasks) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}

		if _, err := lipgloss.Fprintln(w, SectionStyle.Render("Tasks:")); err != nil {
			return err
		}

		items := make([]any, 0, len(result.Tasks))
		for _, t := range result.Tasks {
			items = append(items, formatTaskLine(t))
		}

		l := list.New(items...).
			Enumerator(taskListEnumerator(result.Tasks)).
			EnumeratorStyle(lipgloss.NewStyle())

		if _, err := lipgloss.Fprintln(w, l); err != nil {
			return err
		}
	}

	for _, t := range result.Tasks {
		if t.FailedStep == "" && t.Logs == "" {
			continue
		}

		if _, err := lipgloss.Fprintln(w, SectionStyle.Render("Failed: "+t.Name)); err != nil {
			return err
		}

		if t.FailedStep != "" {
			if _, err := lipgloss.Fprintf(w, "%s %s (exit code %d)\n",
				ReasonLabel.Render("Step:"),
				t.FailedStep, t.ExitCode,
			); err != nil {
				return err
			}
		}

		if t.Message != "" {
			if _, err := lipgloss.Fprintf(w, "%s %s\n",
				ReasonLabel.Render("Message:"),
				t.Message,
			); err != nil {
				return err
			}
		}

		if t.Logs != "" {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}

			if _, err := lipgloss.Fprintln(w, SectionStyle.Render("Logs: "+t.Name)); err != nil {
				return err
			}

			if _, err := fmt.Fprintln(w, t.Logs); err != nil {
				return err
			}
		}
	}

	return nil
}

// RenderRunInfo renders a single pipeline run's info header.
func RenderRunInfo(w io.Writer, run *portal.PipelineRunInfo) error {
	return renderRunHeader(w, run)
}

// RenderNoTaskData writes a dim status message for --reason when no task data is available.
func RenderNoTaskData(w io.Writer, status string) error {
	var msg string
	if status == portal.StatusRunning {
		msg = "Pipeline is still running. Task data will appear here as tasks complete."
	} else {
		msg = "Task data is not available. The run may not yet be indexed in Tekton Results."
	}

	_, err := lipgloss.Fprintln(w, DimStyle.Render(msg))

	return err
}

func renderRunHeader(w io.Writer, run *portal.PipelineRunInfo) error {
	if _, err := lipgloss.Fprintln(w, SectionStyle.Render("Pipeline: "+run.Name)); err != nil {
		return err
	}

	if _, err := lipgloss.Fprintf(w, "%s %s\n",
		ReasonLabel.Render("Status:"),
		PipelineStatusColor(run.Status),
	); err != nil {
		return err
	}

	if run.Duration != "" {
		if _, err := lipgloss.Fprintf(w, "%s %s\n",
			ReasonLabel.Render("Duration:"),
			run.Duration,
		); err != nil {
			return err
		}
	}

	if run.Pipeline != "" {
		if _, err := lipgloss.Fprintf(w, "%s %s\n",
			ReasonLabel.Render("Pipeline:"),
			run.Pipeline,
		); err != nil {
			return err
		}
	}

	if run.Project != "" {
		if _, err := lipgloss.Fprintf(w, "%s %s\n",
			ReasonLabel.Render("Project:"),
			run.Project,
		); err != nil {
			return err
		}
	}

	return nil
}

func taskListEnumerator(tasks []portal.TaskRunInfo) list.Enumerator {
	return func(_ list.Items, i int) string {
		if i >= len(tasks) {
			return "  "
		}

		if portal.IsFailureStatus(tasks[i].Status) {
			return FailStyle.Render("✗") + " "
		}

		return SuccessStyle.Render("✓") + " "
	}
}

// TruncateTaskLogs trims each failed task's log to the last N lines,
// keeping only the tail that typically contains the real failure reason.
func TruncateTaskLogs(result *portal.PipelineRunListResult) {
	for i := range result.Tasks {
		if result.Tasks[i].Logs != "" {
			result.Tasks[i].Logs = tailLines(result.Tasks[i].Logs, reasonMaxLogLines)
		}
	}
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

func formatTaskLine(t portal.TaskRunInfo) string {
	status := SuccessStyle.Render(t.Status)
	if portal.IsFailureStatus(t.Status) {
		status = FailStyle.Render(t.Status)
	}

	return fmt.Sprintf("%-28s %s  %s", t.Name, status, DimStyle.Render(t.Duration))
}
