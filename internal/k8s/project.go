package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var codebaseGVR = schema.GroupVersionResource{
	Group:    "v2.edp.epam.com",
	Version:  "v1",
	Resource: "codebases",
}

// Project represents a KubeRocketCI Codebase resource.
type Project struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Language  string `json:"language"`
	BuildTool string `json:"buildTool"`
	Framework string `json:"framework,omitempty"`
	GitServer string `json:"gitServer"`
	GitURL    string `json:"gitUrl,omitempty"`
	Status    string `json:"status"`
	Available bool   `json:"available"`
}

// ProjectService provides access to Codebase resources.
type ProjectService struct {
	client    dynamic.Interface
	namespace string
}

// NewProjectService creates a ProjectService for the given namespace.
func NewProjectService(client dynamic.Interface, namespace string) *ProjectService {
	return &ProjectService{client: client, namespace: namespace}
}

// List returns all Codebases in the configured namespace.
func (s *ProjectService) List(ctx context.Context) ([]Project, error) {
	list, err := s.client.Resource(codebaseGVR).Namespace(s.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing codebases: %w", err)
	}

	projects := make([]Project, 0, len(list.Items))
	for i := range list.Items {
		projects = append(projects, mapProject(&list.Items[i]))
	}

	return projects, nil
}

// Get returns a single Codebase by name.
func (s *ProjectService) Get(ctx context.Context, name string) (*Project, error) {
	obj, err := s.client.Resource(codebaseGVR).Namespace(s.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting codebase %q: %w", name, err)
	}

	p := mapProject(obj)

	return &p, nil
}

// mapProject extracts a Project from an unstructured Codebase resource.
func mapProject(obj *unstructured.Unstructured) Project {
	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	status, _, _ := unstructured.NestedMap(obj.Object, "status")

	return Project{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
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
