package output

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/list"

	"github.com/KubeRocketCI/cli/internal/portal"
)

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

		if _, err := fmt.Fprintln(w, SectionStyle.Render("Tasks:")); err != nil {
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

		if _, err := fmt.Fprintln(w, SectionStyle.Render("Failed: "+SingleLine(t.Name))); err != nil {
			return err
		}

		if t.FailedStep != "" {
			if _, err := fmt.Fprintf(w, "%s %s (exit code %d)\n",
				ReasonLabel.Render("Step:"),
				SingleLine(t.FailedStep), t.ExitCode,
			); err != nil {
				return err
			}
		}

		if t.Message != "" {
			if _, err := fmt.Fprintf(w, "%s %s\n",
				ReasonLabel.Render("Message:"),
				SingleLine(t.Message),
			); err != nil {
				return err
			}
		}

		if t.Logs != "" {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}

			if _, err := fmt.Fprintln(w, SectionStyle.Render("Logs: "+SingleLine(t.Name))); err != nil {
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

	_, err := fmt.Fprintln(w, DimStyle.Render(msg))

	return err
}

func renderRunHeader(w io.Writer, run *portal.PipelineRunInfo) error {
	if _, err := fmt.Fprintln(w, SectionStyle.Render("Pipeline: "+SingleLine(run.Name))); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "%s %s\n",
		ReasonLabel.Render("Status:"),
		PipelineStatusColor(run.Status),
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

		if _, err := fmt.Fprintf(w, "%s %s\n", ReasonLabel.Render(field.label), SingleLine(field.value)); err != nil {
			return err
		}
	}

	return renderResults(w, run.Results)
}

// renderResults prints pipeline results as name=value lines, sorted by name,
// under one Results: label. Array and object values print as compact JSON.
func renderResults(w io.Writer, results map[string]any) error {
	label := ReasonLabel.Render("Results:")
	indent := strings.Repeat(" ", lipgloss.Width(label))

	for i, name := range slices.Sorted(maps.Keys(results)) {
		prefix := indent
		if i == 0 {
			prefix = label
		}

		if _, err := fmt.Fprintf(w, "%s %s=%s\n", prefix, SingleLine(name), resultValue(results[name])); err != nil {
			return err
		}
	}

	return nil
}

func resultValue(v any) string {
	if s, ok := v.(string); ok {
		return SingleLine(s)
	}

	b, err := json.Marshal(v)
	if err != nil {
		return cell(fmt.Sprint(v))
	}

	return cell(string(b))
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

func formatTaskLine(t portal.TaskRunInfo) string {
	style := SuccessStyle
	if portal.IsFailureStatus(t.Status) {
		style = FailStyle
	}

	return fmt.Sprintf("%-28s %s  %s",
		SingleLine(t.Name), style.Render(cell(t.Status)), DimStyle.Render(SingleLine(t.Duration)))
}
