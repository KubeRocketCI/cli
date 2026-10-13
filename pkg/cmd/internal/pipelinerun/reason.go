package pipelinerun

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/list"

	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

var (
	sectionStyle = output.HeaderStyle.UnsetPadding()
	reasonLabel  = output.LabelStyle.Width(12)
)

// StatusColor colors a pipeline-run status. An unknown status is left
// unstyled.
func StatusColor(status string) output.Cell {
	switch status {
	case portal.StatusSucceeded:
		return output.Green(status)
	case portal.StatusFailed, portal.StatusCancelled, portal.StatusTimeout:
		return output.Red(status)
	case portal.StatusRunning:
		return output.Blue(status)
	default:
		return output.Text(status)
	}
}

// RenderReason renders pipeline info + task tree + failure diagnosis using lipgloss.
func RenderReason(w io.Writer, result *portal.PipelineRunListResult) error {
	if len(result.PipelineRuns) == 0 {
		return fmt.Errorf("no pipeline run data to render")
	}

	run := &result.PipelineRuns[0]

	if err := RenderRunInfo(w, run); err != nil {
		return err
	}

	if len(result.Tasks) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}

		if _, err := fmt.Fprintln(w, sectionStyle.Render("Tasks:")); err != nil {
			return err
		}

		items := make([]any, 0, len(result.Tasks))
		for _, t := range result.Tasks {
			items = append(items, formatTaskLine(t))
		}

		l := list.New(items...).
			Enumerator(taskListEnumerator(result.Tasks)).
			EnumeratorStyle(lipgloss.NewStyle())

		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}

	for _, t := range result.Tasks {
		if t.FailedStep == "" && t.Logs == "" {
			continue
		}

		if _, err := fmt.Fprintln(w, sectionStyle.Render("Failed: "+output.SingleLine(t.Name))); err != nil {
			return err
		}

		if t.FailedStep != "" {
			if _, err := fmt.Fprintf(w, "%s %s (exit code %d)\n",
				reasonLabel.Render("Step:"),
				output.SingleLine(t.FailedStep), t.ExitCode,
			); err != nil {
				return err
			}
		}

		if t.Message != "" {
			if _, err := fmt.Fprintf(w, "%s %s\n",
				reasonLabel.Render("Message:"),
				output.SingleLine(t.Message),
			); err != nil {
				return err
			}
		}

		if t.Logs != "" {
			if err := RenderLogs(w, "Logs: "+output.SingleLine(t.Name), t.Logs); err != nil {
				return err
			}
		}
	}

	return nil
}

// RenderLogs writes a blank line, header in the section style and logs.
func RenderLogs(w io.Writer, header, logs string) error {
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, sectionStyle.Render(header)); err != nil {
		return err
	}

	_, err := fmt.Fprintln(w, logs)

	return err
}

// RenderNoTaskData writes a dim note for --reason when no task data is
// available; reason is the TasksUnavailable value of the result.
func RenderNoTaskData(w io.Writer, reason string) error {
	var msg string

	switch reason {
	case portal.TasksRunNotFinished:
		msg = "Pipeline run has not finished yet. Task data is available after it finishes."
	case portal.TasksNone:
		msg = "The run finished without scheduling any task."
	default:
		msg = "Task data is not available. The run may not yet be indexed in Tekton Results."
	}

	_, err := fmt.Fprintln(w, output.DimStyle.Render(msg))

	return err
}

// RenderRunInfo renders a single pipeline run's info header.
func RenderRunInfo(w io.Writer, run *portal.PipelineRunInfo) error {
	if _, err := fmt.Fprintln(w, sectionStyle.Render("Pipeline: "+output.SingleLine(run.Name))); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "%s %s\n",
		reasonLabel.Render("Status:"),
		StatusColor(run.Status).Render(true),
	); err != nil {
		return err
	}

	for _, field := range []struct{ label, value string }{
		{"Duration:", run.Duration},
		{"Pipeline:", run.Pipeline},
		{"Project:", run.Project},
		{"Deployment:", run.Deployment},
		{"Env:", run.Env},
	} {
		if field.value == "" {
			continue
		}

		if _, err := fmt.Fprintf(w, "%s %s\n", reasonLabel.Render(field.label), output.SingleLine(field.value)); err != nil {
			return err
		}
	}

	return renderResults(w, run.Results)
}

// renderResults prints pipeline results as name=value lines, sorted by name,
// under one Results: label. Array and object values print as compact JSON.
func renderResults(w io.Writer, results map[string]any) error {
	label := reasonLabel.Render("Results:")
	indent := strings.Repeat(" ", lipgloss.Width(label))

	for i, name := range slices.Sorted(maps.Keys(results)) {
		prefix := indent
		if i == 0 {
			prefix = label
		}

		if _, err := fmt.Fprintf(w, "%s %s=%s\n", prefix, output.SingleLine(name), resultValue(results[name])); err != nil {
			return err
		}
	}

	return nil
}

func resultValue(v any) string {
	if s, ok := v.(string); ok {
		return output.SingleLine(s)
	}

	b, err := json.Marshal(v)
	if err != nil {
		b = []byte(fmt.Sprint(v))
	}

	return output.Text(string(b)).Render(false)
}

func taskListEnumerator(tasks []portal.TaskRunInfo) list.Enumerator {
	return func(_ list.Items, i int) string {
		if i >= len(tasks) {
			return "  "
		}

		if portal.IsFailureStatus(tasks[i].Status) {
			return output.Red("✗").Render(true) + " "
		}

		return output.Green("✓").Render(true) + " "
	}
}

func formatTaskLine(t portal.TaskRunInfo) string {
	status := output.Green(t.Status)
	if portal.IsFailureStatus(t.Status) {
		status = output.Red(t.Status)
	}

	return fmt.Sprintf("%-28s %s  %s",
		output.SingleLine(t.Name), status.Render(true), output.DimStyle.Render(output.SingleLine(t.Duration)))
}
