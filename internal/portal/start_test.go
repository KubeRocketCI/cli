package portal

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func newStartService(t *testing.T, handler http.HandlerFunc) (*PipelineRunStartService, func()) {
	t.Helper()

	client, closer := newTestClient(t, handler)

	return NewPipelineRunStartService(client, "edp"), closer
}

func TestStartService_Start_Success(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/pipelineruns/start" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"pipeline":"foo-build"`) {
			t.Fatalf("body missing pipeline: %s", body)
		}

		if !strings.Contains(string(body), `"namespace":"edp"`) {
			t.Fatalf("body missing namespace: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{
			"kind": "created",
			"row": {
				"name": "foo-build-run-x9k2p",
				"status": "Pending",
				"project": "",
				"pr": "",
				"author": "",
				"type": "build",
				"started": "",
				"duration": ""
			}
		}`))
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	got, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got.Name != "foo-build-run-x9k2p" {
		t.Fatalf("name = %q", got.Name)
	}

	if got.Status != "Pending" || got.Type != "build" {
		t.Fatalf("unexpected fields: %+v", got)
	}

	if len(got.DryRunManifest) != 0 {
		t.Fatalf("dry-run manifest should be empty on live path: %v", got.DryRunManifest)
	}
}

func TestStartService_Start_ParamsAndLabels_InBody(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		var got struct {
			Params map[string]string `json:"params"`
			Labels map[string]string `json:"labels"`
		}

		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if got.Params["git-revision"] != "main" {
			t.Errorf("params not forwarded: %+v", got.Params)
		}

		if got.Labels["app.edp.epam.com/codebase"] != "my-app" {
			t.Errorf("labels not forwarded: %+v", got.Labels)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"kind": "created",
			"row": {
				"name": "foo-build-run-abcde",
				"status": "Pending",
				"project": "my-app",
				"pr": "",
				"author": "",
				"type": "build",
				"started": "",
				"duration": ""
			}
		}`))
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	_, err := svc.Start(context.Background(), StartInput{
		Pipeline: "foo-build",
		Params:   map[string]string{"git-revision": "main"},
		Labels:   map[string]string{"app.edp.epam.com/codebase": "my-app"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestStartService_Start_DryRun(t *testing.T) {
	t.Parallel()

	// Portal returns the rendered draft as a JSON object (procedures/start/index.ts);
	// transport parses it at the wire boundary so the CLI receives a map.
	manifest := map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata": map[string]any{
			"generateName": "foo-build-run-",
		},
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		if !strings.Contains(string(body), `"dryRun":true`) {
			t.Fatalf("dryRun not forwarded: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")

		payload := map[string]any{
			"kind":     "dryRun",
			"manifest": manifest,
		}

		_ = json.NewEncoder(w).Encode(payload)
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	got, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build", DryRun: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got.DryRunManifest["kind"] != "PipelineRun" {
		t.Fatalf("dry-run manifest kind mismatch: %v", got.DryRunManifest["kind"])
	}

	metadata, ok := got.DryRunManifest["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("dry-run metadata not parsed as object: %T", got.DryRunManifest["metadata"])
	}

	if metadata["generateName"] != "foo-build-run-" {
		t.Fatalf("dry-run generateName mismatch: %v", metadata["generateName"])
	}

	if got.Name != "" {
		t.Fatalf("name should be empty on dry-run: %q", got.Name)
	}
}

func TestStartService_Start_PipelineNotFound(t *testing.T) {
	t.Parallel()

	// Portal sends the static HTTP status phrase as `error.message` and
	// communicates the failure mode via the stable `error.reason` tag.
	// Asserts: never expect the pipeline name in `error.message`.
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","reason":"pipeline_not_found","message":"Not Found"}}`))
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	_, err := svc.Start(context.Background(), StartInput{Pipeline: "ghost"})
	if !errors.Is(err, ErrPipelineNotFound) {
		t.Fatalf("want ErrPipelineNotFound, got %v", err)
	}

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound match too, got %v", err)
	}

	// CLI synthesises the user-facing message from the pipeline name
	// (which the caller already has) plus the reason tag.
	want := `pipeline 'ghost' not found`
	if err.Error() != want {
		t.Fatalf("want exact synthesised message %q, got: %v", want, err)
	}
}

func TestStartService_Start_PipelineNotFound_NoReasonOnPlainText(t *testing.T) {
	t.Parallel()

	// A plain-text 404 from a misbehaving proxy carries no reason tag.
	// The CLI defaults to pipeline-not-found with the synthesised message —
	// pipeline names always come from the caller, never the body.
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("plain text 404 from a misbehaving proxy"))
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	_, err := svc.Start(context.Background(), StartInput{Pipeline: "ghost"})
	if !errors.Is(err, ErrPipelineNotFound) {
		t.Fatalf("want ErrPipelineNotFound, got %v", err)
	}

	if !strings.Contains(err.Error(), `pipeline 'ghost' not found`) {
		t.Fatalf("expected synthesised message, got: %v", err)
	}
}

