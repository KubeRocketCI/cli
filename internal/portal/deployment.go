package portal

import (
	"context"
	"fmt"
	"sort"

	"golang.org/x/sync/errgroup"
)

var cdPipelineConfig = k8sResourceConfig{
	APIVersion:   "v2.edp.epam.com/v1",
	Kind:         "CDPipeline",
	Group:        "v2.edp.epam.com",
	Version:      "v1",
	SingularName: "cdpipeline",
	PluralName:   "cdpipelines",
}

var stageConfig = k8sResourceConfig{
	APIVersion:   "v2.edp.epam.com/v1",
	Kind:         "Stage",
	Group:        "v2.edp.epam.com",
	Version:      "v1",
	SingularName: "stage",
	PluralName:   "stages",
}

// DeploymentService provides access to CDPipeline and Stage resources via the portal API.
type DeploymentService struct {
	client      *Client
	clusterName string
	namespace   string
}

// NewDeploymentService creates a DeploymentService for the given cluster and namespace.
func NewDeploymentService(client *Client, clusterName, namespace string) *DeploymentService {
	return &DeploymentService{client: client, clusterName: clusterName, namespace: namespace}
}

// List returns all CDPipelines with their stage names ordered by spec.order.
func (s *DeploymentService) List(ctx context.Context) ([]Deployment, error) {
	pipelineInput := k8sListInput{
		ClusterName:    s.clusterName,
		Namespace:      s.namespace,
		ResourceConfig: cdPipelineConfig,
	}

	stageInput := k8sListInput{
		ClusterName:    s.clusterName,
		Namespace:      s.namespace,
		ResourceConfig: stageConfig,
	}

	var pipelineList k8sList
	var stageList k8sList

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := s.client.Query(ctx, procedureK8sList, pipelineInput, &pipelineList); err != nil {
			return fmt.Errorf("listing cdpipelines: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		if err := s.client.Query(ctx, procedureK8sList, stageInput, &stageList); err != nil {
			return fmt.Errorf("listing stages: %w", err)
		}

		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	stagesByPipeline := groupStagesByPipeline(stageList.Items)

	deployments := make([]Deployment, 0, len(pipelineList.Items))
	for _, item := range pipelineList.Items {
		d := mapDeployment(item)
		d.StageNames = orderedStageNames(stagesByPipeline[d.Name])
		deployments = append(deployments, d)
	}

	return deployments, nil
}

// Get returns a single CDPipeline with its Stages sorted by order.
func (s *DeploymentService) Get(ctx context.Context, name string) (*DeploymentDetail, error) {
	pipelineInput := k8sGetInput{
		ClusterName:    s.clusterName,
		Namespace:      s.namespace,
		Name:           name,
		ResourceConfig: cdPipelineConfig,
	}

	stageInput := k8sListInput{
		ClusterName:    s.clusterName,
		Namespace:      s.namespace,
		ResourceConfig: stageConfig,
	}

	var obj map[string]any
	var stageList k8sList

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := s.client.Query(ctx, procedureK8sGet, pipelineInput, &obj); err != nil {
			return fmt.Errorf("getting cdpipeline %q: %w", name, err)
		}

		return nil
	})

	g.Go(func() error {
		if err := s.client.Query(ctx, procedureK8sList, stageInput, &stageList); err != nil {
			return fmt.Errorf("listing stages for cdpipeline %q: %w", name, err)
		}

		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	detail := mapDeploymentDetail(obj)
	detail.Stages = filterAndSortStages(stageList.Items, name)

	return &detail, nil
}

func mapDeployment(obj map[string]any) Deployment {
	metadata := nestedMap(obj, "metadata")
	spec := nestedMap(obj, "spec")
	status := nestedMap(obj, "status")

	return Deployment{
		Name:         nestedString(metadata, "name"),
		Namespace:    nestedString(metadata, "namespace"),
		Applications: nestedStringSlice(obj, "spec", "applications"),
		Description:  nestedString(spec, "description"),
		Status:       nestedString(status, "status"),
		Available:    isAvailable(status),
	}
}

func mapDeploymentDetail(obj map[string]any) DeploymentDetail {
	metadata := nestedMap(obj, "metadata")
	spec := nestedMap(obj, "spec")
	status := nestedMap(obj, "status")

	return DeploymentDetail{
		Name:         nestedString(metadata, "name"),
		Namespace:    nestedString(metadata, "namespace"),
		Applications: nestedStringSlice(obj, "spec", "applications"),
		Description:  nestedString(spec, "description"),
		Status:       nestedString(status, "status"),
		Available:    isAvailable(status),
	}
}

// mapStage extracts a Stage from an unstructured Stage resource.
func mapStage(obj map[string]any) Stage {
	spec := nestedMap(obj, "spec")
	status := nestedMap(obj, "status")

	return Stage{
		Name:         nestedString(spec, "name"),
		Order:        nestedInt64(obj, "spec", "order"),
		TriggerType:  nestedString(spec, "triggerType"),
		QualityGates: extractQualityGates(spec),
		Namespace:    nestedString(spec, "namespace"),
		ClusterName:  nestedString(spec, "clusterName"),
		Description:  nestedString(spec, "description"),
		Status:       nestedString(status, "status"),
		Available:    isAvailable(status),
	}
}

// extractQualityGates parses the qualityGates array from a Stage spec.
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
			Name: nestedString(m, "stepName"),
			Type: nestedString(m, "qualityGateType"),
		})
	}

	return gates
}

// groupStagesByPipeline groups stages by their spec.cdPipeline field.
func groupStagesByPipeline(items []map[string]any) map[string][]Stage {
	grouped := make(map[string][]Stage)

	for _, item := range items {
		spec := nestedMap(item, "spec")
		pipelineName := nestedString(spec, "cdPipeline")

		if pipelineName == "" {
			continue
		}

		grouped[pipelineName] = append(grouped[pipelineName], mapStage(item))
	}

	return grouped
}

// orderedStageNames returns stage names sorted by their Order field.
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

// filterAndSortStages returns stages matching the given pipeline name, sorted by order.
func filterAndSortStages(items []map[string]any, pipelineName string) []Stage {
	var stages []Stage

	for _, item := range items {
		spec := nestedMap(item, "spec")
		if nestedString(spec, "cdPipeline") != pipelineName {
			continue
		}

		stages = append(stages, mapStage(item))
	}

	sort.Slice(stages, func(i, j int) bool {
		return stages[i].Order < stages[j].Order
	})

	return stages
}
