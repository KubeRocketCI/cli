package portal

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/ptr"
)

// podObject builds a Pod in namespace my-pipeline-dev; metadata adds to or
// overrides its name and namespace.
func podObject(name string, metadata, spec, status map[string]any) map[string]any {
	meta := map[string]any{"name": name, "namespace": "my-pipeline-dev"}
	for k, v := range metadata {
		meta[k] = v
	}

	return map[string]any{"metadata": meta, "spec": spec, "status": status}
}

func TestEnvService_Pods(t *testing.T) {
	t.Parallel()

	rec := envWithStage(t, "in-cluster")
	rec.listByKind["Pod"] = objectList(
		podObject("foo-6c9f7d9b8-x2x9k",
			map[string]any{
				"creationTimestamp": "2026-10-08T07:40:11Z",
				"labels":            map[string]any{"app.kubernetes.io/instance": "foo"},
				"ownerReferences": []any{
					map[string]any{"kind": "ReplicaSet", "name": "foo-6c9f7d9b8", "controller": true},
				},
			},
			map[string]any{
				"nodeName": "node-1",
				"containers": []any{map[string]any{
					"name": "foo", "image": "registry.example.com/ns/foo:1.2.0",
					"env": []any{map[string]any{"name": "TOKEN", "value": "s3cr3t"}},
				}},
			},
			map[string]any{
				"phase": "Running",
				"conditions": []any{
					map[string]any{"type": "PodScheduled", "status": "True"},
					map[string]any{
						"type": "Ready", "status": "False", "reason": "ContainersNotReady",
						"message": "containers with unready status: [foo]", "lastTransitionTime": "2026-10-08T07:41:00Z",
					},
				},
				"containerStatuses": []any{
					map[string]any{
						"name": "foo", "image": "sha256:0f0f", "ready": false, "restartCount": 7,
						"imageID": "registry.example.com/ns/foo@sha256:abc12345",
						"state": map[string]any{"waiting": map[string]any{
							"reason": "CrashLoopBackOff", "message": "back-off 5m0s restarting failed container=foo",
						}},
						"lastState": map[string]any{"terminated": map[string]any{
							"reason": "Error", "exitCode": 1, "startedAt": "2026-10-08T08:09:58Z", "finishedAt": "2026-10-08T08:10:02Z",
						}},
					},
				},
			}),
		podObject("bar-5d8f7c6b9d-abcde",
			map[string]any{
				"creationTimestamp": "2026-10-06T10:00:00Z",
				"labels":            map[string]any{"app.kubernetes.io/instance": "bar"},
				"ownerReferences": []any{
					map[string]any{"kind": "Node", "name": "ignored"},
					map[string]any{"kind": "ReplicaSet", "name": "bar-5d8f7c6b9d", "controller": true},
				},
			},
			map[string]any{"nodeName": "node-2", "containers": []any{
				map[string]any{"name": "bar", "image": "registry.example.com/ns/bar:2.0.1"},
			}},
			map[string]any{
				"phase":      "Running",
				"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
				"containerStatuses": []any{
					map[string]any{
						"name": "bar", "image": "registry.example.com/ns/bar:2.0.1", "ready": true, "restartCount": 0,
						"state": map[string]any{"running": map[string]any{"startedAt": "2026-10-06T10:00:05Z"}},
					},
				},
			}),
		podObject("debug",
			map[string]any{"labels": map[string]any{"app.kubernetes.io/instance": "not-registered"}},
			map[string]any{"containers": []any{map[string]any{"name": "shell"}}},
			map[string]any{"phase": "Pending"}),
	)

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	got, err := svc.Pods(context.Background(), "my-pipeline", "dev")
	if err != nil {
		t.Fatalf("Pods error: %v", err)
	}

	want := &EnvPodsPayload{
		EnvNamespace: EnvNamespace{
			Deployment: "my-pipeline", Env: "dev", Cluster: "in-cluster", Namespace: "my-pipeline-dev",
		},
		Pods: []EnvPod{
			{
				Name: "bar-5d8f7c6b9d-abcde", Project: ptr.To("bar"), Status: "Running", Phase: "Running",
				ReadyContainers: 1, TotalContainers: 1, CreatedAt: ptr.To("2026-10-06T10:00:00Z"), Node: ptr.To("node-2"),
				Owner:      &PodOwner{Kind: "ReplicaSet", Name: "bar-5d8f7c6b9d"},
				Conditions: []PodCondition{},
				Containers: []PodContainer{{
					Name: "bar", Image: "registry.example.com/ns/bar:2.0.1", Ready: true,
					State: ContainerRunning, StartedAt: ptr.To("2026-10-06T10:00:05Z"),
				}},
			},
			{
				Name: "debug", Status: "Pending", Phase: "Pending", TotalContainers: 1,
				Conditions: []PodCondition{}, Containers: []PodContainer{},
			},
			{
				Name: "foo-6c9f7d9b8-x2x9k", Project: ptr.To("foo"), Status: "CrashLoopBackOff", Phase: "Running",
				TotalContainers: 1, Restarts: 7, LastRestartAt: ptr.To("2026-10-08T08:10:02Z"),
				CreatedAt: ptr.To("2026-10-08T07:40:11Z"), Node: ptr.To("node-1"),
				Owner: &PodOwner{Kind: "ReplicaSet", Name: "foo-6c9f7d9b8"},
				Conditions: []PodCondition{{
					Type: "Ready", Status: "False", Reason: ptr.To("ContainersNotReady"),
					Message:            ptr.To("containers with unready status: [foo]"),
					LastTransitionTime: ptr.To("2026-10-08T07:41:00Z"),
				}},
				Containers: []PodContainer{{
					Name: "foo", Image: "registry.example.com/ns/foo:1.2.0", Restarts: 7,
					ImageID:     ptr.To("registry.example.com/ns/foo@sha256:abc12345"),
					ImageDigest: ptr.To("sha256:abc12345"),
					State:       ContainerWaiting, Reason: ptr.To("CrashLoopBackOff"),
					Message: ptr.To("back-off 5m0s restarting failed container=foo"),
					LastTermination: &ContainerTermination{
						Reason: ptr.To("Error"), ExitCode: 1,
						StartedAt: ptr.To("2026-10-08T08:09:58Z"), FinishedAt: ptr.To("2026-10-08T08:10:02Z"),
					},
				}},
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("payload mismatch\ngot:  %s\nwant: %s", gotJSON, wantJSON)
	}

	raw := mustJSON(got)
	if strings.Contains(raw, "s3cr3t") || strings.Contains(raw, "TOKEN") {
		t.Errorf("the payload must not carry the pod spec: %s", raw)
	}

	calls := callsOfKind(rec, "Pod")
	if len(calls) != 1 {
		t.Fatalf("expected 1 Pod list call, got %d", len(calls))
	}

	if calls[0].Namespace != "my-pipeline-dev" {
		t.Errorf("pods listed in namespace %q, want the Stage namespace my-pipeline-dev", calls[0].Namespace)
	}

	if len(calls[0].LabelSelectors) != 0 {
		t.Errorf("pods must be listed without a label selector, got %v", calls[0].LabelSelectors)
	}
}

func TestEnvService_Pods_Empty(t *testing.T) {
	t.Parallel()

	rec := envWithStage(t, "")

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	got, err := svc.Pods(context.Background(), "my-pipeline", "dev")
	if err != nil {
		t.Fatalf("Pods error: %v", err)
	}

	if raw := mustJSON(got); !strings.Contains(raw, `"pods":[]`) {
		t.Errorf("an empty namespace must marshal as pods:[], got %s", raw)
	}
}

func TestEnvService_Pods_UndecodableItem(t *testing.T) {
	t.Parallel()

	rec := envWithStage(t, "in-cluster")
	rec.listByKind["Pod"] = objectList(podObject("foo", nil, nil, map[string]any{
		"containerStatuses": []any{map[string]any{"name": "foo", "restartCount": "seven"}},
	}))

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	_, err := svc.Pods(context.Background(), "my-pipeline", "dev")

	want := namespaceReadError(podResourceConfig, "my-pipeline-dev", errors.New("decoding the response: ")).Error()
	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("error = %v, want it to start with %q", err, want)
	}
}

func terminated(reason string, exitCode, signal int) k8sContainerState {
	return k8sContainerState{Terminated: &k8sContainerTerminated{Reason: reason, ExitCode: exitCode, Signal: signal}}
}

func waiting(reason string) k8sContainerState {
	return k8sContainerState{Waiting: &k8sContainerWaiting{Reason: reason}}
}

func running() k8sContainerState {
	return k8sContainerState{Running: &k8sContainerRunning{StartedAt: "2026-10-08T08:00:00Z"}}
}

func containers(names ...string) []k8sContainerSpec {
	out := make([]k8sContainerSpec, 0, len(names))
	for _, n := range names {
		out = append(out, k8sContainerSpec{Name: n})
	}

	return out
}

// TestPodSummary pins STATUS, READY and RESTARTS to the values
// `kubectl get pods` prints for the same pod.
func TestPodSummary(t *testing.T) {
	t.Parallel()

	deleted := ptr.To("2026-10-08T08:00:00Z")
	initialized := []k8sPodCondition{{Type: "Initialized", Status: "True"}}
	ready := []k8sPodCondition{{Type: "Ready", Status: "True"}}

	cases := map[string]struct {
		pod      k8sPod
		status   string
		ready    int
		total    int
		restarts int
	}{
		"running and ready": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app", "proxy")},
				Status: k8sPodStatus{Phase: "Running", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", Ready: true, State: running()},
					{Name: "proxy", Ready: true, RestartCount: 2, State: running()},
				}},
			},
			status: "Running", ready: 2, total: 2, restarts: 2,
		},
		"not scheduled": {
			pod:    k8sPod{Spec: k8sPodSpec{Containers: containers("app")}, Status: k8sPodStatus{Phase: "Pending"}},
			status: "Pending", total: 1,
		},
		"a completed container gives way to one that failed": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app", "worker")},
				Status: k8sPodStatus{Phase: "Failed", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", State: terminated("Completed", 0, 0)},
					{Name: "worker", State: terminated("Error", 1, 0)},
				}},
			},
			status: "Error", total: 2,
		},
		"pod reason wins over the phase": {
			pod: k8sPod{
				Spec:   k8sPodSpec{Containers: containers("app")},
				Status: k8sPodStatus{Phase: "Failed", Reason: "Evicted"},
			},
			status: "Evicted", total: 1,
		},
		"scheduling gated": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app")},
				Status: k8sPodStatus{Phase: "Pending", Conditions: []k8sPodCondition{
					{Type: "PodScheduled", Status: "False", Reason: "SchedulingGated"},
				}},
			},
			status: "SchedulingGated", total: 1,
		},
		"init container failed with a reason": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app"), InitContainers: containers("migrate")},
				Status: k8sPodStatus{Phase: "Pending", InitContainerStatuses: []k8sContainerStatus{
					{Name: "migrate", RestartCount: 3, State: terminated("Error", 1, 0)},
				}},
			},
			status: "Init:Error", total: 1, restarts: 3,
		},
		"init container killed by a signal": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app"), InitContainers: containers("migrate")},
				Status: k8sPodStatus{Phase: "Pending", InitContainerStatuses: []k8sContainerStatus{
					{Name: "migrate", State: terminated("", 137, 9)},
				}},
			},
			status: "Init:Signal:9", total: 1,
		},
		"init container exited without a reason": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app"), InitContainers: containers("migrate")},
				Status: k8sPodStatus{Phase: "Pending", InitContainerStatuses: []k8sContainerStatus{
					{Name: "migrate", State: terminated("", 2, 0)},
				}},
			},
			status: "Init:ExitCode:2", total: 1,
		},
		"init container waiting with a reason": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app"), InitContainers: containers("migrate")},
				Status: k8sPodStatus{Phase: "Pending", InitContainerStatuses: []k8sContainerStatus{
					{Name: "migrate", State: waiting("ImagePullBackOff")},
				}},
			},
			status: "Init:ImagePullBackOff", total: 1,
		},
		"second init container in progress": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app"), InitContainers: containers("fetch", "migrate")},
				Status: k8sPodStatus{Phase: "Pending", InitContainerStatuses: []k8sContainerStatus{
					{Name: "fetch", RestartCount: 1, State: terminated("Completed", 0, 0)},
					{Name: "migrate", State: running()},
				}},
			},
			status: "Init:1/2", total: 1, restarts: 1,
		},
		"init container waiting to start": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app"), InitContainers: containers("fetch", "migrate")},
				Status: k8sPodStatus{Phase: "Pending", InitContainerStatuses: []k8sContainerStatus{
					{Name: "fetch", State: waiting("PodInitializing")},
					{Name: "migrate", State: waiting("PodInitializing")},
				}},
			},
			status: "Init:0/2", total: 1,
		},
		"sidecar counts as a container and its restarts stay": {
			pod: k8sPod{
				Spec: k8sPodSpec{
					Containers: containers("app"),
					InitContainers: []k8sContainerSpec{
						{Name: "fetch"},
						{Name: "mesh", RestartPolicy: "Always"},
					},
				},
				Status: k8sPodStatus{
					Phase: "Running", Conditions: initialized,
					InitContainerStatuses: []k8sContainerStatus{
						{Name: "fetch", RestartCount: 4, State: terminated("Completed", 0, 0)},
						{Name: "mesh", Ready: true, Started: ptr.To(true), RestartCount: 1, State: running()},
					},
					ContainerStatuses: []k8sContainerStatus{{Name: "app", Ready: true, RestartCount: 2, State: running()}},
				},
			},
			status: "Running", ready: 2, total: 2, restarts: 3,
		},
		"sidecar failing after the pod initialized": {
			pod: k8sPod{
				Spec: k8sPodSpec{
					Containers:     containers("app"),
					InitContainers: []k8sContainerSpec{{Name: "mesh", RestartPolicy: "Always"}},
				},
				Status: k8sPodStatus{
					Phase: "Running", Conditions: initialized,
					InitContainerStatuses: []k8sContainerStatus{
						{Name: "mesh", Started: ptr.To(false), RestartCount: 5, State: waiting("CrashLoopBackOff")},
					},
					ContainerStatuses: []k8sContainerStatus{{Name: "app", Ready: true, State: running()}},
				},
			},
			status: "Init:CrashLoopBackOff", ready: 1, total: 2, restarts: 5,
		},
		"container in a crash loop": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app", "proxy")},
				Status: k8sPodStatus{Phase: "Running", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", RestartCount: 7, State: waiting("CrashLoopBackOff")},
					{Name: "proxy", Ready: true, State: running()},
				}},
			},
			status: "CrashLoopBackOff", ready: 1, total: 2, restarts: 7,
		},
		"first container decides when several are not running": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app", "proxy")},
				Status: k8sPodStatus{Phase: "Pending", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", State: waiting("ErrImagePull")},
					{Name: "proxy", State: waiting("CreateContainerConfigError")},
				}},
			},
			status: "ErrImagePull", total: 2,
		},
		"container terminated with a reason": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app")},
				Status: k8sPodStatus{Phase: "Failed", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", State: terminated("OOMKilled", 137, 0)},
				}},
			},
			status: "OOMKilled", total: 1,
		},
		"container exited without a reason": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app")},
				Status: k8sPodStatus{Phase: "Failed", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", State: terminated("", 3, 0)},
				}},
			},
			status: "ExitCode:3", total: 1,
		},
		"container killed by a signal": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app")},
				Status: k8sPodStatus{Phase: "Failed", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", State: terminated("", 143, 15)},
				}},
			},
			status: "Signal:15", total: 1,
		},
		"completed": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("job")},
				Status: k8sPodStatus{Phase: "Succeeded", ContainerStatuses: []k8sContainerStatus{
					{Name: "job", State: terminated("Completed", 0, 0)},
				}},
			},
			status: "Completed", total: 1,
		},
		"one container completed, another runs and the pod is ready": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app", "seed")},
				Status: k8sPodStatus{Phase: "Running", Conditions: ready, ContainerStatuses: []k8sContainerStatus{
					{Name: "app", Ready: true, State: running()},
					{Name: "seed", State: terminated("Completed", 0, 0)},
				}},
			},
			status: "Running", ready: 1, total: 2,
		},
		"one container completed, another runs and the pod is not ready": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app", "seed")},
				Status: k8sPodStatus{Phase: "Running", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", Ready: true, State: running()},
					{Name: "seed", State: terminated("Completed", 0, 0)},
				}},
			},
			status: "NotReady", ready: 1, total: 2,
		},
		"running but not ready": {
			pod: k8sPod{
				Spec: k8sPodSpec{Containers: containers("app")},
				Status: k8sPodStatus{Phase: "Running", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", State: running()},
				}},
			},
			status: "Running", total: 1,
		},
		"terminating": {
			pod: k8sPod{
				Metadata: k8sPodMetadata{DeletionTimestamp: deleted},
				Spec:     k8sPodSpec{Containers: containers("app")},
				Status: k8sPodStatus{Phase: "Running", ContainerStatuses: []k8sContainerStatus{
					{Name: "app", Ready: true, State: running()},
				}},
			},
			status: "Terminating", ready: 1, total: 1,
		},
		"deleted on a lost node": {
			pod: k8sPod{
				Metadata: k8sPodMetadata{DeletionTimestamp: deleted},
				Spec:     k8sPodSpec{Containers: containers("app")},
				Status:   k8sPodStatus{Phase: "Running", Reason: "NodeLost"},
			},
			status: "Unknown", total: 1,
		},
		"deleted after it finished": {
			pod: k8sPod{
				Metadata: k8sPodMetadata{DeletionTimestamp: deleted},
				Spec:     k8sPodSpec{Containers: containers("job")},
				Status: k8sPodStatus{Phase: "Succeeded", ContainerStatuses: []k8sContainerStatus{
					{Name: "job", State: terminated("Completed", 0, 0)},
				}},
			},
			status: "Completed", total: 1,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := summarizePod(tc.pod)
			if got.status != tc.status || got.ready != tc.ready || got.total != tc.total || got.restarts.count != tc.restarts {
				t.Errorf("summarizePod = %s %d/%d restarts %d, want %s %d/%d restarts %d",
					got.status, got.ready, got.total, got.restarts.count, tc.status, tc.ready, tc.total, tc.restarts)
			}
		})
	}
}

