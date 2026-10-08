package pods

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/output"
	"github.com/KubeRocketCI/cli/internal/portal"
	"github.com/KubeRocketCI/cli/internal/ptr"
	"github.com/KubeRocketCI/cli/pkg/cmd/env/internal/envtestutil"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/cmdtest"
	"github.com/KubeRocketCI/cli/pkg/cmd/internal/discovery"
)

const crashingPod = `{
  "metadata": {
    "name": "foo-6c9f7d9b8-x2x9k", "creationTimestamp": "2026-01-02T03:04:05Z",
    "labels": {"app.kubernetes.io/instance": "foo"},
    "ownerReferences": [{"kind": "ReplicaSet", "name": "foo-6c9f7d9b8", "controller": true}]
  },
  "spec": {"nodeName": "node-1", "containers": [{"name": "foo", "image": "registry.example.com/ns/foo:1.2.0"}]},
  "status": {
    "phase": "Running",
    "containerStatuses": [{
      "name": "foo", "imageID": "registry.example.com/ns/foo@sha256:abc12345", "ready": false, "restartCount": 7,
      "state": {"waiting": {"reason": "CrashLoopBackOff", "message": "back-off 5m0s restarting failed container=foo"}},
      "lastState": {"terminated": {"reason": "Error", "exitCode": 1, "finishedAt": "2026-01-02T03:10:00Z"}}
    }]
  }
}`

func TestPods_RejectsInvalidInput(t *testing.T) {
	t.Parallel()

	envtestutil.CheckInvalidTarget(t, NewCmdPods)
}

