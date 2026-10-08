package events

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/ptr"
	"github.com/KubeRocketCI/cli/pkg/cmd/env/internal/envtestutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
)

const namespaceEvents = `{
  "metadata": {"name": "foo-6c9f7d9b8-x2x9k.1"},
  "type": "Warning", "reason": "BackOff", "count": 42,
  "message": "Back-off restarting failed container foo in pod foo-6c9f7d9b8-x2x9k",
  "involvedObject": {"kind": "Pod", "name": "foo-6c9f7d9b8-x2x9k"},
  "firstTimestamp": "2026-01-02T03:04:05Z", "lastTimestamp": "2026-01-02T03:10:00Z",
  "source": {"component": "kubelet"}
},{
  "metadata": {"name": "foo-6c9f7d9b8.1"},
  "type": "Normal", "reason": "SuccessfulCreate",
  "message": "Created pod: foo-6c9f7d9b8-x2x9k",
  "involvedObject": {"kind": "ReplicaSet", "name": "foo-6c9f7d9b8"},
  "firstTimestamp": "2026-01-02T03:04:00Z", "lastTimestamp": "2026-01-02T03:04:00Z",
  "source": {"component": "replicaset-controller"}
}`

func TestEvents_RejectsInvalidInput(t *testing.T) {
	t.Parallel()

	envtestutil.CheckInvalidTarget(t, NewCmdEvents)

	cases := map[string]struct {
		pod  string
		want string
	}{
		"not a name":     {pod: "Bad_Pod", want: "--pod must be a valid DNS-1123 name"},
		"a kind prefix":  {pod: "pod/foo-6c9f7d9b8-x2x9k", want: "--pod must be a valid DNS-1123 name"},
		"an empty value": {pod: "", want: "--pod must not be empty"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts, err := cmdtest.RunCmd(t, NewCmdEvents, []string{envtestutil.Deployment, envtestutil.Env, "--pod", tc.pod})
			if err == nil || !strings.Contains(err.Error(), tc.want) || opts != nil {
				t.Errorf("error = %v, want it to contain %q before the run function", err, tc.want)
			}
		})
	}
}

func TestEvents_AcceptsValidArgs(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdEvents,
		[]string{"my-pipeline", "prod", "--pod", "my.app-6c9f7d9b8-x2x9k", "--warnings", "-o", "json"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if opts == nil || opts.Deployment != "my-pipeline" || opts.Env != "prod" || opts.OutputFormat != "json" ||
		opts.Pod != "my.app-6c9f7d9b8-x2x9k" || !opts.Warnings {
		t.Errorf("options = %+v", opts)
	}
}

func TestEventsRun_JSONEnvelope(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		flags []string
		want  []string
	}{
		"every event":       {want: []string{"BackOff", "SuccessfulCreate"}},
		"warnings":          {flags: []string{"--warnings"}, want: []string{"BackOff"}},
		"one pod":           {flags: []string{"--pod", "foo-6c9f7d9b8-x2x9k"}, want: []string{"BackOff"}},
		"a pod without any": {flags: []string{"--pod", "gone"}, want: []string{}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f, stdout := cmdtest.NewPortalFactory(t, envtestutil.Portal(envtestutil.LocalCluster, "Event", namespaceEvents))

			args := append([]string{envtestutil.Deployment, envtestutil.Env, "-o", "json"}, tc.flags...)
			if err := cmdtest.Execute(NewCmdEvents, f, args); err != nil {
				t.Fatalf("Execute error: %v", err)
			}

			var env struct {
				SchemaVersion string                  `json:"schemaVersion"`
				Data          portal.EnvEventsPayload `json:"data"`
			}

			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("stdout is not the JSON envelope: %v\n%s", err, stdout)
			}

			want := portal.EnvNamespace{
				Deployment: envtestutil.Deployment, Env: envtestutil.Env,
				Cluster: envtestutil.LocalCluster, Namespace: envtestutil.Namespace,
			}
			if env.SchemaVersion != discovery.SchemaVersion || env.Data.EnvNamespace != want {
				t.Errorf("envelope = %+v", env)
			}

			reasons := make([]string, 0, len(env.Data.Events))
			for _, e := range env.Data.Events {
				reasons = append(reasons, e.Reason)
			}

			if !slices.Equal(reasons, tc.want) {
				t.Errorf("reasons = %v, want %v", reasons, tc.want)
			}

			keys := []string{`"events": []`}
			if len(tc.want) > 0 {
				keys = []string{
					`"type": "Warning"`, `"reason": "BackOff"`, `"involvedObject": {`, `"kind": "Pod"`, `"count": 42`,
					`"firstSeen": "2026-01-02T03:04:05Z"`, `"lastSeen": "2026-01-02T03:10:00Z"`, `"source": "kubelet"`,
				}
			}

			for _, key := range keys {
				if !strings.Contains(stdout.String(), key) {
					t.Errorf("stdout misses %s:\n%s", key, stdout)
				}
			}

			if stderr := cmdtest.Stderr(f); stderr != "" {
				t.Errorf("-o json keeps stderr silent, got %q", stderr)
			}
		})
	}
}

