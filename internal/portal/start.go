package portal

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

type StartInput struct {
	Pipeline string            // DNS-1123 subdomain name of the Tekton Pipeline (max 253 chars)
	Params   map[string]string // user-supplied parameter overrides (may be nil)
	Labels   map[string]string // labels to attach to metadata.labels (may be nil)
	DryRun   bool              // true → render manifest without create
}

// StartResult is the row shape returned by `pipelinerun start`. Mirrors the
// `pipelinerun list` columns exactly so users pivot directly between the two
// verbs.
type StartResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Project  string `json:"project"`
	PR       string `json:"pr"`
	Author   string `json:"author"`
	Type     string `json:"type"`
	Started  string `json:"started"`
	Duration string `json:"duration"`

	// DryRunManifest is the rendered PipelineRun resource emitted when
	// DryRun=true; nil on the live-create path.
	DryRunManifest map[string]any `json:"dryRunManifest,omitempty"`
}

// Stable machine-readable error reasons surfaced by the Portal under
// `error.reason` (see `apps/server/src/config/openapi.ts handleTRPCError`).
// The Portal deliberately does not put resource-identifying text in
// `error.message` — `message` is always the static HTTP status phrase. The
// CLI must therefore key error mapping off `reason`, not the message.
//
// `reason=pipeline_not_found` and an absent reason both fall through to the
// default pipeline-not-found branch, so no constant is declared for it.
const (
	reasonTriggerTemplateNotFound = "trigger_template_not_found"
	reasonMalformedTTLabel        = "malformed_trigger_template_label"
)

// Discriminator values for the start-response oneOf body.
const (
	startKindCreated = "created"
	startKindDryRun  = "dryRun"
)

type PipelineRunStartService struct {
	client    *restapi.ClientWithResponses
	namespace string
}

func NewPipelineRunStartService(client *restapi.ClientWithResponses, namespace string) *PipelineRunStartService {
	return &PipelineRunStartService{client: client, namespace: namespace}
}

// Start calls `POST /rest/v1/pipelineruns/start`.
//
// Error mapping (Portal stable-reason contract):
//   - 200: returns *StartResult
//   - 400 reason=malformed_trigger_template_label: ErrPlatformReject (synthesised message)
//   - 400 default: ErrPlatformReject (generic — Portal hardening strips K8s admission detail)
//   - 401: ErrUnauthorized
//   - 403: ErrPermissionDenied (no resource metadata leak)
//   - 404 reason=trigger_template_not_found: ErrTriggerTemplateNotFound
//   - 404 default (incl. reason=pipeline_not_found): ErrPipelineNotFound
//   - 408/409/422/429: ErrPlatformReject (K8s admission classes; 422 covers
//     the common "missing required Pipeline param" case)
//   - 5xx: ErrUpstreamUnavailable
func (s *PipelineRunStartService) Start(ctx context.Context, in StartInput) (*StartResult, error) {
	body := restapi.PipelineRunStartJSONRequestBody{
		Namespace: s.namespace,
		Pipeline:  in.Pipeline,
	}

	if len(in.Params) > 0 {
		body.Params = ptr.To(in.Params)
	}

	if len(in.Labels) > 0 {
		body.Labels = ptr.To(in.Labels)
	}

	if in.DryRun {
		body.DryRun = ptr.To(true)
	}

	resp, err := s.client.PipelineRunStartWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("calling pipelinerun start: %w", err)
	}

	if err := checkStartResponse(resp.StatusCode(), resp.Body, in.Pipeline); err != nil {
		return nil, err
	}

	return decodeStartBody(resp.Body)
}

