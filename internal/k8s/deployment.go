package k8s

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var cdPipelineGVR = schema.GroupVersionResource{
	Group:    "v2.edp.epam.com",
	Version:  "v1",
	Resource: "cdpipelines",
}

var stageGVR = schema.GroupVersionResource{
	Group:    "v2.edp.epam.com",
	Version:  "v1",
	Resource: "stages",
}

// Deployment represents a KubeRocketCI CDPipeline resource.
type Deployment struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	Applications []string `json:"applications"`
	StageNames   []string `json:"stages"`
	Description  string   `json:"description,omitempty"`
	Status       string   `json:"status"`
	Available    bool     `json:"available"`
}

// DeploymentDetail represents a CDPipeline with its associated Stages.
type DeploymentDetail struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	Applications []string `json:"applications"`
	Description  string   `json:"description,omitempty"`
	Status       string   `json:"status"`
	Available    bool     `json:"available"`
	Stages       []Stage  `json:"stages"`
}

// Stage represents a KubeRocketCI Stage resource belonging to a CDPipeline.
type Stage struct {
	Name         string        `json:"name"`
	Order        int64         `json:"order"`
	TriggerType  string        `json:"triggerType"`
	QualityGates []QualityGate `json:"qualityGates"`
	Namespace    string        `json:"namespace"`
	ClusterName  string        `json:"clusterName,omitempty"`
	Description  string        `json:"description,omitempty"`
	Status       string        `json:"status"`
	Available    bool          `json:"available"`
}

// Quality gate type constants.
const (
	QualityGateTypeAutotests = "autotests"
	QualityGateTypeManual    = "manual"
)

// QualityGate represents a quality gate step within a Stage.
type QualityGate struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// DeploymentService provides access to CDPipeline and Stage resources.
type DeploymentService struct {
	client    dynamic.Interface
	namespace string
}

// NewDeploymentService creates a DeploymentService for the given namespace.
func NewDeploymentService(client dynamic.Interface, namespace string) *DeploymentService {
	return &DeploymentService{client: client, namespace: namespace}
}

// List returns all CDPipelines with their stage names ordered by spec.order.
func (s *DeploymentService) List(ctx context.Context) ([]Deployment, error) {
	pipelineList, err := s.client.Resource(cdPipelineGVR).Namespace(s.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing cdpipelines: %w", err)
	}

	stageList, err := s.client.Resource(stageGVR).Namespace(s.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing stages: %w", err)
	}

	stagesByPipeline := groupStagesByPipeline(stageList.Items)

	deployments := make([]Deployment, 0, len(pipelineList.Items))
	for i := range pipelineList.Items {
		d := mapDeployment(&pipelineList.Items[i])
		d.StageNames = orderedStageNames(stagesByPipeline[d.Name])
		deployments = append(deployments, d)
	}

	return deployments, nil
}

// Get returns a single CDPipeline with its Stages sorted by order.
func (s *DeploymentService) Get(ctx context.Context, name string) (*DeploymentDetail, error) {
	obj, err := s.client.Resource(cdPipelineGVR).Namespace(s.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting cdpipeline %q: %w", name, err)
	}

	stageList, err := s.client.Resource(stageGVR).Namespace(s.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing stages for cdpipeline %q: %w", name, err)
	}

	detail := mapDeploymentDetail(obj)
	detail.Stages = filterAndSortStages(stageList.Items, name)

	return &detail, nil
}

// mapDeployment extracts a Deployment from an unstructured CDPipeline resource.
func mapDeployment(obj *unstructured.Unstructured) Deployment {
	apps, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "applications")
	status, _, _ := unstructured.NestedMap(obj.Object, "status")
	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")

	return Deployment{
		Name:         obj.GetName(),
		Namespace:    obj.GetNamespace(),
		Applications: apps,
		Description:  nestedString(spec, "description"),
		Status:       nestedString(status, "status"),
		Available:    isAvailable(status),
	}
}

// mapDeploymentDetail extracts a DeploymentDetail from an unstructured CDPipeline resource.
func mapDeploymentDetail(obj *unstructured.Unstructured) DeploymentDetail {
	apps, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "applications")
	status, _, _ := unstructured.NestedMap(obj.Object, "status")
	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")

	return DeploymentDetail{
		Name:         obj.GetName(),
		Namespace:    obj.GetNamespace(),
		Applications: apps,
		Description:  nestedString(spec, "description"),
		Status:       nestedString(status, "status"),
		Available:    isAvailable(status),
	}
}

// mapStage extracts a Stage from an unstructured Stage resource.
func mapStage(obj *unstructured.Unstructured) Stage {
	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	status, _, _ := unstructured.NestedMap(obj.Object, "status")
	order, _, _ := unstructured.NestedInt64(obj.Object, "spec", "order")

	return Stage{
		Name:         nestedString(spec, "name"),
		Order:        order,
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
func groupStagesByPipeline(stages []unstructured.Unstructured) map[string][]Stage {
	grouped := make(map[string][]Stage)

	for i := range stages {
		pipelineName := nestedStringFromObj(&stages[i], "spec", "cdPipeline")
		if pipelineName == "" {
			continue
		}

		grouped[pipelineName] = append(grouped[pipelineName], mapStage(&stages[i]))
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
func filterAndSortStages(items []unstructured.Unstructured, pipelineName string) []Stage {
	var stages []Stage

	for i := range items {
		cdPipeline := nestedStringFromObj(&items[i], "spec", "cdPipeline")
		if cdPipeline != pipelineName {
			continue
		}

		stages = append(stages, mapStage(&items[i]))
	}

	sort.Slice(stages, func(i, j int) bool {
		return stages[i].Order < stages[j].Order
	})

	return stages
}

// nestedStringFromObj extracts a nested string value from an unstructured object.
func nestedStringFromObj(obj *unstructured.Unstructured, fields ...string) string {
	val, found, err := unstructured.NestedString(obj.Object, fields...)
	if err != nil || !found {
		return ""
	}

	return val
}
