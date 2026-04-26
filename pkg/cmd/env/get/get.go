// Package get implements the "krci env get" command.
package get

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
)

// GetOptions holds all inputs for `krci env get <deployment> <env>`.
type GetOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Deployment   string
	Env          string
	OutputFormat string
}

// NewCmdGet returns the "env get" cobra.Command.
// runF is the business-logic function; pass nil to use the default getRun.
func NewCmdGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "get <deployment> <env>",
		Short: "Show full detail for one environment",
		Long: `Show the full detail of one (deployment, env) pair: infrastructure
(cluster, namespace, trigger type, deploy/clean pipelines), quality gates,
and the projects deployed there with health, sync, version, image digest,
ingress URLs, ArgoCD link, and last-deployed timestamp.

<deployment> is the parent CDPipeline name. <env> is Stage.spec.name (the
short user-facing identifier such as "dev", "stage", "prod").`,
		Args: cmdutil.ExactArgs(2,
			"a deployment and an env (e.g. krci env get my-pipeline prod)",
			"to see available environments: krci env list"),
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

			if err := discovery.ValidateOutputFormat(opts.OutputFormat); err != nil {
				return err
			}

			if err := discovery.ValidateDeployment(opts.Deployment); err != nil {
				return err
			}

			if err := discovery.ValidateEnv(opts.Env); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return getRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func getRun(ctx context.Context, opts *GetOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	client, err := opts.RestClient()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewEnvService(client, cfg.ClusterName, cfg.Namespace)

	detail, err := svc.Get(ctx, opts.Deployment, opts.Env)
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, mapNotFound(err, opts.Deployment, opts.Env))
	}

	return discovery.Render(opts.IO, opts.OutputFormat, detail, func(w io.Writer, isTTY bool) error {
		return renderDetail(w, isTTY, detail)
	})
}

// mapNotFound rewrites the typed not-found sentinels from EnvService.Get into
// the precise user-facing message the spec requires (M4 scenarios). Other
// errors pass through unchanged.
func mapNotFound(err error, deployment, env string) error {
	switch {
	case errors.Is(err, portal.ErrEnvNotFound):
		return fmt.Errorf("environment %q not found in deployment %q", env, deployment)
	case errors.Is(err, portal.ErrDeploymentNotFound):
		return fmt.Errorf("deployment %q not found", deployment)
	default:
		return err
	}
}