// checkStartResponse extends checkResponse with start-specific status mapping.
// Uses error.reason (not error.message) — Portal's handleTRPCError replaces
// message with the static HTTP phrase, stripping all resource names.
func checkStartResponse(statusCode int, body []byte, pipeline string) error {
	switch statusCode {
	case http.StatusBadRequest:
		reason, message := parseErrorEnvelope(body)

		if reason == reasonMalformedTTLabel {
			return fmt.Errorf("%w: pipeline '%s' has malformed TriggerTemplate label",
				ErrPlatformReject, pipeline)
		}
		// Portal strips K8s admission messages. Surface the generic
		// status phrase so the user knows to inspect the Pipeline
		// definition for missing required params or other admission-time
		// errors.
		return fmt.Errorf("%w: %s", ErrPlatformReject,
			cmp.Or(message, http.StatusText(http.StatusBadRequest)))
	case http.StatusForbidden:
		return ErrPermissionDenied
	case http.StatusNotFound:
		reason, _ := parseErrorEnvelope(body)
		if reason == reasonTriggerTemplateNotFound {
			return newNotFoundErr(
				fmt.Sprintf("pipeline '%s' references a TriggerTemplate that does not exist", pipeline),
				ErrTriggerTemplateNotFound,
			)
		}

		// reason=pipeline_not_found OR reason absent (e.g. plain-text 404
		// from a misbehaving proxy): default to pipeline-not-found with a
		// synthesised message. The pipeline name is known client-side, so we
		// never need the Portal to echo it.
		return newNotFoundErr(
			fmt.Sprintf("pipeline '%s' not found", pipeline),
			ErrPipelineNotFound,
		)
	case http.StatusRequestTimeout, http.StatusConflict,
		http.StatusUnprocessableEntity, http.StatusTooManyRequests:
		// K8s admission rejection. Portal's handleK8sError forwards
		// these without a stable reason tag, so we discriminate on
		// status code alone and surface the static HTTP phrase.
		_, message := parseErrorEnvelope(body)
		return fmt.Errorf("%w: %s", ErrPlatformReject,
			cmp.Or(message, http.StatusText(statusCode)))
	case http.StatusBadGateway, http.StatusServiceUnavailable,
		http.StatusInternalServerError, http.StatusGatewayTimeout:
		return fmt.Errorf("%w: %s", ErrUpstreamUnavailable, truncateBody(body))
	}

	return checkResponse(statusCode, body)
}

// parseErrorEnvelope reads error.reason and the user-facing message from a
// Portal error body. Returns ("", "") on parse failure. message prefers
// error.message, falling back to top-level message.
func parseErrorEnvelope(body []byte) (reason, message string) {
	var env struct {
		Error struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &env); err != nil {
		return "", ""
	}

	message = env.Error.Message
	if message == "" {
		message = env.Message
	}

	return env.Error.Reason, message
}

// decodeStartBody projects the discriminated-union 200 body into the flat
// StartResult. The portal procedure returns one of:
//
//	{"kind":"created","row":{...row fields...}}
//	{"kind":"dryRun","manifest":{...PipelineRun resource...}}
//
// oapi-codegen does not emit usable accessors for the `oneOf` (the union field
// is unexported), so we discriminate on `kind` against the raw response body.
func decodeStartBody(body []byte) (*StartResult, error) {
	var env struct {
		Kind     string          `json:"kind"`
		Row      json.RawMessage `json:"row"`
		Manifest json.RawMessage `json:"manifest"`
	}

	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decoding pipelinerun start response: %w", err)
	}

	switch env.Kind {
	case startKindCreated:
		var row StartResult
		if err := json.Unmarshal(env.Row, &row); err != nil {
			return nil, fmt.Errorf("decoding pipelinerun start created row: %w", err)
		}

		return &row, nil

	case startKindDryRun:
		var manifest map[string]any
		if err := json.Unmarshal(env.Manifest, &manifest); err != nil {
			return nil, fmt.Errorf("decoding pipelinerun start dry-run manifest: %w", err)
		}

		return &StartResult{DryRunManifest: manifest}, nil

	default:
		return nil, fmt.Errorf("unexpected pipelinerun start response kind: %q", env.Kind)
	}
}
