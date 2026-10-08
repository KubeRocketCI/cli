package portal

import (
	"context"
	"sort"
	"strings"

	"github.com/KubeRocketCI/cli/internal/ptr"
)

// EnvEventFilters narrows an `env events` query.
type EnvEventFilters struct {
	// Pod keeps the events about the pod of this name, or "" for every object.
	Pod string
	// WarningsOnly keeps the events of type EventTypeWarning.
	WarningsOnly bool
}

// k8sEvent is the part of a core/v1 Event the env verbs read.
type k8sEvent struct {
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	InvolvedObject struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"involvedObject"`
	Count          int    `json:"count"`
	FirstTimestamp string `json:"firstTimestamp"`
	LastTimestamp  string `json:"lastTimestamp"`
	EventTime      string `json:"eventTime"`
	Series         *struct {
		Count            int    `json:"count"`
		LastObservedTime string `json:"lastObservedTime"`
	} `json:"series"`
	Source struct {
		Component string `json:"component"`
	} `json:"source"`
	ReportingComponent string `json:"reportingComponent"`
}

// Events returns the Kubernetes events in the namespace of one environment
// that pass the filters, newest first. A missing deployment or environment is
// answered like Get, an environment on another cluster with ErrRemoteCluster.
func (s *EnvService) Events(
	ctx context.Context, deployment, env string, f EnvEventFilters,
) (*EnvEventsPayload, error) {
	ns, _, err := s.namespaceOf(ctx, deployment, env)
	if err != nil {
		return nil, err
	}

	items, err := listNamespace[k8sEvent](ctx, s, ns.Namespace, eventResourceConfig)
	if err != nil {
		return nil, err
	}

	return &EnvEventsPayload{EnvNamespace: ns, Events: mapEvents(items, f)}, nil
}

// mapEvents projects the events that pass the filters and orders them by the
// time last seen, newest first. The result is never nil.
func mapEvents(items []k8sEvent, f EnvEventFilters) []EnvEvent {
	podKind := podResourceConfig.ResourceConfig.Kind
	events := make([]EnvEvent, 0, len(items))

	for _, e := range items {
		if f.WarningsOnly && e.Type != EventTypeWarning {
			continue
		}

		if f.Pod != "" && (e.InvolvedObject.Kind != podKind || e.InvolvedObject.Name != f.Pod) {
			continue
		}

		events = append(events, mapEvent(e))
	}

	sort.SliceStable(events, func(i, j int) bool {
		return parseTime(ptr.Deref(events[i].LastSeen, "")).After(parseTime(ptr.Deref(events[j].LastSeen, "")))
	})

	return events
}

// mapEvent derives the count and the first and last time seen as
// `kubectl get events` does (printEvent in
// pkg/printers/internalversion/printers.go of k8s.io/kubernetes): the legacy
// timestamps, else the event time; a series overrides the count and the last
// time; an event without a count happened once.
func mapEvent(e k8sEvent) EnvEvent {
	first := e.FirstTimestamp
	if first == "" {
		first = e.EventTime
	}

	last := e.LastTimestamp
	if last == "" {
		last = first
	}

	count := e.Count

	switch {
	case e.Series != nil:
		last = e.Series.LastObservedTime
		count = e.Series.Count
	case count == 0:
		count = 1
	}

	source := e.Source.Component
	if source == "" {
		source = e.ReportingComponent
	}

	return EnvEvent{
		Type:           e.Type,
		Reason:         e.Reason,
		Message:        strings.TrimSpace(e.Message),
		InvolvedObject: EventObject{Kind: e.InvolvedObject.Kind, Name: e.InvolvedObject.Name},
		Count:          count,
		FirstSeen:      nonEmpty(first),
		LastSeen:       nonEmpty(last),
		Source:         nonEmpty(source),
	}
}
