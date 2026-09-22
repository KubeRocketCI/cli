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
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	Applications    []string `json:"applications"`
	StageNames      []string `json:"stages"`
	Description     string   `json:"description,omitempty"`
	Status          string   `json:"status"`
	DetailedMessage string   `json:"detailedMessage,omitempty"`
	Available       bool     `json:"available"`
}

// DeploymentDetail represents a CDPipeline with its associated Stages.
type DeploymentDetail struct {
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	Applications    []string `json:"applications"`
	Description     string   `json:"description,omitempty"`
	Status          string   `json:"status"`
	DetailedMessage string   `json:"detailedMessage,omitempty"`
	Available       bool     `json:"available"`
	Stages          []Stage  `json:"stages"`
}

// Stage represents a KubeRocketCI Stage resource belonging to a CDPipeline.
// DetailedMessage carries the operator's status.detailed_message, the reason
// behind a failed status, and is omitted when the resource has none.
type Stage struct {
	Name            string        `json:"name"`
	Order           int64         `json:"order"`
	TriggerType     string        `json:"triggerType"`
	QualityGates    []QualityGate `json:"qualityGates"`
	Namespace       string        `json:"namespace"`
	ClusterName     string        `json:"clusterName,omitempty"`
	Description     string        `json:"description,omitempty"`
	Status          string        `json:"status"`
	DetailedMessage string        `json:"detailedMessage,omitempty"`
	Available       bool          `json:"available"`
}

const (
	QualityGateTypeAutotests = "autotests"
	QualityGateTypeManual    = "manual"
)

// ArgoHealthStatus is one of ArgoCD's documented health-status values for an
// Application's `status.health.status` field. The CLI surfaces these in
// lowercase across `env get` and `project deployments`; renderers use the
// constants below to dispatch coloring without raw-string switches.
type ArgoHealthStatus string

const (
	ArgoHealthHealthy     ArgoHealthStatus = "healthy"
	ArgoHealthDegraded    ArgoHealthStatus = "degraded"
	ArgoHealthMissing     ArgoHealthStatus = "missing"
	ArgoHealthProgressing ArgoHealthStatus = "progressing"
	ArgoHealthSuspended   ArgoHealthStatus = "suspended"
	ArgoHealthUnknown     ArgoHealthStatus = "unknown"
)

// AppCondition is one entry of an Argo CD Application's status.conditions:
// the reason a comparison, sync, or health evaluation did not complete, such
// as an unreachable target cluster or a chart that fails to render.
type AppCondition struct {
	Type               string  `json:"type"`
	Message            string  `json:"message"`
	LastTransitionTime *string `json:"lastTransitionTime"`
}

// AppOperation summarizes the last sync operation of an Argo CD Application
// (status.operationState). Phase is one of Argo CD's operation phases
// (Running, Succeeded, Failed, Error, Terminating).
type AppOperation struct {
	Phase      string  `json:"phase"`
	Message    *string `json:"message"`
	StartedAt  *string `json:"startedAt"`
	FinishedAt *string `json:"finishedAt"`
}

// QualityGate represents a quality gate step within a Stage.
type QualityGate struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// EnvSummary is one row in `krci env list`. The JSON envelope emits this
// slice under the wire-key "stages" (see EnvListPayload.Stages) to match the
// upstream Stage K8s resource; the Go-side name "EnvSummary" reflects the
// CLI's user-facing "environment" abstraction.
type EnvSummary struct {
	Deployment  string `json:"deployment"`
	Env         string `json:"env"`
	Cluster     string `json:"cluster"`
	Namespace   string `json:"namespace"`
	TriggerType string `json:"triggerType"`
	Status      string `json:"status"`
	Order       int    `json:"order"`
}

// EnvListPayload is the envelope `data` block for `krci env list`.
type EnvListPayload struct {
	Stages []EnvSummary `json:"stages"`
}

