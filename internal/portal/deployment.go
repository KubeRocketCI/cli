package portal

import (
	"context"
	"fmt"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

// newK8sResourceConfig assembles a K8sListJSONBody whose ResourceConfig
// identifies a single CRD by its group/version/kind triple. Centralizes the
// anonymous-struct shape from the generated restapi package so each new kind
// (CDPipeline, Stage, Application, …) reduces to one line.
func newK8sResourceConfig(group, version, kind, singular, plural string) restapi.K8sListJSONBody {
	return restapi.K8sListJSONBody{
		ResourceConfig: struct {
			ApiVersion    string             `json:"apiVersion"`
			ClusterScoped *bool              `json:"clusterScoped,omitempty"`
			Group         string             `json:"group"`
			Kind          string             `json:"kind"`
			Labels        *map[string]string `json:"labels,omitempty"`
			PluralName    string             `json:"pluralName"`
			SingularName  string             `json:"singularName"`
			Version       string             `json:"version"`
		}{
			ApiVersion:   group + "/" + version,
			Group:        group,
			Version:      version,
			Kind:         kind,
			SingularName: singular,
			PluralName:   plural,
		},
	}
}

var (
	cdPipelineResourceConfig = newK8sResourceConfig(
		"v2.edp.epam.com", "v1", "CDPipeline", "cdpipeline", "cdpipelines")
	stageResourceConfig = newK8sResourceConfig(
		"v2.edp.epam.com", "v1", "Stage", "stage", "stages")
	applicationResourceConfig = newK8sResourceConfig(
		"argoproj.io", "v1alpha1", "Application", "application", "applications")
)

// Canonical KRCI / ArgoCD label keys used by the deployment-discovery surface.
const (
	labelAppName        = "app.edp.epam.com/app-name"
	labelCDPipelineName = "app.edp.epam.com/cdPipelineName"
	labelStage          = "app.edp.epam.com/stage"
	labelPipeline       = "app.edp.epam.com/pipeline"
)

// k8sItem is the type of items returned by K8sList.
type k8sItem = restapi.K8sList_200_Items_Item

// buildK8sListBody assembles a K8sListJSONRequestBody from the given cluster,
// namespace, resource config, and optional label selector. Labels are omitted
// (kept nil) when empty.
func buildK8sListBody(
	clusterName, namespace string,
	rc restapi.K8sListJSONBody,
	labels map[string]string,
) restapi.K8sListJSONRequestBody {
	ns := namespace
	body := restapi.K8sListJSONRequestBody{
		ClusterName:    clusterName,
		Namespace:      &ns,
		ResourceConfig: rc.ResourceConfig,
	}

	if len(labels) > 0 {
		body.Labels = &labels
	}

	return body
}

// DeploymentService provides access to CDPipeline and Stage resources via the portal REST API.
type DeploymentService struct {
	client      *restapi.ClientWithResponses
	clusterName string
	namespace   string
}

// NewDeploymentService creates a DeploymentService for the given cluster and namespace.
func NewDeploymentService(client *restapi.ClientWithResponses, clusterName, namespace string) *DeploymentService {
	return &DeploymentService{client: client, clusterName: clusterName, namespace: namespace}
}

func (s *DeploymentService) k8sList(ctx context.Context, rc restapi.K8sListJSONBody) (*restapi.K8sListResponse, error) {
	return s.client.K8sListWithResponse(ctx, buildK8sListBody(s.clusterName, s.namespace, rc, nil))
}

// List returns all CDPipelines with their stage names ordered by spec.order.
func (s *DeploymentService) List(ctx context.Context) ([]Deployment, error) {
	var pipelineResp, stageResp *restapi.K8sListResponse

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		pipelineResp, err = s.k8sList(ctx, cdPipelineResourceConfig)
		if err != nil {
			return fmt.Errorf("listing cdpipelines: %w", err)
		}

		return checkResponse(pipelineResp.StatusCode(), pipelineResp.Body)
	})

	g.Go(func() error {
		var err error
		stageResp, err = s.k8sList(ctx, stageResourceConfig)
		if err != nil {
			return fmt.Errorf("listing stages: %w", err)
		}

		return checkResponse(stageResp.StatusCode(), stageResp.Body)
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if pipelineResp.JSON200 == nil {
		return nil, nil
	}

	var stageItems []k8sItem
	if stageResp.JSON200 != nil {
		stageItems = stageResp.JSON200.Items
	}

	stagesByPipeline := groupStagesByPipeline(stageItems)

	deployments := make([]Deployment, 0, len(pipelineResp.JSON200.Items))
	for _, item := range pipelineResp.JSON200.Items {
		d := mapDeployment(item)
		d.StageNames = orderedStageNames(stagesByPipeline[d.Name])
		deployments = append(deployments, d)
	}

	return deployments, nil
}

// Get returns a single CDPipeline with its Stages sorted by order.
func (s *DeploymentService) Get(ctx context.Context, name string) (*DeploymentDetail, error) {
	ns := s.namespace

	var getResp *restapi.K8sGetResponse
	var stageResp *restapi.K8sListResponse

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		body := restapi.K8sGetJSONRequestBody{
			ClusterName:    s.clusterName,
			Namespace:      &ns,
			Name:           name,
			ResourceConfig: cdPipelineResourceConfig.ResourceConfig,
		}

		var err error
		getResp, err = s.client.K8sGetWithResponse(ctx, body)
		if err != nil {
			return fmt.Errorf("getting cdpipeline %q: %w", name, err)
		}

		return checkResponse(getResp.StatusCode(), getResp.Body)
	})

	g.Go(func() error {
		var err error
		stageResp, err = s.k8sList(ctx, stageResourceConfig)
		if err != nil {
			return fmt.Errorf("listing stages for cdpipeline %q: %w", name, err)
		}

		return checkResponse(stageResp.StatusCode(), stageResp.Body)
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if getResp.JSON200 == nil {
		return nil, fmt.Errorf("cdpipeline %q: %w", name, ErrNotFound)
	}

	detail := mapDeploymentDetail(getResp.JSON200.Metadata, getResp.JSON200.Spec, getResp.JSON200.Status)

	var stageItems []k8sItem
	if stageResp.JSON200 != nil {
		stageItems = stageResp.JSON200.Items
	}

	detail.Stages = filterAndSortStages(stageItems, name)

	return &detail, nil
}

func mapDeployment(item k8sItem) Deployment {
	spec := ptr.Deref(item.Spec, nil)
	status := ptr.Deref(item.Status, nil)

	return Deployment{
		Name:         item.Metadata.Name,
		Namespace:    ptr.Deref(item.Metadata.Namespace, ""),
		Applications: stringSliceVal(spec, "applications"),
		Description:  stringVal(spec, "description"),
		Status:       stringVal(status, "status"),
		Available:    availableVal(status),
	}
}

func mapDeploymentDetail(meta restapi.K8sGet_200_Metadata, spec, status *map[string]any) DeploymentDetail {
	s := ptr.Deref(spec, nil)
	st := ptr.Deref(status, nil)

	return DeploymentDetail{
		Name:         meta.Name,
		Namespace:    ptr.Deref(meta.Namespace, ""),
		Applications: stringSliceVal(s, "applications"),
		Description:  stringVal(s, "description"),
		Status:       stringVal(st, "status"),
		Available:    availableVal(st),
	}
}

func mapStage(item k8sItem) Stage {
	spec := ptr.Deref(item.Spec, nil)
	status := ptr.Deref(item.Status, nil)

	return Stage{
		Name:         stringVal(spec, "name"),
		Order:        int64Val(spec, "order"),
		TriggerType:  stringVal(spec, "triggerType"),
		QualityGates: extractQualityGates(spec),
		Namespace:    stringVal(spec, "namespace"),
		ClusterName:  stringVal(spec, "clusterName"),
		Description:  stringVal(spec, "description"),
		Status:       stringVal(status, "status"),
		Available:    availableVal(status),
	}
}

func extractQualityGates(spec map[string]any) []QualityGate {
	raw, ok := spec["qualityGates"]
	if !ok {
		return nil
	}

	items, ok := raw.([]any)
	if !ok {
		return nil
	}

	gates := make([]QualityGate, 0, len(items))

	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		gates = append(gates, QualityGate{
			Name: stringVal(m, "stepName"),
			Type: stringVal(m, "qualityGateType"),
		})
	}

	return gates
}

