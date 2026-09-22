package portal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
)

type tagStub struct {
	name    string
	created string
	digest  string
}

// imageStreamStub describes one CodebaseImageStream: the branch resource it
// belongs to (label, may be empty) and its tags (nil → spec.tags omitted).
type imageStreamStub struct {
	name      string
	codebase  string
	branchRes string
	image     string
	tags      []tagStub
}

func imageStreamListJSON(items []imageStreamStub) string {
	type metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels,omitempty"`
	}

	type stream struct {
		Metadata metadata       `json:"metadata"`
		Spec     map[string]any `json:"spec"`
		Status   map[string]any `json:"status"`
	}

	out := make([]stream, 0, len(items))
	for _, it := range items {
		labels := map[string]string{"app.edp.epam.com/codebase": it.codebase}
		if it.branchRes != "" {
			labels["app.edp.epam.com/codebasebranch"] = it.branchRes
		}

		spec := map[string]any{"codebase": it.codebase, "imageName": it.image}

		if it.tags != nil {
			tags := make([]any, 0, len(it.tags))
			for _, tg := range it.tags {
				entry := map[string]any{"name": tg.name, "created": tg.created}
				if tg.digest != "" {
					entry["digest"] = tg.digest
				}
				tags = append(tags, entry)
			}
			spec["tags"] = tags
		}

		out = append(out, stream{
			Metadata: metadata{Name: it.name, Namespace: "ns", Labels: labels},
			Spec:     spec,
			Status:   map[string]any{},
		})
	}

	return mustJSON(map[string]any{"apiVersion": "v1", "kind": "List", "metadata": map[string]any{}, "items": out})
}

type branchStub struct {
	name     string // CodebaseBranch resource name
	codebase string
	branch   string // spec.branchName
}

func codebaseBranchListJSON(items []branchStub) string {
	type metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels,omitempty"`
	}

	type cb struct {
		Metadata metadata       `json:"metadata"`
		Spec     map[string]any `json:"spec"`
		Status   map[string]any `json:"status"`
	}

	out := make([]cb, 0, len(items))
	for _, it := range items {
		out = append(out, cb{
			Metadata: metadata{
				Name:      it.name,
				Namespace: "ns",
				Labels: map[string]string{
					"app.edp.epam.com/codebase":     it.codebase,
					"app.edp.epam.com/codebaseName": it.codebase,
				},
			},
			Spec:   map[string]any{"branchName": it.branch, "codebaseName": it.codebase},
			Status: map[string]any{"status": "created"},
		})
	}

	return mustJSON(map[string]any{"apiVersion": "v1", "kind": "List", "metadata": map[string]any{}, "items": out})
}

func codebaseGetJSON(name string) string {
	return mustJSON(map[string]any{
		"apiVersion": "v2.edp.epam.com/v1",
		"kind":       "Codebase",
		"metadata":   map[string]any{"name": name, "namespace": "ns"},
		"spec":       map[string]any{"type": "application", "lang": "go", "buildTool": "go", "gitServer": "github"},
		"status":     map[string]any{"status": "created", "available": true},
	})
}

func newProjectVersionsServiceForTest(t *testing.T, rec *envTestRecorder) (*ProjectVersionsService, func()) {
	t.Helper()

	url, closer := newEnvTestServer(t, rec)

	client, err := restapi.NewClientWithResponses(url + "/rest")
	if err != nil {
		closer()
		t.Fatalf("new client: %v", err)
	}

	return NewProjectVersionsService(client, "in-cluster", "ns"), closer
}

func versionsFixture(t *testing.T) *envTestRecorder {
	t.Helper()

	return &envTestRecorder{
		t: t,
		listByKind: map[string]string{
			"CodebaseImageStream": imageStreamListJSON([]imageStreamStub{
				{
					name: "my-app-release-2-27-781f0", codebase: "my-app", branchRes: "my-app-release-2-27-781f0",
					image: "registry.example.com/ns/my-app",
					tags:  []tagStub{{name: "2.27.0-SNAPSHOT.1", created: "2026-09-01T10:00:00Z"}},
				},
				{
					name: "my-app-main", codebase: "my-app", branchRes: "my-app-main",
					image: "registry.example.com/ns/my-app",
					tags: []tagStub{
						{name: "0.1.0-SNAPSHOT.1", created: "2026-08-30T09:00:00Z", digest: "sha256:aaaa1111bbbb2222"},
						{name: "0.1.0-SNAPSHOT.2", created: "2026-09-02T09:00:00Z", digest: "sha256:cccc3333dddd4444"},
					},
				},
				{name: "my-app-feature-x", codebase: "my-app", image: "registry.example.com/ns/my-app"},
			}),
			"CodebaseBranch": codebaseBranchListJSON([]branchStub{
				{name: "my-app-main", codebase: "my-app", branch: "main"},
				{name: "my-app-release-2-27-781f0", codebase: "my-app", branch: "release/2.27"},
			}),
		},
		getByName: map[string]string{"my-app": codebaseGetJSON("my-app")},
	}
}