// TestSummarizePod_LastRestart pins the restart time to the restarts the count
// keeps, as printPod dates RESTARTS.
func TestSummarizePod_LastRestart(t *testing.T) {
	t.Parallel()

	restarted := func(name string, count int, finishedAt string) k8sContainerStatus {
		return k8sContainerStatus{
			Name: name, RestartCount: count, State: running(), Ready: true, Started: ptr.To(true),
			LastState: k8sContainerState{Terminated: &k8sContainerTerminated{Reason: "Error", ExitCode: 1, FinishedAt: finishedAt}},
		}
	}
	sidecarSpec := []k8sContainerSpec{{Name: "proxy", RestartPolicy: containerRestartAlways}, {Name: "migrate"}}
	initialized := []k8sPodCondition{{Type: "Initialized", Status: "True"}}

	cases := map[string]struct {
		pod   k8sPod
		count int
		want  *string
	}{
		"no restart": {
			pod: k8sPod{Spec: k8sPodSpec{Containers: containers("app")}, Status: k8sPodStatus{
				ContainerStatuses: []k8sContainerStatus{{Name: "app", Ready: true, State: running()}},
			}},
		},
		"the newest container restart": {
			pod: k8sPod{Spec: k8sPodSpec{Containers: containers("app", "proxy")}, Status: k8sPodStatus{
				ContainerStatuses: []k8sContainerStatus{
					restarted("app", 2, "2026-10-08T08:00:00Z"),
					restarted("proxy", 1, "2026-10-08T09:00:00Z"),
				},
			}},
			count: 3, want: ptr.To("2026-10-08T09:00:00Z"),
		},
		"an initialized pod drops a regular init restart": {
			pod: k8sPod{Spec: k8sPodSpec{InitContainers: sidecarSpec, Containers: containers("app")}, Status: k8sPodStatus{
				Conditions: initialized,
				InitContainerStatuses: []k8sContainerStatus{
					restarted("proxy", 1, "2026-10-08T08:00:00Z"),
					{
						Name: "migrate", RestartCount: 1, State: terminated("Completed", 0, 0),
						LastState: k8sContainerState{Terminated: &k8sContainerTerminated{ExitCode: 1, FinishedAt: "2026-10-08T08:05:00Z"}},
					},
				},
				ContainerStatuses: []k8sContainerStatus{{Name: "app", Ready: true, State: running()}},
			}},
			count: 1, want: ptr.To("2026-10-08T08:00:00Z"),
		},
		"an initializing pod keeps the init restart": {
			pod: k8sPod{Spec: k8sPodSpec{InitContainers: sidecarSpec, Containers: containers("app")}, Status: k8sPodStatus{
				InitContainerStatuses: []k8sContainerStatus{
					restarted("proxy", 1, "2026-10-08T08:00:00Z"),
					{
						Name: "migrate", RestartCount: 2, State: waiting("CrashLoopBackOff"),
						LastState: k8sContainerState{Terminated: &k8sContainerTerminated{ExitCode: 1, FinishedAt: "2026-10-08T08:05:00Z"}},
					},
				},
			}},
			count: 3, want: ptr.To("2026-10-08T08:05:00Z"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := summarizePod(tc.pod).restarts
			if got.count != tc.count || !reflect.DeepEqual(got.lastAt(), tc.want) {
				t.Errorf("restarts = %d at %v, want %d at %v",
					got.count, ptr.Deref(got.lastAt(), "nil"), tc.count, ptr.Deref(tc.want, "nil"))
			}
		})
	}
}