// EnvDetail is the response for `krci env get <deployment> <env>`.
// DetailedMessage is the Stage's status.detailed_message, null when the
// operator reported none.
type EnvDetail struct {
	Deployment      string              `json:"deployment"`
	Env             string              `json:"env"`
	Status          string              `json:"status"`
	DetailedMessage *string             `json:"detailedMessage"`
	Description     *string             `json:"description"`
	Order           int                 `json:"order"`
	Infrastructure  Infrastructure      `json:"infrastructure"`
	QualityGates    []QualityGateDetail `json:"qualityGates"`
	Projects        []EnvProject        `json:"projects"`
}

// Infrastructure carries the technical placement of an environment.
type Infrastructure struct {
	Cluster        string  `json:"cluster"`
	Namespace      string  `json:"namespace"`
	TriggerType    string  `json:"triggerType"`
	DeployPipeline string  `json:"deployPipeline"`
	CleanPipeline  *string `json:"cleanPipeline"`
}

// QualityGateDetail extends QualityGate with autotest + branch metadata
// surfaced in `krci env get`.
type QualityGateDetail struct {
	Type         string  `json:"type"`
	StepName     string  `json:"stepName"`
	AutotestName *string `json:"autotestName"`
	BranchName   *string `json:"branchName"`
}

// EnvProject is one row in EnvDetail.Projects. Conditions is always an array
// and Operation is null until the Application has been synced once.
type EnvProject struct {
	Name           string         `json:"name"`
	Status         *string        `json:"status"`
	Sync           *string        `json:"sync"`
	Version        *string        `json:"version"`
	ImageTag       *string        `json:"imageTag"`
	ImageDigest    *string        `json:"imageDigest"`
	IngressURLs    []string       `json:"ingressUrls"`
	ArgocdURL      *string        `json:"argocdUrl"`
	DeployedAt     *string        `json:"deployedAt"`
	ValuesOverride *bool          `json:"valuesOverride"`
	Conditions     []AppCondition `json:"conditions"`
	Operation      *AppOperation  `json:"operation"`
}

// ProjectDeploymentRow is one row in `krci project deployments <project>`.
// Conditions and Operation follow the EnvProject rules.
type ProjectDeploymentRow struct {
	Deployment  string         `json:"deployment"`
	Env         string         `json:"env"`
	Deployed    bool           `json:"deployed"`
	Status      *string        `json:"status"`
	Sync        *string        `json:"sync"`
	Version     *string        `json:"version"`
	ImageTag    *string        `json:"imageTag"`
	ImageDigest *string        `json:"imageDigest"`
	Cluster     string         `json:"cluster"`
	Namespace   string         `json:"namespace"`
	TriggerType string         `json:"triggerType"`
	DeployedAt  *string        `json:"deployedAt"`
	IngressURLs []string       `json:"ingressUrls"`
	ArgocdURL   *string        `json:"argocdUrl"`
	Conditions  []AppCondition `json:"conditions"`
	Operation   *AppOperation  `json:"operation"`
}

// ProjectDeploymentsPayload is the envelope `data` block for
// `krci project deployments <project>`.
type ProjectDeploymentsPayload struct {
	Project string                 `json:"project"`
	Rows    []ProjectDeploymentRow `json:"rows"`
}

// ImageVersion is one tag of a CodebaseImageStream: a version the build
// pipeline pushed for a branch, with its creation time and, when the
// registry reported one, the image digest.
type ImageVersion struct {
	Name    string `json:"name"`
	Created string `json:"created"`
	Digest  string `json:"digest,omitempty"`
}

// ProjectVersionStream is one CodebaseImageStream of a project: the image
// repository of one git branch and its versions, newest first. Versions is
// always an array, empty for a branch that has never been built.
type ProjectVersionStream struct {
	Branch   string         `json:"branch"`
	Image    string         `json:"image"`
	Versions []ImageVersion `json:"versions"`
}

// ProjectVersionsPayload is the envelope `data` block for
// `krci project versions <project>`.
type ProjectVersionsPayload struct {
	Project string                 `json:"project"`
	Streams []ProjectVersionStream `json:"streams"`
}