func TestStartService_Start_TriggerTemplateNotFound(t *testing.T) {
	t.Parallel()

	// Trigger-template-not-found path. Portal communicates the failure via
	// `reason`, not the message. The TriggerTemplate name is intentionally
	// NOT echoed (Portal hardening policy) — the CLI's synthesised message
	// therefore omits it.
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","reason":"trigger_template_not_found","message":"Not Found"}}`))
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	_, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build"})
	if !errors.Is(err, ErrTriggerTemplateNotFound) {
		t.Fatalf("want ErrTriggerTemplateNotFound, got %v", err)
	}

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound match too, got %v", err)
	}

	want := `pipeline 'foo-build' references a TriggerTemplate that does not exist`
	if err.Error() != want {
		t.Fatalf("want exact synthesised message %q, got: %v", want, err)
	}
}

func TestStartService_Start_MalformedTriggerTemplateLabel_400(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"BAD_REQUEST","reason":"malformed_trigger_template_label","message":"Bad Request"}}`))
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	_, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build"})
	if !errors.Is(err, ErrPlatformReject) {
		t.Fatalf("want ErrPlatformReject, got %v", err)
	}

	if !strings.Contains(err.Error(), "malformed TriggerTemplate label") {
		t.Fatalf("expected synthesised malformed-label message, got: %v", err)
	}
}

func TestStartService_Start_PlatformReject_400_NoReason(t *testing.T) {
	t.Parallel()

	// 400 with no reason tag → ErrPlatformReject + static HTTP phrase.
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"BAD_REQUEST","message":"Bad Request"}}`))
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	_, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build"})
	if !errors.Is(err, ErrPlatformReject) {
		t.Fatalf("want ErrPlatformReject, got %v", err)
	}

	if !strings.Contains(err.Error(), "Bad Request") {
		t.Fatalf("expected status phrase in error, got: %v", err)
	}
}

func TestStartService_Start_PlatformReject_422(t *testing.T) {
	t.Parallel()

	// 422 from the apiserver → ErrPlatformReject + static HTTP phrase.
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"code":"UNPROCESSABLE_CONTENT","message":"Unprocessable Entity"}}`))
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	_, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build"})
	if !errors.Is(err, ErrPlatformReject) {
		t.Fatalf("want ErrPlatformReject, got %v", err)
	}

	if !strings.Contains(err.Error(), "Unprocessable Entity") {
		t.Fatalf("expected status phrase in error, got: %v", err)
	}
}

func TestStartService_Start_PlatformReject_AdmissionStatuses(t *testing.T) {
	t.Parallel()

	// 408/409/429 follow the same pattern as 422: handleK8sError forwards
	// these K8s admission classes without a stable reason tag, and the CLI
	// surfaces ErrPlatformReject plus the static HTTP phrase.
	cases := []struct {
		code    int
		envCode string
	}{
		{http.StatusRequestTimeout, "TIMEOUT"},
		{http.StatusConflict, "CONFLICT"},
		{http.StatusTooManyRequests, "TOO_MANY_REQUESTS"},
	}

	for _, tc := range cases {
		t.Run(http.StatusText(tc.code), func(t *testing.T) {
			t.Parallel()

			body := fmt.Sprintf(`{"error":{"code":%q,"message":%q}}`, tc.envCode, http.StatusText(tc.code))
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(body))
			})

			svc, closer := newStartService(t, handler)
			defer closer()

			_, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build"})
			if !errors.Is(err, ErrPlatformReject) {
				t.Fatalf("status %d: want ErrPlatformReject, got %v", tc.code, err)
			}

			if !strings.Contains(err.Error(), http.StatusText(tc.code)) {
				t.Fatalf("status %d: expected status phrase %q in error, got: %v",
					tc.code, http.StatusText(tc.code), err)
			}
		})
	}
}

func TestStartService_Start_RBAC_403(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	_, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build"})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("want ErrPermissionDenied, got %v", err)
	}
}

func TestStartService_Start_Unauthorized_401(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}

	svc, closer := newStartService(t, handler)
	defer closer()

	_, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestStartService_Start_Upstream_5xx(t *testing.T) {
	t.Parallel()

	for _, code := range []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			t.Parallel()

			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				_, _ = w.Write([]byte("portal exploded"))
			})

			client, closer := newTestClient(t, handler)
			defer closer()

			svc := NewPipelineRunStartService(client, "edp")

			_, err := svc.Start(context.Background(), StartInput{Pipeline: "foo-build"})
			if !errors.Is(err, ErrUpstreamUnavailable) {
				t.Fatalf("status %d: want ErrUpstreamUnavailable, got %v", code, err)
			}
		})
	}
}

func TestParseErrorEnvelope_Reason(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want string
	}{
		{"with reason", `{"error":{"code":"NOT_FOUND","reason":"pipeline_not_found","message":"Not Found"}}`, "pipeline_not_found"},
		{"no reason field", `{"error":{"code":"NOT_FOUND","message":"Not Found"}}`, ""},
		{"empty body", "", ""},
		{"plain text body", "Not Found", ""},
		{"malformed JSON", `{not json`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, _ := parseErrorEnvelope([]byte(tc.body)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseErrorEnvelope_Message(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		body     string
		fallback string
		want     string
	}{
		{"with nested message", `{"error":{"message":"boom"}}`, "fb", "boom"},
		{"no message", `{"error":{"code":"X"}}`, "fb", "fb"},
		{"empty message falls back", `{"error":{"message":""}}`, "fb", "fb"},
		{"empty body", "", "fb", "fb"},
		{"top-level message", `{"message":"top"}`, "fb", "top"},
		{"malformed JSON falls back", `{not json`, "fb", "fb"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, msg := parseErrorEnvelope([]byte(tc.body))
			if got := cmp.Or(msg, tc.fallback); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
