package portal

import (
	"context"
	"fmt"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

var codebaseConfig = restapi.K8sListJSONBody{
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
		Kind:         "Codebase",
		Group:        "v2.edp.epam.com",
		Version:      "v1",
		SingularName: "codebase",
		PluralName:   "codebases",
	},
}

// ProjectService provides access to Codebase resources via the portal REST API.
type ProjectService struct {
	client      *restapi.ClientWithResponses
	clusterName string
	namespace   string
}

// NewProjectService creates a ProjectService for the given cluster and namespace.
func NewProjectService(client *restapi.ClientWithResponses, clusterName, namespace string) *ProjectService {
	return &ProjectService{client: client, clusterName: clusterName, namespace: namespace}
}

// List returns all Codebases in the configured namespace.
func (s *ProjectService) List(ctx context.Context) ([]Project, error) {
	ns := s.namespace
	body := restapi.K8sListJSONRequestBody{
		ClusterName:    s.clusterName,
		Namespace:      &ns,
		ResourceConfig: codebaseConfig.ResourceConfig,
	}

	resp, err := s.client.K8sListWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}

	if resp.JSON200 == nil {
		return nil, nil
	}

	projects := make([]Project, 0, len(resp.JSON200.Items))
	for _, item := range resp.JSON200.Items {
		p := mapK8sProject(
			item.Metadata.Name, ptr.Deref(item.Metadata.Namespace, ""),
			item.Spec, item.Status,
		)
		projects = append(projects, p)
	}

	return projects, nil
}

// Get returns a single Codebase by name.
func (s *ProjectService) Get(ctx context.Context, name string) (*Project, error) {
	ns := s.namespace
	body := restapi.K8sGetJSONRequestBody{
		ClusterName:    s.clusterName,
		Namespace:      &ns,
		Name:           name,
		ResourceConfig: codebaseConfig.ResourceConfig,
	}

	resp, err := s.client.K8sGetWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("getting project %q: %w", name, err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, fmt.Errorf("getting project %q: %w", name, err)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("project %q: %w", name, ErrNotFound)
	}

	p := mapK8sProject(
		resp.JSON200.Metadata.Name, ptr.Deref(resp.JSON200.Metadata.Namespace, ""),
		resp.JSON200.Spec, resp.JSON200.Status,
	)

	return &p, nil
}

// mapK8sProject extracts a Project from typed K8s resource fields.
func mapK8sProject(name, namespace string, spec, status *map[string]any) Project {
	s := ptr.Deref(spec, nil)
	st := ptr.Deref(status, nil)

	return Project{
		Name:      name,
		Namespace: namespace,
		Type:      stringVal(s, "type"),
		Language:  stringVal(s, "lang"),
		BuildTool: stringVal(s, "buildTool"),
		Framework: stringVal(s, "framework"),
		GitServer: stringVal(s, "gitServer"),
		GitURL:    stringVal(st, "gitWebUrl"),
		Status:    stringVal(st, "status"),
		Available: availableVal(st),
	}
}

func stringVal(m map[string]any, key string) string {
	if m == nil {
		return ""
	}

	v, ok := m[key]
	if !ok {
		return ""
	}

	s, _ := v.(string)

	return s
}

func availableVal(m map[string]any) bool {
	if m == nil {
		return false
	}

	v, ok := m["available"]
	if !ok {
		return false
	}

	b, _ := v.(bool)

	return b
}
