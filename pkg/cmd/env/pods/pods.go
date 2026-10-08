// Package pods implements the "krci env pods" command.
package pods

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KubeRocketCI/cli/internal/cmdutil"
	"github.com/KubeRocketCI/cli/internal/config"
	"github.com/KubeRocketCI/cli/internal/iostreams"
	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
	envinternal "github.com/KubeRocketCI/cli/pkg/cmd/env/internal"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
)

// PodsOptions holds all inputs for `krci env pods <deployment> <env>`.
type PodsOptions struct {
	IO           *iostreams.IOStreams
	RestClient   func() (*restapi.ClientWithResponses, error)
	Config       func() (*config.Config, error)
	Deployment   string
	Env          string
	OutputFormat string
}

// NewCmdPods returns the "env pods" cobra.Command.
// runF is the business-logic function; pass nil to use the default podsRun.
func NewCmdPods(f *cmdutil.Factory, runF func(*PodsOptions) error) *cobra.Command {
	opts := &PodsOptions{
		IO:         f.IOStreams,
		RestClient: f.RestClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "pods <deployment> <env>",
		Short: "List the pods of one environment",
		Long: `List the pods in the namespace of one (deployment, env) pair: the status,
ready containers and restarts kubectl prints for a pod, and for every
container its state with the reason, message and exit code, and how its
previous run ended. Shows why a project of "krci env get" is degraded: a
crash loop, an image that cannot be pulled, a pod that cannot be scheduled.

` + envinternal.TargetHelp + `

Limits:
  - The Portal reads the namespace with your session, so you need the right
    to list pods there.
  - The Portal reads only the cluster it runs on. An environment on another
    cluster is refused.
  - Every pod of the namespace is listed. PROJECT is set for a pod that
    carries the name of a project of the deployment in its
    ` + portal.PodProjectLabel + ` label.`,
		Args: envinternal.TargetArgs("pods"),
		Example: `  # Default
  krci env pods my-pipeline dev

  # JSON envelope (container states, conditions, owner, node)
  krci env pods my-pipeline dev -o json

  # Scripting — why is each container of a not-ready pod not running?
  krci env pods my-pipeline dev -o json |
    jq -r '.data.pods[] | select(.readyContainers < .totalContainers) | .name as $p |
           .containers[] | select(.state != "running") |
           "\($p)/\(.name): \(.state) \(.reason // "") \(.message // "")"'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Deployment = args[0]
			opts.Env = args[1]

			if err := envinternal.ValidateTarget(opts.OutputFormat, opts.Deployment, opts.Env); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return podsRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "",
		"Output format: table, json (default: table)")

	return cmd
}

func podsRun(ctx context.Context, opts *PodsOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	client, err := opts.RestClient()
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, err)
	}

	svc := portal.NewEnvService(client, cfg.ClusterName, cfg.Namespace)

	payload, err := svc.Pods(ctx, opts.Deployment, opts.Env)
	if err != nil {
		return discovery.HandleError(opts.IO, opts.OutputFormat, envinternal.MapNotFound(err, opts.Deployment, opts.Env))
	}

	if err := discovery.Render(opts.IO, opts.OutputFormat, payload, func(w io.Writer, isTTY bool) error {
		return renderTable(w, isTTY, payload.Pods)
	}); err != nil {
		return err
	}

	if len(payload.Pods) == 0 {
		return discovery.PrintEmptyNote(opts.IO, opts.OutputFormat,
			fmt.Sprintf("No pods found in namespace %s.", payload.Namespace))
	}

	return nil
}

func renderTable(w io.Writer, isTTY bool, pods []portal.EnvPod) error {
	headers := []string{"POD", "PROJECT", "STATUS", "READY", "RESTARTS", "CREATED"}
	rows := make([][]string, 0, len(pods))

	for _, p := range pods {
		rows = append(rows, []string{
			p.Name,
			discovery.OptCell(p.Project),
			statusCell(p, isTTY),
			fmt.Sprintf("%d/%d", p.ReadyContainers, p.TotalContainers),
			restartsCell(p),
			discovery.OptTimeCell(p.CreatedAt),
		})
	}

	if err := discovery.PrintTable(w, isTTY, headers, rows); err != nil {
		return err
	}

	return discovery.PrintSection(w, isTTY, "Details", detailLines(pods))
}

// statusCell colors the status on a TTY: green for a healthy pod, red for a
// Failed one, yellow otherwise.
func statusCell(p portal.EnvPod, isTTY bool) string {
	status := output.SingleLine(p.Status)

	switch {
	case !isTTY:
		return status
	case p.Healthy():
		return output.GreenText(status)
	case p.Phase == portal.PodPhaseFailed:
		return output.RedText(status)
	default:
		return output.YellowText(status)
	}
}

// restartsCell renders the restart count, followed by the time of the newest
// restart it counts.
func restartsCell(p portal.EnvPod) string {
	cell := strconv.Itoa(p.Restarts)
	if p.LastRestartAt == nil {
		return cell
	}

	return fmt.Sprintf("%s (%s)", cell, output.FormatRelativeTime(*p.LastRestartAt))
}

// detailLines flattens what the pods report beyond their status into the rows
// of the Details block:
//
//	"  - <pod>: <reason>: <message>"                         the pod carries its own reason or message
//	"  - <pod>: <condition> <status> (<reason>): <message>"  a pod that reports no container yet
//	"  - <pod>/<container>: <state> (<reason>), exit <code>: <message>; last termination: …"
//	                                                         a container that needs attention
//	"      image: <image>@<short digest>"                    second line of a container row
//	"      image: <image> (image ID <short image ID>)"       an image without a registry digest
//
// An init container is marked "(init)", a sidecar "(sidecar)". Every line passes
// output.SingleLine.
func detailLines(pods []portal.EnvPod) []string {
	lines := make([]string, 0, len(pods))
	row := func(subject, text string) string {
		return "  - " + output.SingleLine(subject+": "+text)
	}
	add := func(subject, text string) {
		lines = append(lines, row(subject, text))
	}

	for _, p := range pods {
		if p.Reason != nil || p.Message != nil {
			add(p.Name, withMessage(ptr.Deref(p.Reason, ""), p.Message))
		}

		if len(p.Containers) == 0 {
			for _, c := range p.Conditions {
				state := c.Type + " " + c.Status
				if c.Reason != nil {
					state += " (" + *c.Reason + ")"
				}

				add(p.Name, withMessage(state, c.Message))
			}
		}

		for _, c := range p.Containers {
			line := containerLine(c)
			if line == "" {
				continue
			}

			name := p.Name + "/" + c.Name

			switch {
			case c.Sidecar:
				name += " (sidecar)"
			case c.Init:
				name += " (init)"
			}

			entry := row(name, line)
			if ref := imageRef(c); ref != "" {
				entry += "\n      image: " + output.SingleLine(ref)
			}

			lines = append(lines, entry)
		}
	}

	return lines
}

// imageRef is the image the container asks for, without a digest it names,
// then "@" and the short registry digest, else the short local image ID; ""
// without an image.
func imageRef(c portal.PodContainer) string {
	if c.Image == "" {
		return ""
	}

	image, _, _ := strings.Cut(c.Image, "@")

	switch {
	case c.ImageDigest != nil:
		return image + "@" + discovery.ShortDigestCell(c.ImageDigest)
	case c.ImageID != nil:
		_, id, found := strings.Cut(*c.ImageID, "://")
		if !found {
			id = *c.ImageID
		}

		return image + " (image ID " + discovery.ShortDigestCell(&id) + ")"
	default:
		return image
	}
}

// containerLine describes a container that needs attention, "" for any other.
func containerLine(c portal.PodContainer) string {
	if !c.NeedsAttention() {
		return ""
	}

	exit := ptr.Deref(c.ExitCode, 0)

	state := c.State
	if c.FailsReadiness() {
		state += ", not ready"
	}

	if c.Reason != nil {
		state += " (" + *c.Reason + ")"
	}

	if exit != 0 {
		state += fmt.Sprintf(", exit %d", exit)
	}

	line := withMessage(state, c.Message)

	if t := c.LastTermination; t != nil {
		last := fmt.Sprintf("exit %d", t.ExitCode)
		if t.Reason != nil {
			last = *t.Reason + ", " + last
		}

		line += "; last termination: " + withMessage(last, t.Message)
	}

	return line
}

// withMessage joins a state with its optional message as "<state>: <message>".
func withMessage(state string, message *string) string {
	msg := ptr.Deref(message, "")

	switch {
	case msg == "":
		return state
	case state == "":
		return msg
	default:
		return state + ": " + msg
	}
}
