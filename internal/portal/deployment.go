package portal

import (
	"context"
	"fmt"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

var cdPipelineResourceConfig = restapi.K8sListJSONBody{
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
		ApiVersion:   "v2.edp.epam.com/v1",
		Kind:         "CDPipeline",
		Group:        "v2.edp.epam.com",
		Version:      "v1",
		SingularName: "cdpipeline",
		PluralName:   "cdpipelines",
	},
}

var stageResourceConfig = restapi.K8sListJSONBody{
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
		ApiVersion:   "v2.edp.epam.com/v1",
		Kind:         "Stage",
		Group:        "v2.edp.epam.com",
		Version:      "v1",
		SingularName: "stage",
		PluralName:   "stages",
	},
}

// k8sItem is the type of items returned by K8sList.
type k8sItem = restapi.K8sList_200_Items_Item

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
	ns := s.namespace
	body := restapi.K8sListJSONRequestBody{
		ClusterName:    s.clusterName,
		Namespace:      &ns,
		ResourceConfig: rc.ResourceConfig,
	}

	return s.client.K8sListWithResponse(ctx, body)
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
