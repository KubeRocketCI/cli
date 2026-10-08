package portal

import "github.com/KubeRocketCI/cli/internal/ptr"

// EnvNamespace names the namespace `krci env pods` and `krci env events`
// read: the environment and where its Stage places it.
type EnvNamespace struct {
	Deployment string `json:"deployment"`
	Env        string `json:"env"`
	Cluster    string `json:"cluster"`
	Namespace  string `json:"namespace"`
}

// EnvPodsPayload is the envelope `data` block for
// `krci env pods <deployment> <env>`: the pods of the environment namespace,
// sorted by name.
type EnvPodsPayload struct {
	EnvNamespace
	Pods []EnvPod `json:"pods"`
}

// EnvPod is one pod of an environment namespace. Status, ReadyContainers,
// TotalContainers and Restarts are the STATUS, READY and RESTARTS values
// `kubectl get pods` prints for the pod. Project is the project registered in
// the deployment whose name the pod carries in PodProjectLabel, null for any
// other pod. LastRestartAt is the time of the newest restart Restarts counts,
// null without one. Reason and Message are the pod's own status.reason and
// status.message (Evicted, NodeLost), null when the pod sets none.
type EnvPod struct {
	Name            string         `json:"name"`
	Project         *string        `json:"project"`
	Status          string         `json:"status"`
	Phase           string         `json:"phase"`
	ReadyContainers int            `json:"readyContainers"`
	TotalContainers int            `json:"totalContainers"`
	Restarts        int            `json:"restarts"`
	LastRestartAt   *string        `json:"lastRestartAt"`
	CreatedAt       *string        `json:"createdAt"`
	Node            *string        `json:"node"`
	Owner           *PodOwner      `json:"owner"`
	Reason          *string        `json:"reason"`
	Message         *string        `json:"message"`
	Conditions      []PodCondition `json:"conditions"`
	Containers      []PodContainer `json:"containers"`
}

// Pod phases of EnvPod.Phase that the status derivation and the table read.
const (
	PodPhaseRunning   = "Running"
	PodPhaseSucceeded = "Succeeded"
	PodPhaseFailed    = "Failed"
)

// Healthy reports whether the pod has completed, or runs with every container
// ready and a Status equal to its Phase.
func (p EnvPod) Healthy() bool {
	return p.Phase == PodPhaseSucceeded ||
		(p.Phase == PodPhaseRunning && p.Status == p.Phase && p.ReadyContainers == p.TotalContainers)
}

// PodProjectLabel is the pod label the Portal reads to find the pods of one
// application. The platform installs a project as a Helm release named after
// it, and the chart it scaffolds for a project sets the label to the release
// name.
const PodProjectLabel = "app.kubernetes.io/instance"

// PodOwner is the owner of a pod: its controller (a ReplicaSet for a
// Deployment, a StatefulSet, a DaemonSet, a Job), or its first owner when
// none is marked as the controller.
type PodOwner struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PodCondition is one pod condition that holds the pod back: a condition that
// is not True, or a DisruptionTarget that is.
type PodCondition struct {
	Type               string  `json:"type"`
	Status             string  `json:"status"`
	Reason             *string `json:"reason"`
	Message            *string `json:"message"`
	LastTransitionTime *string `json:"lastTransitionTime"`
}

// Container states of PodContainer.State.
const (
	ContainerWaiting    = "waiting"
	ContainerRunning    = "running"
	ContainerTerminated = "terminated"
)

// PodContainer is one container the kubelet reported for a pod, init
// containers first. Image is the image the pod spec asks for. ImageID is the
// status imageID the runtime reports, null until the image is present.
// ImageDigest is the registry digest in ImageID, null when ImageID names none:
// a bare "sha256:<hex>" is the image's local ID, not its registry digest.
// Reason and Message belong to a waiting or terminated State, ExitCode and
// FinishedAt to a terminated one, StartedAt to a running or terminated one.
// LastTermination is the previous run of a restarted container. Sidecar marks
// an init container that restarts always: it runs beside the containers and
// READY counts it.
type PodContainer struct {
	Name            string                `json:"name"`
	Init            bool                  `json:"init"`
	Sidecar         bool                  `json:"sidecar"`
	Image           string                `json:"image"`
	ImageID         *string               `json:"imageID"`
	ImageDigest     *string               `json:"imageDigest"`
	Ready           bool                  `json:"ready"`
	Restarts        int                   `json:"restarts"`
	State           string                `json:"state"`
	Reason          *string               `json:"reason"`
	Message         *string               `json:"message"`
	ExitCode        *int                  `json:"exitCode"`
	StartedAt       *string               `json:"startedAt"`
	FinishedAt      *string               `json:"finishedAt"`
	LastTermination *ContainerTermination `json:"lastTermination"`
}

// NeedsAttention reports whether the container has been restarted, or is
// neither running ready nor terminated with exit code 0.
func (c PodContainer) NeedsAttention() bool {
	fine := (c.State == ContainerRunning && c.Ready) ||
		(c.State == ContainerTerminated && ptr.Deref(c.ExitCode, 0) == 0)

	return !fine || c.LastTermination != nil
}

// FailsReadiness reports whether a running container or sidecar is not ready.
// A regular init container is not ready while it runs.
func (c PodContainer) FailsReadiness() bool {
	return c.State == ContainerRunning && !c.Ready && (!c.Init || c.Sidecar)
}

// ContainerTermination is how the previous run of a container ended.
type ContainerTermination struct {
	Reason     *string `json:"reason"`
	Message    *string `json:"message"`
	ExitCode   int     `json:"exitCode"`
	StartedAt  *string `json:"startedAt"`
	FinishedAt *string `json:"finishedAt"`
}

// EnvEventsPayload is the envelope `data` block for
// `krci env events <deployment> <env>`: the Kubernetes events of the
// environment namespace, newest first.
type EnvEventsPayload struct {
	EnvNamespace
	Events []EnvEvent `json:"events"`
}

// EventTypeWarning is the Kubernetes event type `env events --warnings` keeps.
const EventTypeWarning = "Warning"

// EnvEvent is one Kubernetes event. Count, FirstSeen and LastSeen are the
// values `kubectl get events` derives: an event recorded as a series reports
// the series count and its last observed time.
type EnvEvent struct {
	Type           string      `json:"type"`
	Reason         string      `json:"reason"`
	Message        string      `json:"message"`
	InvolvedObject EventObject `json:"involvedObject"`
	Count          int         `json:"count"`
	FirstSeen      *string     `json:"firstSeen"`
	LastSeen       *string     `json:"lastSeen"`
	Source         *string     `json:"source"`
}

// EventObject is the object an event is about.
type EventObject struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}