func groupStagesByPipeline(items []k8sItem) map[string][]Stage {
	grouped := make(map[string][]Stage)

	for _, item := range items {
		spec := ptr.Deref(item.Spec, nil)
		pipelineName := stringVal(spec, "cdPipeline")

		if pipelineName == "" {
			continue
		}

		grouped[pipelineName] = append(grouped[pipelineName], mapStage(item))
	}

	return grouped
}

func orderedStageNames(stages []Stage) []string {
	sort.Slice(stages, func(i, j int) bool {
		return stages[i].Order < stages[j].Order
	})

	names := make([]string, 0, len(stages))
	for _, s := range stages {
		names = append(names, s.Name)
	}

	return names
}

func filterAndSortStages(items []k8sItem, pipelineName string) []Stage {
	var stages []Stage

	for _, item := range items {
		spec := ptr.Deref(item.Spec, nil)
		if stringVal(spec, "cdPipeline") != pipelineName {
			continue
		}

		stages = append(stages, mapStage(item))
	}

	sort.Slice(stages, func(i, j int) bool {
		return stages[i].Order < stages[j].Order
	})

	return stages
}

func stringSliceVal(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}

	raw, ok := m[key]
	if !ok {
		return nil
	}

	items, ok := raw.([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(items))

	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			continue
		}

		result = append(result, s)
	}

	return result
}

func int64Val(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}

	raw, ok := m[key]
	if !ok {
		return 0
	}

	switch v := raw.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

// stageOrder returns the integer Stage.spec.order. Defaults to 0 when missing
// or non-numeric.
func stageOrder(spec map[string]any) int {
	return int(int64Val(spec, "order"))
}

// findSliceItem returns the first map within slice whose entry at key equals
// val, or nil when no match exists.
func findSliceItem(slice []any, key, val string) map[string]any {
	for _, raw := range slice {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if stringVal(m, key) == val {
			return m
		}
	}

	return nil
}

// deepGet walks a nested *map[string]any tree by key. Returns nil if any
// intermediate key is missing or a non-map value blocks the path. Defensive
// against the schemaless `*map[string]any` shape returned by k8s.list.
func deepGet(m map[string]any, keys ...string) map[string]any {
	cur := m
	for _, k := range keys {
		if cur == nil {
			return nil
		}

		raw, ok := cur[k]
		if !ok {
			return nil
		}

		next, ok := raw.(map[string]any)
		if !ok {
			return nil
		}

		cur = next
	}

	return cur
}

// sliceVal returns the slice at m[key] as []any, or nil when missing or not a
// slice.
func sliceVal(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}

	raw, ok := m[key]
	if !ok {
		return nil
	}

	items, _ := raw.([]any)

	return items
}
