package portal

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/KubeRocketCI/cli/internal/ptr"
)

// k8sPod and the k8s* types below decode the part of a core/v1 Pod the env
// verbs read. Do not add container env, command or args: they can carry
// secrets in plain text.
type k8sPod struct {
	Metadata k8sPodMetadata `json:"metadata"`
	Spec     k8sPodSpec     `json:"spec"`
	Status   k8sPodStatus   `json:"status"`
}

type k8sPodMetadata struct {
	Name              string              `json:"name"`
	CreationTimestamp string              `json:"creationTimestamp"`
	DeletionTimestamp *string             `json:"deletionTimestamp"`
	Labels            map[string]string   `json:"labels"`
	OwnerReferences   []k8sOwnerReference `json:"ownerReferences"`
}

type k8sOwnerReference struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Controller bool   `json:"controller"`
}

type k8sPodSpec struct {
	NodeName       string             `json:"nodeName"`
	Containers     []k8sContainerSpec `json:"containers"`
	InitContainers []k8sContainerSpec `json:"initContainers"`
}

type k8sContainerSpec struct {
	Name          string `json:"name"`
	Image         string `json:"image"`
	RestartPolicy string `json:"restartPolicy"`
}

// sidecar reports whether an init container restarts always, which makes it a
// sidecar.
func (c k8sContainerSpec) sidecar() bool {
	return c.RestartPolicy == containerRestartAlways
}

type k8sPodStatus struct {
	Phase                 string               `json:"phase"`
	Reason                string               `json:"reason"`
	Message               string               `json:"message"`
	Conditions            []k8sPodCondition    `json:"conditions"`
	InitContainerStatuses []k8sContainerStatus `json:"initContainerStatuses"`
	ContainerStatuses     []k8sContainerStatus `json:"containerStatuses"`
}

type k8sPodCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
	LastTransitionTime string `json:"lastTransitionTime"`
}

type k8sContainerStatus struct {
	Name         string            `json:"name"`
	ImageID      string            `json:"imageID"`
	Ready        bool              `json:"ready"`
	Started      *bool             `json:"started"`
	RestartCount int               `json:"restartCount"`
	State        k8sContainerState `json:"state"`
	LastState    k8sContainerState `json:"lastState"`
}

type k8sContainerState struct {
	Waiting    *k8sContainerWaiting    `json:"waiting"`
	Running    *k8sContainerRunning    `json:"running"`
	Terminated *k8sContainerTerminated `json:"terminated"`
}

type k8sContainerWaiting struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type k8sContainerRunning struct {
	StartedAt string `json:"startedAt"`
}

