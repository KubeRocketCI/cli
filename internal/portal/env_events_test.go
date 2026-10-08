package portal

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/ptr"
)

// eventObject builds an Event about kind/name with extra top-level fields.
func eventObject(eventType, reason, kind, name string, fields map[string]any) map[string]any {
	out := map[string]any{
		"metadata":       map[string]any{"name": name + "." + reason, "namespace": "my-pipeline-dev"},
		"type":           eventType,
		"reason":         reason,
		"involvedObject": map[string]any{"kind": kind, "name": name, "namespace": "my-pipeline-dev"},
	}
	for k, v := range fields {
		out[k] = v
	}

	return out
}

// envEvents serves the events every Events test filters and orders.
func envEvents(t *testing.T) *envTestRecorder {
	t.Helper()

	rec := envWithStage(t, "in-cluster")
	rec.listByKind["Event"] = objectList(
		eventObject("Normal", "Undated", "Pod", "quiet", nil),
		eventObject("Normal", "Scheduled", "Pod", "foo-6c9f7d9b8-x2x9k", map[string]any{
			"message":             "Successfully assigned my-pipeline-dev/foo-6c9f7d9b8-x2x9k to node-1",
			"eventTime":           "2026-10-08T07:40:11.123456Z",
			"reportingComponent":  "default-scheduler",
			"firstTimestamp":      nil,
			"lastTimestamp":       nil,
			"deprecatedLastField": "ignored",
		}),
		eventObject("Warning", "BackOff", "Pod", "foo-6c9f7d9b8-x2x9k", map[string]any{
			"message":        "  Back-off restarting failed container foo in pod foo-6c9f7d9b8-x2x9k\n",
			"count":          42,
			"firstTimestamp": "2026-10-08T07:41:00Z",
			"lastTimestamp":  "2026-10-08T08:10:02Z",
			"source":         map[string]any{"component": "kubelet", "host": "node-1"},
		}),
		eventObject("Warning", "FailedCreate", "ReplicaSet", "bar-5d8f7c6b9d", map[string]any{
			"message":        "pods \"bar-5d8f7c6b9d-\" is forbidden: exceeded quota",
			"count":          3,
			"firstTimestamp": "2026-10-08T07:50:00Z",
			"lastTimestamp":  "2026-10-08T07:55:00Z",
			"series":         map[string]any{"count": 9, "lastObservedTime": "2026-10-08T08:20:00.000000Z"},
			"source":         map[string]any{"component": "replicaset-controller"},
		}),
		eventObject("Normal", "Pulled", "Pod", "bar-5d8f7c6b9d", map[string]any{
			"message":        "a pod that shares its name with the ReplicaSet above",
			"firstTimestamp": "2026-10-08T07:30:00Z",
		}),
	)

	return rec
}

func TestEnvService_Events(t *testing.T) {
	t.Parallel()

	rec := envEvents(t)

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	got, err := svc.Events(context.Background(), "my-pipeline", "dev", EnvEventFilters{})
	if err != nil {
		t.Fatalf("Events error: %v", err)
	}

	want := &EnvEventsPayload{
		EnvNamespace: EnvNamespace{
			Deployment: "my-pipeline", Env: "dev", Cluster: "in-cluster", Namespace: "my-pipeline-dev",
		},
		Events: []EnvEvent{
			{
				Type: "Warning", Reason: "FailedCreate", Count: 9,
				Message:        `pods "bar-5d8f7c6b9d-" is forbidden: exceeded quota`,
				InvolvedObject: EventObject{Kind: "ReplicaSet", Name: "bar-5d8f7c6b9d"},
				FirstSeen:      ptr.To("2026-10-08T07:50:00Z"), LastSeen: ptr.To("2026-10-08T08:20:00.000000Z"),
				Source: ptr.To("replicaset-controller"),
			},
			{
				Type: "Warning", Reason: "BackOff", Count: 42,
				Message:        "Back-off restarting failed container foo in pod foo-6c9f7d9b8-x2x9k",
				InvolvedObject: EventObject{Kind: "Pod", Name: "foo-6c9f7d9b8-x2x9k"},
				FirstSeen:      ptr.To("2026-10-08T07:41:00Z"), LastSeen: ptr.To("2026-10-08T08:10:02Z"),
				Source: ptr.To("kubelet"),
			},
			{
				Type: "Normal", Reason: "Scheduled", Count: 1,
				Message:        "Successfully assigned my-pipeline-dev/foo-6c9f7d9b8-x2x9k to node-1",
				InvolvedObject: EventObject{Kind: "Pod", Name: "foo-6c9f7d9b8-x2x9k"},
				FirstSeen:      ptr.To("2026-10-08T07:40:11.123456Z"), LastSeen: ptr.To("2026-10-08T07:40:11.123456Z"),
				Source: ptr.To("default-scheduler"),
			},
			{
				Type: "Normal", Reason: "Pulled", Count: 1,
				Message:        "a pod that shares its name with the ReplicaSet above",
				InvolvedObject: EventObject{Kind: "Pod", Name: "bar-5d8f7c6b9d"},
				FirstSeen:      ptr.To("2026-10-08T07:30:00Z"), LastSeen: ptr.To("2026-10-08T07:30:00Z"),
			},
			{Type: "Normal", Reason: "Undated", Count: 1, InvolvedObject: EventObject{Kind: "Pod", Name: "quiet"}},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payload mismatch\ngot:  %s\nwant: %s", mustJSON(got), mustJSON(want))
	}

	calls := callsOfKind(rec, "Event")
	if len(calls) != 1 {
		t.Fatalf("expected 1 Event list call, got %d", len(calls))
	}

	if calls[0].Namespace != "my-pipeline-dev" {
		t.Errorf("events listed in namespace %q, want the Stage namespace my-pipeline-dev", calls[0].Namespace)
	}
}

func TestEnvService_Events_Filters(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		filters EnvEventFilters
		want    []string
	}{
		"one pod":             {filters: EnvEventFilters{Pod: "foo-6c9f7d9b8-x2x9k"}, want: []string{"BackOff", "Scheduled"}},
		"a pod, not its kin":  {filters: EnvEventFilters{Pod: "bar-5d8f7c6b9d"}, want: []string{"Pulled"}},
		"warnings":            {filters: EnvEventFilters{WarningsOnly: true}, want: []string{"FailedCreate", "BackOff"}},
		"warnings of one pod": {filters: EnvEventFilters{Pod: "foo-6c9f7d9b8-x2x9k", WarningsOnly: true}, want: []string{"BackOff"}},
		"no match":            {filters: EnvEventFilters{Pod: "gone"}, want: []string{}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc, closer := newEnvServiceForTest(t, envEvents(t))
			defer closer()

			got, err := svc.Events(context.Background(), "my-pipeline", "dev", tc.filters)
			if err != nil {
				t.Fatalf("Events error: %v", err)
			}

			reasons := make([]string, 0, len(got.Events))
			for _, e := range got.Events {
				reasons = append(reasons, e.Reason)
			}

			if !slices.Equal(reasons, tc.want) {
				t.Errorf("reasons = %v, want %v", reasons, tc.want)
			}

			if raw := mustJSON(got); len(tc.want) == 0 && !strings.Contains(raw, `"events":[]`) {
				t.Errorf("no match must marshal as events:[], got %s", raw)
			}
		})
	}
}
