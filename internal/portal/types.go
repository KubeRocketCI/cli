package portal

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

const (
	QualityGateTypeAutotests = "autotests"
	QualityGateTypeManual    = "manual"
)

// QualityGate represents a quality gate step within a Stage.
type QualityGate struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