func TestEventsRun_Table(t *testing.T) {
	t.Parallel()

	f, stdout := cmdtest.NewPortalFactory(t, envtestutil.Portal(envtestutil.LocalCluster, "Event", namespaceEvents))

	if err := cmdtest.Execute(NewCmdEvents, f, []string{envtestutil.Deployment, envtestutil.Env}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	want := `FIRST_SEEN     LAST_SEEN      TYPE      REASON             OBJECT                     COUNT   MESSAGE
Jan 02 03:04   Jan 02 03:10   Warning   BackOff            Pod/foo-6c9f7d9b8-x2x9k    42      Back-off restarting failed container foo in pod foo-6c9f7d9b8-x2x9k
Jan 02 03:04   Jan 02 03:04   Normal    SuccessfulCreate   ReplicaSet/foo-6c9f7d9b8   1       Created pod: foo-6c9f7d9b8-x2x9k
`

	if got := stdout.String(); got != want {
		t.Errorf("table mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestEventsRun_NoEvents(t *testing.T) {
	t.Parallel()

	f, _ := cmdtest.NewPortalFactory(t, envtestutil.Portal(envtestutil.LocalCluster, "Event", namespaceEvents))

	err := cmdtest.Execute(NewCmdEvents, f, []string{envtestutil.Deployment, envtestutil.Env, "--pod", "gone"})
	if err != nil {
		t.Fatalf("no matching event is a success, got %v", err)
	}

	if want := "No events found in namespace " + envtestutil.Namespace + ".\n"; cmdtest.Stderr(f) != want {
		t.Errorf("stderr = %q, want %q", cmdtest.Stderr(f), want)
	}
}

func TestEventsRun_Refusals(t *testing.T) {
	t.Parallel()

	envtestutil.CheckNotFound(t, NewCmdEvents, "Event", namespaceEvents)
	envtestutil.CheckRemoteCluster(t, NewCmdEvents, "Event", namespaceEvents)
}

func TestEventRow(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("слово ", 40)
	warning := portal.EnvEvent{
		Type: portal.EventTypeWarning, Reason: "Unhealthy\x1b[2J", Count: 3, Message: "Readiness probe failed:\n\x1b]0;title\x07" + long,
		InvolvedObject: portal.EventObject{Kind: "Pod", Name: "foo"},
		FirstSeen:      ptr.To("2026-01-02T03:04:00Z"), LastSeen: ptr.To("2026-01-02T03:10:00Z"),
	}
	fullMessage := "Readiness probe failed: ]0;title" + strings.TrimSpace(long)

	piped := eventRow(warning, false)
	if want := []string{"Jan 02 03:04", "Jan 02 03:10", portal.EventTypeWarning, "Unhealthy[2J", "Pod/foo", "3", fullMessage}; !slices.Equal(piped, want) {
		t.Errorf("piped row = %q, want %q", piped, want)
	}

	tty := eventRow(warning, true)
	if tty[2] != output.YellowText(portal.EventTypeWarning) {
		t.Errorf("a Warning is yellow on a TTY, got %q", tty[2])
	}

	if tty[6] != output.Truncate(fullMessage, output.MaxMessageLen) || utf8.RuneCountInString(tty[6]) != output.MaxMessageLen {
		t.Errorf("the message is truncated on a TTY, got %q", tty[6])
	}

	normal := eventRow(portal.EnvEvent{Type: "Normal", Reason: "Pulled"}, true)
	if normal[0] != output.EmptyCell || normal[1] != output.EmptyCell || normal[2] != "Normal" {
		t.Errorf("a Normal event without a time = %q", normal)
	}
}