func TestProjectVersionsService_List_GroupsByBranch(t *testing.T) {
	t.Parallel()

	rec := versionsFixture(t)

	svc, closer := newProjectVersionsServiceForTest(t, rec)
	defer closer()

	streams, err := svc.List(context.Background(), "my-app", "")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(streams) != 3 {
		t.Fatalf("got %d streams, want 3: %+v", len(streams), streams)
	}

	wantBranches := []string{"feature-x", "main", "release/2.27"}
	for i, s := range streams {
		if s.Branch != wantBranches[i] {
			t.Errorf("streams[%d].branch = %q, want %q (sorted by branch)", i, s.Branch, wantBranches[i])
		}

		if s.Image != "registry.example.com/ns/my-app" {
			t.Errorf("streams[%d].image = %q", i, s.Image)
		}
	}

	featureX := streams[0]
	if featureX.Versions == nil || len(featureX.Versions) != 0 {
		t.Errorf("a stream without tags must carry an empty versions array, got %+v", featureX.Versions)
	}

	main := streams[1]
	if len(main.Versions) != 2 || main.Versions[0].Name != "0.1.0-SNAPSHOT.2" || main.Versions[1].Name != "0.1.0-SNAPSHOT.1" {
		t.Errorf("main versions must be newest first, got %+v", main.Versions)
	}

	if main.Versions[0].Digest != "sha256:cccc3333dddd4444" || main.Versions[0].Created != "2026-09-02T09:00:00Z" {
		t.Errorf("main.versions[0] = %+v", main.Versions[0])
	}

	for _, call := range rec.calls {
		if call.Kind == "CodebaseImageStream" || call.Kind == "CodebaseBranch" {
			if got := call.LabelSelectors["app.edp.epam.com/codebase"]; got != "my-app" {
				t.Errorf("%s list must be scoped by the codebase label, got %v", call.Kind, call.LabelSelectors)
			}
		}
	}

	raw, err := json.Marshal(ProjectVersionsPayload{Project: "my-app", Streams: streams})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, want := range []string{`"streams":[`, `"branch":"feature-x"`, `"versions":[]`, `"digest":"sha256:cccc3333dddd4444"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("JSON missing %s: %s", want, raw)
		}
	}
}

func TestProjectVersionsService_List_BranchFilter(t *testing.T) {
	t.Parallel()

	rec := versionsFixture(t)

	svc, closer := newProjectVersionsServiceForTest(t, rec)
	defer closer()

	streams, err := svc.List(context.Background(), "my-app", "release/2.27")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(streams) != 1 || streams[0].Branch != "release/2.27" || len(streams[0].Versions) != 1 {
		t.Fatalf("branch filter should keep only release/2.27, got %+v", streams)
	}

	none, err := svc.List(context.Background(), "my-app", "does-not-exist")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if none == nil || len(none) != 0 {
		t.Errorf("unknown branch should yield an empty array, got %+v", none)
	}
}

func TestProjectVersionsService_List_NoStreams(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{
		t:          t,
		listByKind: map[string]string{},
		getByName:  map[string]string{"my-app": codebaseGetJSON("my-app")},
	}

	svc, closer := newProjectVersionsServiceForTest(t, rec)
	defer closer()

	streams, err := svc.List(context.Background(), "my-app", "")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if streams == nil || len(streams) != 0 {
		t.Errorf("a project without image streams should yield an empty array, got %+v", streams)
	}
}

func TestProjectVersionsService_List_ProjectNotFound(t *testing.T) {
	t.Parallel()

	rec := &envTestRecorder{t: t, listByKind: map[string]string{}, getByName: map[string]string{}}

	svc, closer := newProjectVersionsServiceForTest(t, rec)
	defer closer()

	_, err := svc.List(context.Background(), "ghost", "")
	if err == nil {
		t.Fatal("expected an error for an unknown project")
	}

	if !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("errors.Is(err, ErrProjectNotFound) = false, err = %v", err)
	}

	if !strings.Contains(err.Error(), "project 'ghost' not found") {
		t.Errorf("error = %q, want project 'ghost' not found", err.Error())
	}
}