type k8sContainerTerminated struct {
	Reason     string `json:"reason"`
	Message    string `json:"message"`
	ExitCode   int    `json:"exitCode"`
	Signal     int    `json:"signal"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
}

// Pod values the status derivation matches, spelled as printPod matches them.
const (
	podReasonNodeLost        = "NodeLost"
	podReasonSchedulingGated = "SchedulingGated"
	podReasonInitializing    = "PodInitializing"
	podReasonCompleted       = "Completed"
	podStatusNotReady        = "NotReady"
	podStatusTerminating     = "Terminating"
	podStatusUnknown         = "Unknown"
	podConditionScheduled    = "PodScheduled"
	podConditionInitialized  = "Initialized"
	podConditionReady        = "Ready"
	podConditionDisruption   = "DisruptionTarget"
	containerRestartAlways   = "Always"
)

// Pods returns the pods in the namespace of one environment. A missing
// deployment or environment is answered like Get, an environment on another
// cluster with ErrRemoteCluster.
func (s *EnvService) Pods(ctx context.Context, deployment, env string) (*EnvPodsPayload, error) {
	ns, projects, err := s.namespaceOf(ctx, deployment, env)
	if err != nil {
		return nil, err
	}

	items, err := listNamespace[k8sPod](ctx, s, ns.Namespace, podResourceConfig)
	if err != nil {
		return nil, err
	}

	return &EnvPodsPayload{EnvNamespace: ns, Pods: mapPods(items, projects)}, nil
}

// mapPods projects the pods of a namespace into rows sorted by name. projects
// are the names the deployment registers.
func mapPods(items []k8sPod, projects []string) []EnvPod {
	pods := make([]EnvPod, 0, len(items))

	for _, p := range items {
		pods = append(pods, mapPod(p, projects))
	}

	sort.SliceStable(pods, func(i, j int) bool {
		return pods[i].Name < pods[j].Name
	})

	return pods
}

func mapPod(p k8sPod, projects []string) EnvPod {
	sum := summarizePod(p)

	pod := EnvPod{
		Name:            p.Metadata.Name,
		Status:          sum.status,
		Phase:           p.Status.Phase,
		ReadyContainers: sum.ready,
		TotalContainers: sum.total,
		Restarts:        sum.restarts.count,
		LastRestartAt:   sum.restarts.lastAt(),
		CreatedAt:       nonEmpty(p.Metadata.CreationTimestamp),
		Node:            nonEmpty(p.Spec.NodeName),
		Owner:           podOwner(p.Metadata.OwnerReferences),
		Reason:          nonEmpty(p.Status.Reason),
		Message:         nonEmpty(p.Status.Message),
		Conditions:      mapPodConditions(p.Status.Conditions),
		Containers:      mapPodContainers(p.Spec, p.Status),
	}

	if name := p.Metadata.Labels[PodProjectLabel]; slices.Contains(projects, name) {
		pod.Project = &name
	}

	return pod
}

// podOwner returns the controller among the owner references, the first owner
// when none is marked as the controller, or nil for a pod without owners.
func podOwner(refs []k8sOwnerReference) *PodOwner {
	if len(refs) == 0 {
		return nil
	}

	owner := refs[0]

	for _, r := range refs {
		if r.Controller {
			owner = r
			break
		}
	}

	return &PodOwner{Kind: owner.Kind, Name: owner.Name}
}

// mapPodConditions keeps the conditions that hold a pod back: one that is not
// True, or a DisruptionTarget that is. The result is never nil.
func mapPodConditions(conditions []k8sPodCondition) []PodCondition {
	out := make([]PodCondition, 0, len(conditions))

	for _, c := range conditions {
		holdsBack := c.Status != conditionStatusTrue
		if c.Type == podConditionDisruption {
			holdsBack = c.Status == conditionStatusTrue
		}

		if !holdsBack {
			continue
		}

		out = append(out, PodCondition{
			Type:               c.Type,
			Status:             c.Status,
			Reason:             nonEmpty(c.Reason),
			Message:            nonEmpty(c.Message),
			LastTransitionTime: nonEmpty(c.LastTransitionTime),
		})
	}

	return out
}

// mapPodContainers lists the containers the kubelet reported, init containers
// first. The result is never nil.
func mapPodContainers(spec k8sPodSpec, status k8sPodStatus) []PodContainer {
	out := make([]PodContainer, 0, len(status.InitContainerStatuses)+len(status.ContainerStatuses))

	for _, c := range status.InitContainerStatuses {
		out = append(out, mapPodContainer(c, specOf(spec.InitContainers, c.Name), true))
	}

	for _, c := range status.ContainerStatuses {
		out = append(out, mapPodContainer(c, specOf(spec.Containers, c.Name), false))
	}

	return out
}

// specOf returns the spec of the named container, the zero spec when the pod
// spec lists none.
func specOf(containers []k8sContainerSpec, name string) k8sContainerSpec {
	for _, c := range containers {
		if c.Name == name {
			return c
		}
	}

	return k8sContainerSpec{}
}

// pulledDigest returns the registry digest after the "@" of a container status
// imageID ("<repository>@sha256:<hex>", "docker-pullable://<repository>@…").
// It is nil for an empty imageID and for one without "@": a bare or
// runtime-prefixed "sha256:<hex>" is the image's local ID.
func pulledDigest(imageID string) *string {
	_, digest, _ := strings.Cut(imageID, "@")

	return nonEmpty(digest)
}

// mapPodContainer projects one container status with its spec. Image is the
// image the spec asks for: the image of a container status is the one the
// runtime resolved and may differ (ContainerStatus.image in core/v1). A
// container without a reported state is waiting, the Kubernetes default.
func mapPodContainer(c k8sContainerStatus, spec k8sContainerSpec, init bool) PodContainer {
	out := PodContainer{
		Name:        c.Name,
		Init:        init,
		Sidecar:     init && spec.sidecar(),
		Image:       spec.Image,
		ImageID:     nonEmpty(c.ImageID),
		ImageDigest: pulledDigest(c.ImageID),
		Ready:       c.Ready,
		Restarts:    c.RestartCount,
		State:       ContainerWaiting,
	}

	switch {
	case c.State.Running != nil:
		out.State = ContainerRunning
		out.StartedAt = nonEmpty(c.State.Running.StartedAt)
	case c.State.Terminated != nil:
		t := c.State.Terminated
		out.State = ContainerTerminated
		out.Reason = nonEmpty(t.Reason)
		out.Message = nonEmpty(t.Message)
		out.ExitCode = ptr.To(t.ExitCode)
		out.StartedAt = nonEmpty(t.StartedAt)
		out.FinishedAt = nonEmpty(t.FinishedAt)
	case c.State.Waiting != nil:
		out.Reason = nonEmpty(c.State.Waiting.Reason)
		out.Message = nonEmpty(c.State.Waiting.Message)
	}

	if t := c.LastState.Terminated; t != nil {
		out.LastTermination = &ContainerTermination{
			Reason:     nonEmpty(t.Reason),
			Message:    nonEmpty(t.Message),
			ExitCode:   t.ExitCode,
			StartedAt:  nonEmpty(t.StartedAt),
			FinishedAt: nonEmpty(t.FinishedAt),
		}
	}

	return out
}

// podSummary holds the STATUS, READY and RESTARTS cells of one pod.
type podSummary struct {
	status   string
	ready    int
	total    int
	restarts restartTally
}

// restartTally counts restarts and keeps the newest time a counted container
// last terminated: the restarts and lastRestartDate of printPod.
type restartTally struct {
	count int
	last  time.Time
}

func (r *restartTally) add(c k8sContainerStatus) {
	r.count += c.RestartCount

	if t := c.LastState.Terminated; t != nil {
		if at := parseTime(t.FinishedAt); at.After(r.last) {
			r.last = at
		}
	}
}

// lastAt is the time of the newest counted restart in RFC 3339, nil without
// one.
func (r restartTally) lastAt() *string {
	if r.count == 0 || r.last.IsZero() {
		return nil
	}

	return ptr.To(r.last.UTC().Format(time.RFC3339))
}

// summarizePod derives the STATUS, READY and RESTARTS cells `kubectl get pods`
// prints for a pod, with the time of the newest counted restart. It follows
// printPod of Kubernetes v1.36 (pkg/printers/internalversion/printers.go in
// k8s.io/kubernetes), the printer behind that table: the phase, or the pod
// reason, then the init containers, then the containers, then the deletion
// mark.
func summarizePod(p k8sPod) podSummary {
	sum := podSummary{status: p.Status.Phase, total: len(p.Spec.Containers)}
	if p.Status.Reason != "" {
		sum.status = p.Status.Reason
	}

	for _, c := range p.Status.Conditions {
		if c.Type == podConditionScheduled && c.Reason == podReasonSchedulingGated {
			sum.status = podReasonSchedulingGated
		}
	}

	initializing, sidecarRestarts := summarizeInitContainers(p, &sum)

	if !initializing || podConditionIsTrue(p.Status.Conditions, podConditionInitialized) {
		sum.restarts = sidecarRestarts
		summarizeContainers(p.Status, &sum)
	}

	deleted := p.Metadata.DeletionTimestamp != nil

	switch {
	case deleted && p.Status.Reason == podReasonNodeLost:
		sum.status = podStatusUnknown
	case deleted && p.Status.Phase != PodPhaseSucceeded && p.Status.Phase != PodPhaseFailed:
		sum.status = podStatusTerminating
	}

	return sum
}

// summarizeInitContainers is the init pass of summarizePod. It adds every
// sidecar to the containers, counts the restarts of every init container up to
// the first one that has not completed, which decides the status, and counts a
// started sidecar as ready when it is. It reports whether the pod is
// initializing and the restarts of the sidecars.
func summarizeInitContainers(p k8sPod, sum *podSummary) (initializing bool, sidecarRestarts restartTally) {
	sidecars := map[string]bool{}

	for _, c := range p.Spec.InitContainers {
		if c.sidecar() {
			sidecars[c.Name] = true
			sum.total++
		}
	}

	for i, c := range p.Status.InitContainerStatuses {
		sum.restarts.add(c)

		if sidecars[c.Name] {
			sidecarRestarts.add(c)
		}

		switch {
		case c.State.Terminated != nil && c.State.Terminated.ExitCode == 0:
			continue
		case sidecars[c.Name] && c.Started != nil && *c.Started:
			if c.Ready {
				sum.ready++
			}

			continue
		case c.State.Terminated != nil:
			sum.status = "Init:" + terminationStatus(*c.State.Terminated)
		case c.State.Waiting != nil && c.State.Waiting.Reason != "" && c.State.Waiting.Reason != podReasonInitializing:
			sum.status = "Init:" + c.State.Waiting.Reason
		default:
			sum.status = fmt.Sprintf("Init:%d/%d", i, len(p.Spec.InitContainers))
		}

		return true, sidecarRestarts
	}

	return false, sidecarRestarts
}

// summarizeContainers is the container pass of summarizePod. It walks the
// containers from the last to the first, so when several are waiting or
// terminated the one nearest the start of the list decides the status. A
// Completed status gives way to a container that runs, else to one that
// exited with a non-zero code.
func summarizeContainers(status k8sPodStatus, sum *podSummary) {
	hasRunning, errorStatus := false, ""

	for i := len(status.ContainerStatuses) - 1; i >= 0; i-- {
		c := status.ContainerStatuses[i]
		sum.restarts.add(c)

		switch {
		case c.State.Waiting != nil && c.State.Waiting.Reason != "":
			sum.status = c.State.Waiting.Reason
		case c.State.Terminated != nil:
			sum.status = terminationStatus(*c.State.Terminated)
			if c.State.Terminated.ExitCode != 0 {
				errorStatus = sum.status
			}
		case c.Ready && c.State.Running != nil:
			hasRunning = true
			sum.ready++
		}
	}

	if sum.status != podReasonCompleted {
		return
	}

	switch {
	case hasRunning && podConditionIsTrue(status.Conditions, podConditionReady):
		sum.status = PodPhaseRunning
	case errorStatus != "":
		sum.status = errorStatus
	case hasRunning:
		sum.status = podStatusNotReady
	}
}

// terminationStatus names a terminated container: its reason, or the signal
// or exit code when the runtime reported no reason.
func terminationStatus(t k8sContainerTerminated) string {
	switch {
	case t.Reason != "":
		return t.Reason
	case t.Signal != 0:
		return fmt.Sprintf("Signal:%d", t.Signal)
	default:
		return fmt.Sprintf("ExitCode:%d", t.ExitCode)
	}
}

func podConditionIsTrue(conditions []k8sPodCondition, conditionType string) bool {
	for _, c := range conditions {
		if c.Type == conditionType {
			return c.Status == conditionStatusTrue
		}
	}

	return false
}
