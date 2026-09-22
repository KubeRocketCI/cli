package portal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

func cdPipelineGetJSONWithMessage(name string, applications []string, message string) string {
	envelope := map[string]any{
		"apiVersion": "v1",
		"kind":       "CDPipeline",
		"metadata":   map[string]any{"name": name, "namespace": "ns"},
		"spec":       map[string]any{"applications": stringsToAny(applications)},
		"status":     statusWithMessage("created", message),
	}

	return mustJSON(envelope)
}

// newDeploymentServiceForTest wires up a DeploymentService against an httptest server.
func newDeploymentServiceForTest(t *testing.T, rec *envTestRecorder) (*DeploymentService, func()) {
	t.Helper()

	url, closer := newEnvTestServer(t, rec)

	client, err := restapi.NewClientWithResponses(url + "/rest")
	if err != nil {
		closer()
		t.Fatalf("new client: %v", err)
	}

	return NewDeploymentService(client, "in-cluster", "ns"), closer
}

func TestEnvService_Get_ArgoDiagnostics(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Stage": stageListJSON([]stageStub{
				{name: "my-pipeline-dev", deployment: "my-pipeline", env: "dev", cluster: "remote", namespace: "my-pipeline-dev", trigger: "Auto", order: 0, status: "failed", message: "namespace creation failed"},
			}),
			"Application": applicationListJSON([]applicationStub{
				{
					appName: "foo", pipeline: "my-pipeline", stage: "dev", health: "Unknown", sync: "Unknown",
					conditions:   []conditionStub{{condType: "ComparisonError", message: "Failed to load live state", since: "2026-09-21T10:00:00Z"}},
					opPhase:      "Error",
					opMessage:    "cluster unreachable",
					opFinishedAt: "2026-09-21T10:01:00Z",
				},
				{appName: "bar", pipeline: "my-pipeline", stage: "dev", health: "Healthy", sync: "Synced", imageRepo: "registry/bar", imageTag: "2.0.1"},
			}),
		},
		getByName: map[string]string{
			"my-pipeline": cdPipelineGetJSON("my-pipeline", []string{"foo", "bar", "baz"}),
		},
	}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	d, err := svc.Get(context.Background(), "my-pipeline", "dev")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if d.DetailedMessage == nil || *d.DetailedMessage != "namespace creation failed" {
		t.Errorf("detailedMessage = %v, want the Stage status.detailed_message", d.DetailedMessage)
	}

	byName := map[string]EnvProject{}
	for _, p := range d.Projects {
		byName[p.Name] = p
	}

	foo := byName["foo"]
	if len(foo.Conditions) != 1 {
		t.Fatalf("foo.conditions = %+v, want 1 entry", foo.Conditions)
	}

	if foo.Conditions[0].Type != "ComparisonError" || foo.Conditions[0].Message != "Failed to load live state" {
		t.Errorf("foo.conditions[0] = %+v", foo.Conditions[0])
	}

	if foo.Conditions[0].LastTransitionTime == nil || *foo.Conditions[0].LastTransitionTime != "2026-09-21T10:00:00Z" {
		t.Errorf("foo.conditions[0].lastTransitionTime = %v", foo.Conditions[0].LastTransitionTime)
	}

	if foo.Operation == nil || foo.Operation.Phase != "Error" {
		t.Fatalf("foo.operation = %+v, want phase Error", foo.Operation)
	}

	if foo.Operation.Message == nil || *foo.Operation.Message != "cluster unreachable" {
		t.Errorf("foo.operation.message = %v", foo.Operation.Message)
	}

	if foo.DeployedAt == nil || *foo.DeployedAt != "2026-09-21T10:01:00Z" {
		t.Errorf("foo.deployedAt = %v, want the operation finishedAt", foo.DeployedAt)
	}

	for _, name := range []string{"bar", "baz"} {
		p := byName[name]
		if p.Conditions == nil || len(p.Conditions) != 0 {
			t.Errorf("%s.conditions = %+v, want an empty array", name, p.Conditions)
		}

		if p.Operation != nil {
			t.Errorf("%s.operation = %+v, want null", name, p.Operation)
		}
	}

	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, want := range []string{`"conditions":[]`, `"operation":null`, `"detailedMessage":"namespace creation failed"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("JSON missing %s: %s", want, raw)
		}
	}
}

func TestEnvService_Get_NoDetailedMessage(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Stage": stageListJSON([]stageStub{
				{name: "billing-dev", deployment: "billing", env: "dev", cluster: "in-cluster", namespace: "billing-dev", trigger: "Auto", order: 0, status: "created"},
			}),
			"Application": applicationListJSON(nil),
		},
		getByName: map[string]string{
			"billing": cdPipelineGetJSON("billing", []string{"foo"}),
		},
	}

	svc, closer := newEnvServiceForTest(t, rec)
	defer closer()

	d, err := svc.Get(context.Background(), "billing", "dev")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if d.DetailedMessage != nil {
		t.Errorf("detailedMessage = %q, want nil without status.detailed_message", *d.DetailedMessage)
	}

	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(raw), `"detailedMessage":null`) {
		t.Errorf("JSON should carry detailedMessage:null, got %s", raw)
	}
}

func TestDeploymentByProjectService_List_ArgoDiagnostics(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Application": applicationListJSON([]applicationStub{
				{
					appName: "foo", pipeline: "my-pipeline", stage: "dev", health: "Unknown", sync: "Unknown",
					conditions: []conditionStub{{condType: "ComparisonError", message: "unable to resolve 'build/NaN' to a commit SHA"}},
					opPhase:    "Error",
					opMessage:  "manifest generation failed",
				},
			}),
			"Stage": stageListJSON([]stageStub{
				{name: "my-pipeline-dev", deployment: "my-pipeline", env: "dev", cluster: "in-cluster", namespace: "my-pipeline-dev", trigger: "Auto", order: 0, status: "created"},
				{name: "my-pipeline-qa", deployment: "my-pipeline", env: "qa", cluster: "in-cluster", namespace: "my-pipeline-qa", trigger: "Manual", order: 1, status: "created"},
			}),
			"CDPipeline": cdPipelineListJSON([]cdPipelineStub{
				{name: "my-pipeline", applications: []string{"foo"}},
			}),
		},
	}

	svc, closer := newDeploymentByProjectServiceForTest(t, rec)
	defer closer()

	rows, err := svc.List(context.Background(), "foo")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}

	dev := rows[0]
	if len(dev.Conditions) != 1 || dev.Conditions[0].Type != "ComparisonError" {
		t.Errorf("dev.conditions = %+v, want the ComparisonError", dev.Conditions)
	}

	if dev.Operation == nil || dev.Operation.Phase != "Error" || dev.Operation.Message == nil {
		t.Errorf("dev.operation = %+v, want phase Error with a message", dev.Operation)
	}

	qa := rows[1]
	if qa.Conditions == nil || len(qa.Conditions) != 0 {
		t.Errorf("qa.conditions = %+v, want an empty array on a not-deployed row", qa.Conditions)
	}

	if qa.Operation != nil {
		t.Errorf("qa.operation = %+v, want null on a not-deployed row", qa.Operation)
	}
}

func TestDeploymentService_Get_DetailedMessage(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"Stage": stageListJSON([]stageStub{
				{name: "my-pipeline-dev", deployment: "my-pipeline", env: "dev", cluster: "in-cluster", namespace: "my-pipeline-dev", trigger: "Auto", order: 0, status: "failed", message: "quota exceeded"},
				{name: "my-pipeline-qa", deployment: "my-pipeline", env: "qa", cluster: "in-cluster", namespace: "my-pipeline-qa", trigger: "Manual", order: 1, status: "created"},
			}),
		},
		getByName: map[string]string{
			"my-pipeline": cdPipelineGetJSONWithMessage("my-pipeline", []string{"foo"}, "gitops repository unreachable"),
		},
	}

	svc, closer := newDeploymentServiceForTest(t, rec)
	defer closer()

	d, err := svc.Get(context.Background(), "my-pipeline")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if d.DetailedMessage != "gitops repository unreachable" {
		t.Errorf("detailedMessage = %q, want the CDPipeline status.detailed_message", d.DetailedMessage)
	}

	if len(d.Stages) != 2 {
		t.Fatalf("got %d stages, want 2", len(d.Stages))
	}

	if d.Stages[0].DetailedMessage != "quota exceeded" || d.Stages[1].DetailedMessage != "" {
		t.Errorf("stage messages = %q / %q", d.Stages[0].DetailedMessage, d.Stages[1].DetailedMessage)
	}

	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if got := strings.Count(string(raw), `"detailedMessage"`); got != 2 {
		t.Errorf("detailedMessage should appear twice (pipeline + dev stage), got %d in %s", got, raw)
	}
}

func TestDeploymentService_List_DetailedMessage(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"CDPipeline": cdPipelineListJSON([]cdPipelineStub{
				{name: "broken", applications: []string{"foo"}, message: "gitops repository unreachable"},
				{name: "fine", applications: []string{"bar"}},
			}),
			"Stage": stageListJSON(nil),
		},
	}

	svc, closer := newDeploymentServiceForTest(t, rec)
	defer closer()

	rows, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	if rows[0].DetailedMessage != "gitops repository unreachable" || rows[1].DetailedMessage != "" {
		t.Errorf("row messages = %q / %q", rows[0].DetailedMessage, rows[1].DetailedMessage)
	}
}