func TestMapPodContainers(t *testing.T) {
	t.Parallel()

	spec := k8sPodSpec{
		InitContainers: []k8sContainerSpec{
			{Name: "migrate", Image: "registry.example.com/migrate:1"},
			{Name: "proxy", Image: "registry.example.com/proxy:1", RestartPolicy: containerRestartAlways},
		},
		Containers: []k8sContainerSpec{{Name: "app", Image: "registry.example.com/app:1@sha256:aaa111"}},
	}
	status := k8sPodStatus{
		InitContainerStatuses: []k8sContainerStatus{
			{Name: "migrate", ImageID: "sha256:0f0f", State: k8sContainerState{Terminated: &k8sContainerTerminated{
				Reason: "Completed", StartedAt: "2026-10-08T08:00:00Z", FinishedAt: "2026-10-08T08:00:04Z",
			}}},
			{Name: "proxy", State: running()},
		},
		ContainerStatuses: []k8sContainerStatus{
			{Name: "app", ImageID: "registry.example.com/app@sha256:aaa111", State: k8sContainerState{Terminated: &k8sContainerTerminated{
				Reason: "OOMKilled", Message: "out of memory", ExitCode: 137,
				StartedAt: "2026-10-08T08:01:00Z", FinishedAt: "2026-10-08T08:02:00Z",
			}}},
			{Name: "not-reported"},
		},
	}

	want := []PodContainer{
		{
			Name: "migrate", Init: true, Image: "registry.example.com/migrate:1", ImageID: ptr.To("sha256:0f0f"),
			State:  ContainerTerminated,
			Reason: ptr.To("Completed"), ExitCode: ptr.To(0),
			StartedAt: ptr.To("2026-10-08T08:00:00Z"), FinishedAt: ptr.To("2026-10-08T08:00:04Z"),
		},
		{
			Name: "proxy", Init: true, Sidecar: true, Image: "registry.example.com/proxy:1", State: ContainerRunning,
			StartedAt: ptr.To("2026-10-08T08:00:00Z"),
		},
		{
			Name: "app", Image: "registry.example.com/app:1@sha256:aaa111",
			ImageID:     ptr.To("registry.example.com/app@sha256:aaa111"),
			ImageDigest: ptr.To("sha256:aaa111"),
			State:       ContainerTerminated,
			Reason:      ptr.To("OOMKilled"), Message: ptr.To("out of memory"), ExitCode: ptr.To(137),
			StartedAt: ptr.To("2026-10-08T08:01:00Z"), FinishedAt: ptr.To("2026-10-08T08:02:00Z"),
		},
		{Name: "not-reported", State: ContainerWaiting},
	}

	if got := mapPodContainers(spec, status); !reflect.DeepEqual(got, want) {
		t.Errorf("mapPodContainers mismatch\ngot:  %s\nwant: %s", mustJSON(got), mustJSON(want))
	}
}

