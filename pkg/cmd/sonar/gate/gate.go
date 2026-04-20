// Package gate implements the "krci sonar gate" command.
package gate

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	sonarinternal "github.com/KubeRocketCI/cli/pkg/cmd/sonar/internal"
)

// GateOptions holds all inputs for `krci sonar gate <project>`.
type GateOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Project      string
	PullRequest  string
	OutputFormat string
}

// NewCmdGate returns the "sonar gate" cobra.Command.
func NewCmdGate(f *cmdutil.Factory, runF func(*GateOptions) error) *cobra.Command {
	opts := &GateOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
	}

	cmd := &cobra.Command{
		Use:   "gate <project>",
		Short: "Show the SonarQube quality gate for a project",
		Args: cmdutil.ExactArgs(1, "a SonarQube project key",
			"to see available projects: krci sonar list"),
		Example: `  # Default branch
  krci sonar gate payments-api

  # Pull-request gate check
  krci sonar gate payments-api --pull-request 123

  # CI guardrail
  if [ "$(krci sonar gate payments-api -o json | jq -r '.data.projectStatus.status')" = "ERROR" ]; then
    echo "gate failed"; exit 1
  fi`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Project = args[0]

			if err := sonarinternal.ValidateProjectCommand(cmd, opts.OutputFormat, opts.Project, opts.PullRequest); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return gateRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.PullRequest, "pull-request", "", "SonarQube pull-request id to scope the view")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func gateRun(ctx context.Context, opts *GateOptions) error {
	client, err := opts.RestClient()
	if err != nil {
		return sonarinternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewSonarService(client)

	g, err := svc.Gate(ctx, opts.Project, opts.PullRequest)
	if err != nil {
		return sonarinternal.HandleError(opts.IO, opts.OutputFormat, err)
	}

	return sonarinternal.Render(opts.IO, opts.OutputFormat, g, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, opts.Project, g)
	})
}

func renderTable(w io.Writer, isTTY bool, project string, g *portal.SonarGate) error {
	status := g.ProjectStatus.Status
	if status == "" {
		status = portal.QualityGateNone
	}

	displayStatus := string(status)
	if isTTY {
		displayStatus = output.SonarGateStatusColor(status)
	}

	if _, err := fmt.Fprintf(w, "Quality Gate: %s  (%s)\n\n", displayStatus, project); err != nil {
		return err
	}

	if len(g.ProjectStatus.Conditions) == 0 {
		_, err := fmt.Fprintln(w, "(no conditions — project has no analyses yet)")
		return err
	}

	headers := []string{"METRIC", "OPERATOR", "THRESHOLD", "ACTUAL", "STATUS"}
	rows := make([][]string, 0, len(g.ProjectStatus.Conditions))

	for _, c := range g.ProjectStatus.Conditions {
		s := string(c.Status)
		if isTTY {
			s = output.SonarGateStatusColor(c.Status)
		}

		rows = append(rows, []string{c.MetricKey, c.Comparator, c.ErrorThreshold, c.ActualValue, s})
	}

	return sonarinternal.PrintTable(w, isTTY, headers, rows)
}
