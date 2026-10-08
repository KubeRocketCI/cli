// Package events implements the "krci env events" command.
package events

import (
	"context"
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
	envinternal "github.com/KubeRocketCI/cli/pkg/cmd/env/internal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
)

// EventsOptions holds all inputs for `krci env events <deployment> <env>`.
type EventsOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Deployment   string
	Env          string
	Pod          string
	Warnings     bool
	OutputFormat string
}

// NewCmdEvents returns the "env events" cobra.Command.
// runF is the business-logic function; pass nil to use the default eventsRun.
func NewCmdEvents(f *cmdutil.Factory, runF func(*EventsOptions) error) *cobra.Command {
	opts := &EventsOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "events <deployment> <env>",
		Short: "List the Kubernetes events of one environment",
		Long: `List the Kubernetes events in the namespace of one (deployment, env) pair,
newest first: what the scheduler, the kubelet and the controllers reported
about its pods and workloads, such as a failed image pull, a failed probe, a
back-off, or a pod a quota did not admit.

` + envinternal.TargetHelp + `

Limits:
  - Kubernetes keeps an event for a limited time, one hour by default. An
    environment that broke earlier may have none left; "krci env pods" still
    shows the state of its containers.
  - The Portal reads the namespace with your session, so you need the right
    to list events there.
  - The Portal reads only the cluster it runs on. An environment on another
    cluster is refused.`,
		Args: envinternal.TargetArgs("events"),
		Example: `  # Default
  krci env events my-pipeline dev

  # Only what went wrong
  krci env events my-pipeline dev --warnings

  # The events of one pod (take the name from "krci env pods")
  krci env events my-pipeline dev --pod payments-api-6c9f7d9b8-x2x9k

  # JSON envelope for scripting
  krci env events my-pipeline dev --warnings -o json |
    jq -r '.data.events[] | "\(.involvedObject.kind)/\(.involvedObject.name): \(.reason): \(.message)"'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Deployment = args[0]
			opts.Env = args[1]

			if err := envinternal.ValidateTarget(opts.OutputFormat, opts.Deployment, opts.Env); err != nil {
				return err
			}

			if cmd.Flags().Changed("pod") {
				if err := cmdutil.ValidateK8sSubdomain("--pod", opts.Pod); err != nil {
					return err
				}
			}

			if runF != nil {
				return runF(opts)
			}

			return eventsRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Pod, "pod", "",
		"Show only the events about the pod of this name")
	cmd.Flags().BoolVar(&opts.Warnings, "warnings", false,
		"Show only the events of type "+portal.EventTypeWarning)
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func eventsRun(ctx context.Context, opts *EventsOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	client, err := opts.RestClient()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewEnvService(client, cfg.ClusterName, cfg.Namespace)

	payload, err := svc.Events(ctx, opts.Deployment, opts.Env, portal.EnvEventFilters{
		Pod:          opts.Pod,
		WarningsOnly: opts.Warnings,
	})
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, envinternal.MapNotFound(err, opts.Deployment, opts.Env))
	}

	if err := discovery.Render(opts.IO, opts.OutputFormat, payload, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, payload.Events)
	}); err != nil {
		return err
	}

	if len(payload.Events) == 0 {
		return discovery.PrintEmptyNote(opts.IO, opts.OutputFormat,
			fmt.Sprintf("No events found in namespace %s.", payload.Namespace))
	}

	return nil
}

func renderTable(w io.Writer, isTTY bool, events []portal.EnvEvent) error {
	headers := []string{"FIRST_SEEN", "LAST_SEEN", "TYPE", "REASON", "OBJECT", "COUNT", "MESSAGE"}
	rows := make([][]string, 0, len(events))

	for _, e := range events {
		rows = append(rows, eventRow(e, isTTY))
	}

	return discovery.PrintTable(w, isTTY, headers, rows)
}

// eventRow passes every text cell through output.SingleLine. The Warning color
// and the MESSAGE truncation apply on a TTY only.
func eventRow(e portal.EnvEvent, isTTY bool) []string {
	eventType, msg := output.SingleLine(e.Type), output.SingleLine(e.Message)
	if isTTY {
		msg = output.Truncate(msg, output.MaxMessageLen)

		if e.Type == portal.EventTypeWarning {
			eventType = output.YellowText(eventType)
		}
	}

	return []string{
		discovery.OptTimeCell(e.FirstSeen),
		discovery.OptTimeCell(e.LastSeen),
		eventType,
		output.SingleLine(e.Reason),
		output.SingleLine(e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name),
		strconv.Itoa(e.Count),
		msg,
	}
}