func renderDetail(w io.Writer, isTTY bool, d *portal.EnvDetail) error {
	headerPairs := buildHeaderPairs(d, isTTY)
	infraPairs := buildInfraPairs(d.Infrastructure)

	// Shared label-column width so the header and infrastructure blocks line
	// up across the section break (e.g. "Description" and "Deploy Pipeline"
	// padded to the same column).
	labelWidth := maxLabelWidth(headerPairs, infraPairs)

	if err := printAligned(w, isTTY, headerPairs, "", labelWidth); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := printSectionHeading(w, isTTY, "Infrastructure"); err != nil {
		return err
	}

	if err := printAligned(w, isTTY, infraPairs, "  ", labelWidth); err != nil {
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

	return renderProjects(w, isTTY, d.Projects)
}

func buildHeaderPairs(d *portal.EnvDetail, isTTY bool) []labelValue {
	desc := "—"
	if d.Description != nil {
		desc = *d.Description
	}

	statusValue := d.Status
	if isTTY {
		statusValue = output.StatusColor(d.Status)
	}

	return []labelValue{
		{Label: "Environment", Value: d.Env},
		{Label: "Deployment", Value: d.Deployment},
		{Label: "Status", Value: statusValue},
		{Label: "Description", Value: desc},
		{Label: "Order", Value: strconv.Itoa(d.Order)},
	}
}

func buildInfraPairs(i portal.Infrastructure) []labelValue {
	clean := "—"
	if i.CleanPipeline != nil {
		clean = *i.CleanPipeline
	}

	return []labelValue{
		{Label: "Cluster", Value: i.Cluster},
		{Label: "Namespace", Value: i.Namespace},
		{Label: "Trigger Type", Value: i.TriggerType},
		{Label: "Deploy Pipeline", Value: i.DeployPipeline},
		{Label: "Clean Pipeline", Value: clean},
	}
}

func maxLabelWidth(groups ...[]labelValue) int {
	m := 0
	for _, g := range groups {
		for _, p := range g {
			if len(p.Label) > m {
				m = len(p.Label)
			}
		}
	}
	return m
}

func renderQualityGates(w io.Writer, isTTY bool, gates []portal.QualityGateDetail) error {
	if err := printSectionHeading(w, isTTY, fmt.Sprintf("Quality Gates (%d)", len(gates))); err != nil {
		return err
	}

	if len(gates) == 0 {
		_, err := fmt.Fprintln(w, "  (none)")
		return err
	}

	for _, g := range gates {
		if _, err := fmt.Fprintln(w, formatQualityGateLine(g)); err != nil {
			return err
		}
	}

	return nil
}

// formatQualityGateLine returns the per-gate display row used by `env get`:
//
//	"  - <type>: <stepName-or-autotestName>[ (branch: <branchName>)]"
//
// AutotestName, when present, replaces stepName as the value (matches the
// portal "autotests: smoke-tests (branch: main)" rendering).
func formatQualityGateLine(g portal.QualityGateDetail) string {
	value := g.StepName
	if g.AutotestName != nil && *g.AutotestName != "" {
		value = *g.AutotestName
	}

	line := fmt.Sprintf("  - %s: %s", g.Type, value)

	if g.BranchName != nil && *g.BranchName != "" {
		line += fmt.Sprintf(" (branch: %s)", *g.BranchName)
	}

	return line
}

func renderProjects(w io.Writer, isTTY bool, projects []portal.EnvProject) error {
	if err := printSectionHeading(w, isTTY, fmt.Sprintf("Projects (%d)", len(projects))); err != nil {
		return err
	}

	if len(projects) == 0 {
		_, err := fmt.Fprintln(w, "  (none)")
		return err
	}

	headers := []string{"PROJECT", "STATUS", "SYNC", "VERSION", "IMAGE_SHA", "INGRESS"}
	rows := make([][]string, 0, len(projects))

	for _, p := range projects {
		cells := []string{
			p.Name,
			discovery.OptStatusCell(p.Status, isTTY),
			discovery.OptCell(p.Sync),
			discovery.OptCell(p.Version),
			discovery.ShortDigestCell(p.ImageDigest),
		}
		rows = append(rows, discovery.ExpandIngressRows(cells, discovery.IngressLines(p.IngressURLs, isTTY))...)
	}

	return discovery.PrintTable(w, isTTY, headers, rows)
}

// printSectionHeading writes a section heading: lipgloss-styled when isTTY,
// "<text>:" otherwise. Centralizes the styled/plain branching that every
// detail section shares.
func printSectionHeading(w io.Writer, isTTY bool, text string) error {
	if isTTY {
		_, err := fmt.Fprintln(w, output.HeaderStyle.Render(text))
		return err
	}

	_, err := fmt.Fprintln(w, text+":")
	return err
}

// labelValue is one aligned "label: value" row in the detail block.
type labelValue struct {
	Label string
	Value string
}

// envLabelStyle reuses the shared LabelStyle but lets printAligned control
// the column width dynamically via its own padding logic.
var envLabelStyle = output.LabelStyle.UnsetWidth()

// printAligned prints "<indent><label padded to width>   <value>" for each
// pair. The width parameter lets callers force a column wider than this
// block's own labels so sibling blocks (e.g. header + infrastructure) align
// across a section break.
func printAligned(w io.Writer, isTTY bool, pairs []labelValue, indent string, width int) error {
	for _, p := range pairs {
		pad := width - len(p.Label)

		label := p.Label + ":"
		if isTTY {
			label = envLabelStyle.Render(label)
		}

		if _, err := fmt.Fprintf(w, "%s%s%*s   %s\n", indent, label, pad, "", p.Value); err != nil {
			return err
		}
	}

	return nil
}
