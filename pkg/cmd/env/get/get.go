// Package get implements the "krci env get" command.
package get

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	envinternal "github.com/KubeRocketCI/cli/pkg/cmd/env/internal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/render"
)

// GetOptions holds all inputs for `krci env get <deployment> <env>`.
type GetOptions struct {
	IO           *iostreams.IOStreams
	Services     func() (*portal.Services, error)
	Deployment   string
	Env          string
	OutputFormat string
}

// NewCmdGet returns the "env get" cobra.Command.
// runF is the business-logic function; pass nil to use the default getRun.
func NewCmdGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO:       f.IOStreams,
		Services: f.Services,
	}

	cmd := &cobra.Command{
		Use:   "get <deployment> <env>",
		Short: "Show full detail for one environment",
		Long: `Show the full detail of one (deployment, env) pair: infrastructure
(cluster, namespace, trigger type, deploy/clean pipelines), quality gates,
and the projects deployed there with health, sync, version, image digest,
ingress URLs, ArgoCD link, and last-deployed timestamp.

` + envinternal.TargetHelp,
		Args: envinternal.TargetArgs("get"),
		Example: `  # Default
  krci env get my-pipeline prod

  # JSON envelope (full image digest, full ingress URLs)
  krci env get my-pipeline prod -o json

  # Scripting — list all degraded projects in this env
  krci env get my-pipeline prod -o json |
    jq -r '.data.projects[] | select(.status=="degraded") | .name'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Deployment = args[0]
			opts.Env = args[1]

			if err := envinternal.ValidateTarget(opts.OutputFormat, opts.Deployment, opts.Env); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return getRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		render.OutputFlagUsage)

	return cmd
}

func getRun(ctx context.Context, opts *GetOptions) error {
	services, err := opts.Services()
	if err != nil {
		return render.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := services.Env()

	detail, err := svc.Get(ctx, opts.Deployment, opts.Env)
	if err != nil {
		return render.HandleError(opts.IO, opts.OutputFormat, envinternal.MapNotFound(err, opts.Deployment, opts.Env))
	}

	return render.Render(opts.IO, opts.OutputFormat, detail, func(w io.Writer, isTTY bool) error {
		return renderDetail(w, isTTY, detail)
	})
}

func renderDetail(w io.Writer, isTTY bool, d *portal.EnvDetail) error {
	headerPairs := buildHeaderPairs(d)
	infraPairs := buildInfraPairs(d.Infrastructure)

	// Shared label-column width so the header and infrastructure blocks line
	// up across the section break (e.g. "Description" and "Deploy Pipeline"
	// padded to the same column).
	labelWidth := output.LabelWidth(headerPairs, infraPairs)

	if err := output.PrintAligned(w, isTTY, headerPairs, "", labelWidth); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := output.PrintHeading(w, isTTY, "Infrastructure"); err != nil {
		return err
	}

	if err := output.PrintAligned(w, isTTY, infraPairs, "  ", labelWidth); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := renderQualityGates(w, isTTY, d.QualityGates); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := renderProjects(w, isTTY, d.Projects); err != nil {
		return err
	}

	return renderConditions(w, isTTY, d.Projects)
}

func buildHeaderPairs(d *portal.EnvDetail) []output.DetailLine {
	desc := "—"
	if d.Description != nil {
		desc = *d.Description
	}

	pairs := []output.DetailLine{
		{Label: "Environment:", Value: d.Env},
		{Label: "Deployment:", Value: d.Deployment},
		{Label: "Status:", Value: d.Status, Styled: output.StatusColor(d.Status)},
	}

	if d.DetailedMessage != nil && *d.DetailedMessage != "" {
		pairs = append(pairs, output.DetailLine{Label: "Message:", Value: output.SingleLine(*d.DetailedMessage)})
	}

	return append(pairs,
		output.DetailLine{Label: "Description:", Value: desc},
		output.DetailLine{Label: "Order:", Value: strconv.Itoa(d.Order)},
	)
}

// renderConditions prints the Conditions block below the projects table: one
// line per Argo CD condition and one per sync operation that did not succeed,
// so the reason behind an unknown or degraded status is readable without
// opening Argo CD. Nothing is printed when no project carries a diagnostic.
func renderConditions(w io.Writer, isTTY bool, projects []portal.EnvProject) error {
	return discovery.PrintSection(w, isTTY, "Conditions", conditionLines(projects))
}

// conditionLines flattens the diagnostics of every project into display rows:
//
//	"  - <project>: <condition type>: <message>"
//	"  - <project>: operation <phase>: <message>"
//
// The operation line appears only for a phase other than Succeeded.
func conditionLines(projects []portal.EnvProject) []string {
	lines := make([]string, 0, len(projects))

	for _, p := range projects {
		for _, c := range p.Conditions {
			lines = append(lines, discovery.Bullet(fmt.Sprintf("%s: %s: %s", p.Name, c.Type, c.Message)))
		}

		op := p.Operation
		if op == nil || op.Phase == "" || op.Phase == "Succeeded" {
			continue
		}

		line := fmt.Sprintf("%s: operation %s", p.Name, op.Phase)
		if op.Message != nil && *op.Message != "" {
			line += ": " + *op.Message
		}

		lines = append(lines, discovery.Bullet(line))
	}

	return lines
}

func buildInfraPairs(i portal.Infrastructure) []output.DetailLine {
	clean := "—"
	if i.CleanPipeline != nil {
		clean = *i.CleanPipeline
	}

	return []output.DetailLine{
		{Label: "Cluster:", Value: i.Cluster},
		{Label: "Namespace:", Value: i.Namespace},
		{Label: "Trigger Type:", Value: i.TriggerType},
		{Label: "Deploy Pipeline:", Value: i.DeployPipeline},
		{Label: "Clean Pipeline:", Value: clean},
	}
}

func renderQualityGates(w io.Writer, isTTY bool, gates []portal.QualityGateDetail) error {
	if err := output.PrintHeading(w, isTTY, fmt.Sprintf("Quality Gates (%d)", len(gates))); err != nil {
		return err
	}

	if len(gates) == 0 {
		_, err := fmt.Fprintln(w, "  (none)")
		return err
	}

	for _, g := range gates {
		if _, err := fmt.Fprintln(w, discovery.Bullet(formatQualityGateLine(g))); err != nil {
			return err
		}
	}

	return nil
}

// formatQualityGateLine returns the per-gate display text used by `env get`:
//
//	"<type>: <stepName-or-autotestName>[ (branch: <branchName>)]"
//
// AutotestName, when present, replaces stepName as the value (matches the
// portal "autotests: smoke-tests (branch: main)" rendering).
func formatQualityGateLine(g portal.QualityGateDetail) string {
	value := g.StepName
	if g.AutotestName != nil && *g.AutotestName != "" {
		value = *g.AutotestName
	}

	line := fmt.Sprintf("%s: %s", g.Type, value)

	if g.BranchName != nil && *g.BranchName != "" {
		line += fmt.Sprintf(" (branch: %s)", *g.BranchName)
	}

	return line
}

func renderProjects(w io.Writer, isTTY bool, projects []portal.EnvProject) error {
	if err := output.PrintHeading(w, isTTY, fmt.Sprintf("Projects (%d)", len(projects))); err != nil {
		return err
	}

	if len(projects) == 0 {
		_, err := fmt.Fprintln(w, "  (none)")
		return err
	}

	headers := []string{"PROJECT", "STATUS", "SYNC", "VERSION", "IMAGE_SHA", "INGRESS"}
	rows := make([][]output.Cell, 0, len(projects))

	for _, p := range projects {
		cells := []output.Cell{
			output.Text(p.Name),
			discovery.OptStatusCell(p.Status),
			output.Text(discovery.OptCell(p.Sync)),
			output.Text(discovery.OptCell(p.Version)),
			output.Text(discovery.ShortDigestCell(p.ImageDigest)),
		}
		rows = append(rows, discovery.ExpandIngressRows(cells, discovery.IngressLines(p.IngressURLs))...)
	}

	return output.PrintTable(w, isTTY, headers, rows)
}
