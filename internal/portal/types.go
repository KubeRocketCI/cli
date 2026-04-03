package portal

// k8sResourceConfig matches the portal's tRPC k8sResourceConfigSchema.
type k8sResourceConfig struct {
	APIVersion   string `json:"apiVersion"`
	Kind         string `json:"kind"`
	Group        string `json:"group"`
	Version      string `json:"version"`
	SingularName string `json:"singularName"`
	PluralName   string `json:"pluralName"`
}

// k8sListInput is the input for the portal's k8s.list tRPC procedure.
type k8sListInput struct {
	ClusterName    string            `json:"clusterName"`
	Namespace      string            `json:"namespace,omitempty"`
	ResourceConfig k8sResourceConfig `json:"resourceConfig"`
	Labels         map[string]string `json:"labels,omitempty"`
}

// k8sGetInput is the input for the portal's k8s.get tRPC procedure.
type k8sGetInput struct {
	ClusterName    string            `json:"clusterName"`
	Namespace      string            `json:"namespace,omitempty"`
	Name           string            `json:"name"`
	ResourceConfig k8sResourceConfig `json:"resourceConfig"`
}

// k8sList represents a Kubernetes List response with unstructured items.
type k8sList struct {
	Items []map[string]any `json:"items"`
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
