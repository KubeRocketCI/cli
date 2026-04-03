package portal

import (
	"context"
	"fmt"
)

var codebaseConfig = k8sResourceConfig{
	APIVersion:   "v2.edp.epam.com/v1",
	Kind:         "Codebase",
	Group:        "v2.edp.epam.com",
	Version:      "v1",
	SingularName: "codebase",
	PluralName:   "codebases",
}

// ProjectService provides access to Codebase resources via the portal API.
type ProjectService struct {
	client      *Client
	clusterName string
	namespace   string
}

// NewProjectService creates a ProjectService for the given cluster and namespace.
func NewProjectService(client *Client, clusterName, namespace string) *ProjectService {
	return &ProjectService{client: client, clusterName: clusterName, namespace: namespace}
}

// List returns all Codebases in the configured namespace.
func (s *ProjectService) List(ctx context.Context) ([]Project, error) {
	input := k8sListInput{
		ClusterName:    s.clusterName,
		Namespace:      s.namespace,
		ResourceConfig: codebaseConfig,
	}

	var list k8sList
	if err := s.client.Query(ctx, procedureK8sList, input, &list); err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}

	projects := make([]Project, 0, len(list.Items))
	for _, item := range list.Items {
		projects = append(projects, mapProject(item))
	}

	return projects, nil
}

// Get returns a single Codebase by name.
func (s *ProjectService) Get(ctx context.Context, name string) (*Project, error) {
	input := k8sGetInput{
		ClusterName:    s.clusterName,
		Namespace:      s.namespace,
		Name:           name,
		ResourceConfig: codebaseConfig,
	}

	var obj map[string]any
	if err := s.client.Query(ctx, procedureK8sGet, input, &obj); err != nil {
		return nil, fmt.Errorf("getting project %q: %w", name, err)
	}

	p := mapProject(obj)

	return &p, nil
}

// mapProject extracts a Project from an unstructured Codebase resource.
func mapProject(obj map[string]any) Project {
	metadata := nestedMap(obj, "metadata")
	spec := nestedMap(obj, "spec")
	status := nestedMap(obj, "status")

	return Project{
		Name:      nestedString(metadata, "name"),
		Namespace: nestedString(metadata, "namespace"),
		Type:      nestedString(spec, "type"),
		Language:  nestedString(spec, "lang"),
		BuildTool: nestedString(spec, "buildTool"),
		Framework: nestedString(spec, "framework"),
		GitServer: nestedString(spec, "gitServer"),
		GitURL:    nestedString(status, "gitWebUrl"),
		Status:    nestedString(status, "status"),
		Available: isAvailable(status),
	}
}