func TestPods_AcceptsValidArgs(t *testing.T) {
	t.Parallel()

	opts, err := cmdtest.RunCmd(t, NewCmdPods, []string{"my-pipeline", "prod", "-o", "json"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if opts == nil || opts.Deployment != "my-pipeline" || opts.Env != "prod" || opts.OutputFormat != "json" {
		t.Errorf("options = %+v", opts)
	}
}

func TestPodsRun_JSONEnvelope(t *testing.T) {
	t.Parallel()

	f, stdout := cmdtest.NewPortalFactory(t, envtestutil.Portal(envtestutil.LocalCluster, "Pod", crashingPod))

	if err := cmdtest.Execute(NewCmdPods, f, []string{envtestutil.Deployment, envtestutil.Env, "-o", "json"}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var env struct {
		SchemaVersion string                `json:"schemaVersion"`
		Data          portal.EnvPodsPayload `json:"data"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not the JSON envelope: %v\n%s", err, stdout)
	}

	if env.SchemaVersion != discovery.SchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", env.SchemaVersion, discovery.SchemaVersion)
	}

	want := portal.EnvNamespace{
		Deployment: envtestutil.Deployment, Env: envtestutil.Env,
		Cluster: envtestutil.LocalCluster, Namespace: envtestutil.Namespace,
	}
	if env.Data.EnvNamespace != want {
		t.Errorf("namespace = %+v, want %+v", env.Data.EnvNamespace, want)
	}

	if len(env.Data.Pods) != 1 {
		t.Fatalf("got %d pods, want 1: %s", len(env.Data.Pods), stdout)
	}

	pod := env.Data.Pods[0]
	if pod.Name != "foo-6c9f7d9b8-x2x9k" || pod.Status != "CrashLoopBackOff" || ptr.Deref(pod.Project, "") != "foo" {
		t.Errorf("pod = %+v", pod)
	}

	for _, want := range []string{
		`"deployment": "my-pipeline"`, `"namespace": "my-pipeline-dev"`, `"readyContainers": 0`, `"totalContainers": 1`,
		`"owner": {`, `"image": "registry.example.com/ns/foo:1.2.0"`, `"imageDigest": "sha256:abc12345"`,
		`"lastTermination": {`, `"exitCode": 1`, `"conditions": []`, `"reason": null`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout misses %s:\n%s", want, stdout)
		}
	}
}

func TestPodsRun_Table(t *testing.T) {
	t.Parallel()

	f, stdout := cmdtest.NewPortalFactory(t, envtestutil.Portal(envtestutil.LocalCluster, "Pod", crashingPod))

	if err := cmdtest.Execute(NewCmdPods, f, []string{envtestutil.Deployment, envtestutil.Env}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	want := `POD                   PROJECT   STATUS             READY   RESTARTS           CREATED
foo-6c9f7d9b8-x2x9k   foo       CrashLoopBackOff   0/1     7 (Jan 02 03:10)   Jan 02 03:04

Details (1):
  - foo-6c9f7d9b8-x2x9k/foo: waiting (CrashLoopBackOff): back-off 5m0s restarting failed container=foo; last termination: Error, exit 1
      image: registry.example.com/ns/foo:1.2.0@sha256:abc12345
`

	if got := stdout.String(); got != want {
		t.Errorf("table mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}

	if stderr := cmdtest.Stderr(f); stderr != "" {
		t.Errorf("stderr must stay empty when pods exist, got %q", stderr)
	}
}

func TestPodsRun_NoPods(t *testing.T) {
	t.Parallel()

	t.Run("table", func(t *testing.T) {
		t.Parallel()

		f, stdout := cmdtest.NewPortalFactory(t, envtestutil.Portal(envtestutil.LocalCluster, "Pod", ""))

		if err := cmdtest.Execute(NewCmdPods, f, []string{envtestutil.Deployment, envtestutil.Env}); err != nil {
			t.Fatalf("an empty namespace is a success, got %v", err)
		}

		if want := "No pods found in namespace " + envtestutil.Namespace + ".\n"; cmdtest.Stderr(f) != want {
			t.Errorf("stderr = %q, want %q", cmdtest.Stderr(f), want)
		}

		if strings.Contains(stdout.String(), "Details") {
			t.Errorf("no Details block without pods:\n%s", stdout)
		}
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()

		f, stdout := cmdtest.NewPortalFactory(t, envtestutil.Portal(envtestutil.LocalCluster, "Pod", ""))

		if err := cmdtest.Execute(NewCmdPods, f, []string{envtestutil.Deployment, envtestutil.Env, "-o", "json"}); err != nil {
			t.Fatalf("an empty namespace is a success, got %v", err)
		}

		if !strings.Contains(stdout.String(), `"pods": []`) || cmdtest.Stderr(f) != "" {
			t.Errorf("want pods:[] and a silent stderr, got stdout %s stderr %q", stdout, cmdtest.Stderr(f))
		}
	})
}

func TestPodsRun_Refusals(t *testing.T) {
	t.Parallel()

	envtestutil.CheckNotFound(t, NewCmdPods, "Pod", crashingPod)
	envtestutil.CheckRemoteCluster(t, NewCmdPods, "Pod", crashingPod)
}

func TestDetailLines(t *testing.T) {
	t.Parallel()

	pods := []portal.EnvPod{
		{
			Name:       "healthy",
			Containers: []portal.PodContainer{{Name: "app", Ready: true, State: portal.ContainerRunning}},
		},
		{
			Name:    "evicted",
			Reason:  ptr.To("Evicted"),
			Message: ptr.To("The node was low on resource: memory."),
			Conditions: []portal.PodCondition{
				{Type: "Ready", Status: "False", Reason: ptr.To("PodFailed")},
			},
			Containers: []portal.PodContainer{
				{Name: "app", State: portal.ContainerTerminated, Reason: ptr.To("Error"), ExitCode: ptr.To(137)},
			},
		},
		{
			Name: "pending",
			Conditions: []portal.PodCondition{
				{
					Type: "PodScheduled", Status: "False", Reason: ptr.To("Unschedulable"),
					Message: ptr.To("0/3 nodes are available:\n3 Insufficient memory."),
				},
				{Type: "Custom", Status: "Unknown"},
			},
			Containers: []portal.PodContainer{},
		},
		{
			Name: "mixed",
			Containers: []portal.PodContainer{
				{Name: "migrate", Init: true, State: portal.ContainerTerminated, Reason: ptr.To("Completed"), ExitCode: ptr.To(0)},
				{Name: "fetch", Init: true, State: portal.ContainerRunning},
				{Name: "proxy", Init: true, Sidecar: true, State: portal.ContainerRunning},
				{Name: "mesh", Init: true, Sidecar: true, Ready: true, State: portal.ContainerRunning},
				{Name: "probe", State: portal.ContainerRunning},
				{
					Name: "pull", Image: "registry.example.com/x:bad", State: portal.ContainerWaiting,
					Reason:  ptr.To("ImagePullBackOff"),
					Message: ptr.To(`Back-off pulling image "registry.example.com/x:bad"`),
				},
				{
					Name: "leak", Ready: true, State: portal.ContainerRunning,
					Image:           "registry.example.com/leak:1@sha256:0123456789abcdef",
					ImageDigest:     ptr.To("sha256:0123456789abcdef"),
					LastTermination: &portal.ContainerTermination{Reason: ptr.To("OOMKilled"), ExitCode: 137},
				},
				{
					Name: "retry", State: portal.ContainerTerminated, Reason: ptr.To("Completed"), ExitCode: ptr.To(0),
					LastTermination: &portal.ContainerTermination{ExitCode: 2, Message: ptr.To("config file\nnot found")},
				},
				{Name: "new", State: portal.ContainerWaiting},
				{
					Name: "loaded", Image: "registry.example.com/loaded:1", ImageID: ptr.To("docker://sha256:fedcba9876543210"),
					State: portal.ContainerWaiting,
				},
				{
					Name: "hostile", State: portal.ContainerTerminated, Reason: ptr.To("Error"), ExitCode: ptr.To(1),
					Message: ptr.To("bye\x1b[2J\x1b]0;title\x07"),
				},
			},
		},
	}

	want := []string{
		"  - evicted: Evicted: The node was low on resource: memory.",
		"  - evicted/app: terminated (Error), exit 137",
		"  - pending: PodScheduled False (Unschedulable): 0/3 nodes are available: 3 Insufficient memory.",
		"  - pending: Custom Unknown",
		"  - mixed/fetch (init): running",
		"  - mixed/proxy (sidecar): running, not ready",
		"  - mixed/probe: running, not ready",
		`  - mixed/pull: waiting (ImagePullBackOff): Back-off pulling image "registry.example.com/x:bad"` +
			"\n      image: registry.example.com/x:bad",
		"  - mixed/leak: running; last termination: OOMKilled, exit 137\n      image: registry.example.com/leak:1@sha256:01234567",
		"  - mixed/retry: terminated (Completed); last termination: exit 2: config file not found",
		"  - mixed/new: waiting",
		"  - mixed/loaded: waiting\n      image: registry.example.com/loaded:1 (image ID sha256:fedcba98)",
		"  - mixed/hostile: terminated (Error), exit 1: bye[2J]0;title",
	}

	got := detailLines(pods)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("detailLines mismatch\ngot:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRestartsCell(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		pod  portal.EnvPod
		want string
	}{
		"none":                   {pod: portal.EnvPod{}, want: "0"},
		"without a restart time": {pod: portal.EnvPod{Restarts: 3}, want: "3"},
		"with a restart time": {
			pod:  portal.EnvPod{Restarts: 9, LastRestartAt: ptr.To("2026-01-05T06:07:00Z")},
			want: "9 (Jan 05 06:07)",
		},
	}

	for name, tc := range cases {
		if got := restartsCell(tc.pod); got != tc.want {
			t.Errorf("%s: restartsCell = %q, want %q", name, got, tc.want)
		}
	}
}

func TestStatusCell(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		pod   portal.EnvPod
		color func(string) string
	}{
		"healthy": {
			pod:   portal.EnvPod{Status: "Running", Phase: portal.PodPhaseRunning, ReadyContainers: 2, TotalContainers: 2},
			color: output.GreenText,
		},
		"unhealthy": {
			pod:   portal.EnvPod{Status: "CrashLoopBackOff", Phase: portal.PodPhaseRunning, TotalContainers: 1},
			color: output.YellowText,
		},
		"failed": {
			pod:   portal.EnvPod{Status: "Error", Phase: portal.PodPhaseFailed, TotalContainers: 1},
			color: output.RedText,
		},
	}

	for name, tc := range cases {
		if got := statusCell(tc.pod, false); got != tc.pod.Status {
			t.Errorf("%s: without a TTY the status stays plain, got %q", name, got)
		}

		if got, want := statusCell(tc.pod, true), tc.color(tc.pod.Status); got != want {
			t.Errorf("%s: statusCell on a TTY = %q, want %q", name, got, want)
		}
	}
}
