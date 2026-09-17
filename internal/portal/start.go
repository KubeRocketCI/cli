package portal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

type StartInput struct {
	Pipeline string            // Kubernetes name of the Tekton Pipeline
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

// Stable machine-readable reasons the portal reports under `error.reason` for
// `POST /rest/v1/pipelineruns/start` (see `apps/server/src/config/openapi.ts
// handleTRPCError`). `error.message` is always the static HTTP status phrase;
// map on the reason. trigger_template_not_found is shared with
// build.
const (
	reasonPipelineNotFound        = "pipeline_not_found"
	reasonTriggerTemplateNotFound = "trigger_template_not_found"
	reasonMalformedTTLabel        = "malformed_trigger_template_label"
)

// startReasons renders with %[1]s = pipeline name.
var startReasons = reasonTable{
	reasonPipelineNotFound: {
		sentinel: ErrPipelineNotFound,
		format:   "pipeline '%[1]s' not found",
	},
	reasonTriggerTemplateNotFound: {
		sentinel: ErrTriggerTemplateNotFound,
		format:   "pipeline '%[1]s' references a TriggerTemplate that does not exist",
	},
	reasonMalformedTTLabel: {
		sentinel: ErrPlatformReject,
		format:   "pipeline '%[1]s' has malformed TriggerTemplate label",
	},
}

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

// Start calls `POST /rest/v1/pipelineruns/start`. Errors: startReasons by
// reason tag; a 404 without a reason is ErrPipelineNotFound; the rest per
// checkReasonedResponse.
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

// checkStartResponse maps a start response. A 404 without a reason tag (e.g.
// a plain-text 404 from a proxy) maps to ErrPipelineNotFound.
func checkStartResponse(statusCode int, body []byte, pipeline string) error {
	return checkReasonedResponse(statusCode, body, startReasons, startReasons[reasonPipelineNotFound], pipeline)
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
