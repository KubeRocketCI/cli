package pipelinerun

import (
	"fmt"
	"io"

	"charm.land/lipgloss/v2"

	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
)

// renderRow renders the shared pipeline-run row (NAME/STATUS/PROJECT/PR/
// AUTHOR/TYPE/STARTED/DURATION) for `pipelinerun start` and `project build`:
// -o json emits the schemaVersion envelope, -o table (default) colours
// STATUS on a TTY via output.PipelineStatusColor.
func renderRow(ios *iostreams.IOStreams, format string, result *portal.StartResult) error {
	return output.RenderList(ios, format, result, func(isTTY bool) ([]string, [][]string) {
		status := output.OrDash(result.Status)
		if isTTY && result.Status != "" {
			status = output.PipelineStatusColor(result.Status)
		}

		row := []string{
			output.OrDash(result.Name),
			status,
			output.OrDash(result.Project),
			output.OrDash(result.PR),
			output.OrDash(result.Author),
			output.OrDash(result.Type),
			output.OrDash(result.Started),
			output.OrDash(result.Duration),
		}

		return Headers, [][]string{row}
	})
}

// renderDryRun emits YAML by default for direct use with `kubectl apply -f -`,
// or a schemaVersion-wrapped JSON envelope under -o json. format must already
// be restricted to "", "json", or "yaml" (see ValidateOutputAndDryRun).
func renderDryRun(out io.Writer, format string, manifest map[string]any) error {
	if len(manifest) == 0 {
		return fmt.Errorf("portal returned empty dry-run manifest")
	}

	if format == output.FormatJSON {
		return output.PrintJSONEnvelope(out, SchemaVersion, manifest)
	}

	return output.PrintYAML(out, manifest)
}

// PresentResult writes a start or build result: the dry-run manifest, the JSON
// envelope, or the table row followed by a stderr note that the controller may
// briefly 404 on the new run until its labels reconcile.
func PresentResult(ios *iostreams.IOStreams, format string, dryRun bool, result *portal.StartResult) error {
	if dryRun {
		return renderDryRun(ios.Out, format, result.DryRunManifest)
	}

	if output.ResolveFormat(format) == output.FormatJSON {
		return output.PrintJSONEnvelope(ios.Out, SchemaVersion, result)
	}

	if err := renderRow(ios, format, result); err != nil {
		return err
	}

	if result.Name != "" {
		msg := fmt.Sprintf(
			"note: the controller may briefly 404 on 'krci pipelinerun get %s' until labels reconcile",
			result.Name)
		_, _ = lipgloss.Fprintln(ios.ErrOut, output.DimStyle.Render(msg))
	}

	return nil
}