// TestPulledDigest pins which container status imageID forms carry a registry
// digest. A bare or runtime-prefixed "sha256:<hex>" is the image's local ID.
func TestPulledDigest(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		imageID string
		want    *string
	}{
		"repository digest":         {imageID: "docker.io/library/busybox@sha256:73aaf090", want: ptr.To("sha256:73aaf090")},
		"docker-pullable digest":    {imageID: "docker-pullable://busybox@sha256:73aaf090", want: ptr.To("sha256:73aaf090")},
		"bare image ID":             {imageID: "sha256:fe81a497"},
		"runtime-prefixed image ID": {imageID: "docker://sha256:fe81a497"},
		"not present yet":           {imageID: ""},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := pulledDigest(tc.imageID); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("pulledDigest(%q) = %v, want %v", tc.imageID, ptr.Deref(got, "nil"), ptr.Deref(tc.want, "nil"))
			}
		})
	}
}

func TestEnvPod_Healthy(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		pod  EnvPod
		want bool
	}{
		"completed":            {pod: EnvPod{Status: "Completed", Phase: PodPhaseSucceeded}, want: true},
		"running and ready":    {pod: EnvPod{Status: "Running", Phase: PodPhaseRunning, ReadyContainers: 2, TotalContainers: 2}, want: true},
		"running, one unready": {pod: EnvPod{Status: "Running", Phase: PodPhaseRunning, ReadyContainers: 1, TotalContainers: 2}},
		"terminating":          {pod: EnvPod{Status: "Terminating", Phase: PodPhaseRunning, ReadyContainers: 1, TotalContainers: 1}},
		"crash loop":           {pod: EnvPod{Status: "CrashLoopBackOff", Phase: PodPhaseRunning, TotalContainers: 1}},
		"failed":               {pod: EnvPod{Status: "Error", Phase: PodPhaseFailed, TotalContainers: 1}},
		"pending":              {pod: EnvPod{Status: "Pending", Phase: "Pending", TotalContainers: 1}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tc.pod.Healthy(); got != tc.want {
				t.Errorf("Healthy = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPodContainer_NeedsAttention(t *testing.T) {
	t.Parallel()

	restarted := &ContainerTermination{ExitCode: 137}

	cases := map[string]struct {
		c    PodContainer
		want bool
	}{
		"running ready":                {c: PodContainer{State: ContainerRunning, Ready: true}},
		"running ready, restarted":     {c: PodContainer{State: ContainerRunning, Ready: true, LastTermination: restarted}, want: true},
		"running, not ready":           {c: PodContainer{State: ContainerRunning}, want: true},
		"exited with 0":                {c: PodContainer{State: ContainerTerminated, ExitCode: ptr.To(0)}},
		"exited with 1":                {c: PodContainer{State: ContainerTerminated, ExitCode: ptr.To(1)}, want: true},
		"waiting":                      {c: PodContainer{State: ContainerWaiting}, want: true},
		"init container exited with 0": {c: PodContainer{Init: true, State: ContainerTerminated, ExitCode: ptr.To(0)}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tc.c.NeedsAttention(); got != tc.want {
				t.Errorf("NeedsAttention = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPodContainer_FailsReadiness(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		c    PodContainer
		want bool
	}{
		"running, not ready":                {c: PodContainer{State: ContainerRunning}, want: true},
		"running ready":                     {c: PodContainer{State: ContainerRunning, Ready: true}},
		"init container running, not ready": {c: PodContainer{Init: true, State: ContainerRunning}},
		"sidecar running, not ready":        {c: PodContainer{Init: true, Sidecar: true, State: ContainerRunning}, want: true},
		"sidecar running ready":             {c: PodContainer{Init: true, Sidecar: true, State: ContainerRunning, Ready: true}},
		"waiting":                           {c: PodContainer{State: ContainerWaiting}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tc.c.FailsReadiness(); got != tc.want {
				t.Errorf("FailsReadiness = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMapPodConditions(t *testing.T) {
	t.Parallel()

	got := mapPodConditions([]k8sPodCondition{
		{Type: "PodScheduled", Status: "False", Reason: "Unschedulable", Message: "0/3 nodes are available"},
		{Type: "Initialized", Status: "True"},
		{Type: "Ready", Status: "Unknown"},
		{Type: "DisruptionTarget", Status: "True", Reason: "PreemptionByScheduler"},
		{Type: "DisruptionTarget", Status: "False"},
	})

	want := []PodCondition{
		{Type: "PodScheduled", Status: "False", Reason: ptr.To("Unschedulable"), Message: ptr.To("0/3 nodes are available")},
		{Type: "Ready", Status: "Unknown"},
		{Type: "DisruptionTarget", Status: "True", Reason: ptr.To("PreemptionByScheduler")},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("mapPodConditions mismatch\ngot:  %s\nwant: %s", mustJSON(got), mustJSON(want))
	}
}

func TestPodOwner(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		refs []k8sOwnerReference
		want *PodOwner
	}{
		"no owner":            {refs: nil, want: nil},
		"controller wins":     {refs: []k8sOwnerReference{{Kind: "Node", Name: "n"}, {Kind: "Job", Name: "j", Controller: true}}, want: &PodOwner{Kind: "Job", Name: "j"}},
		"first owner without": {refs: []k8sOwnerReference{{Kind: "Node", Name: "n"}, {Kind: "Other", Name: "o"}}, want: &PodOwner{Kind: "Node", Name: "n"}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := podOwner(tc.refs); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("podOwner = %+v, want %+v", got, tc.want)
			}
		})
	}
}
