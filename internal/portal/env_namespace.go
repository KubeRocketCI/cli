package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

// stageInCluster is the Stage.spec.clusterName of an environment on the
// cluster the platform runs on; the Stage CRD defaults the field to it.
const stageInCluster = "in-cluster"

func remoteClusterError(deployment, env, cluster string) error {
	return newRichErr(fmt.Sprintf(
		"environment %q of deployment %q runs on cluster %q; the Portal reads pods and events only on its own cluster",
		env, deployment, cluster), ErrRemoteCluster)
}

func noNamespaceError(deployment, env string) error {
	return fmt.Errorf("environment %q of deployment %q has no namespace", env, deployment)
}

// namespaceOf resolves the namespace of one environment and the projects its
// deployment registers. The Portal reads only the cluster it runs on, which a
// Stage names stageInCluster or leaves empty; any other cluster is
// ErrRemoteCluster. A Stage without a namespace is refused: the Portal answers
// a list without a namespace with the objects of every namespace.
func (s *EnvService) namespaceOf(ctx context.Context, deployment, env string) (EnvNamespace, []string, error) {
	target, found, err := s.resolveTarget(ctx, deployment, env)
	if err != nil {
		return EnvNamespace{}, nil, err
	}

	if !found {
		return EnvNamespace{}, nil, ErrEnvNotFound
	}

	spec := ptr.Deref(target.stage.Spec, nil)
	ns := EnvNamespace{
		Deployment: deployment,
		Env:        env,
		Cluster:    stringVal(spec, "clusterName"),
		Namespace:  stringVal(spec, "namespace"),
	}

	switch {
	case ns.Cluster != "" && ns.Cluster != stageInCluster:
		return EnvNamespace{}, nil, remoteClusterError(deployment, env, ns.Cluster)
	case ns.Namespace == "":
		return EnvNamespace{}, nil, noNamespaceError(deployment, env)
	}

	return ns, target.projects, nil
}

// listNamespace lists one kind in a namespace and decodes the items from the
// response body into T. The generated K8sList item drops every key outside
// its basic metadata, spec and status: the ownerReferences and
// deletionTimestamp of a Pod, every field of an Event.
func listNamespace[T any](
	ctx context.Context, s *EnvService, namespace string, rc restapi.K8sListJSONBody,
) ([]T, error) {
	resp, err := s.client.K8sListWithResponse(ctx, buildK8sListBody(s.clusterName, namespace, rc, nil))
	if err != nil {
		return nil, namespaceReadError(rc, namespace, err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, namespaceReadError(rc, namespace, err)
	}

	var list struct {
		Items []T `json:"items"`
	}

	if err := json.Unmarshal(resp.Body, &list); err != nil {
		return nil, namespaceReadError(rc, namespace, fmt.Errorf("decoding the response: %w", err))
	}

	return list.Items, nil
}

// parseTime parses an RFC 3339 timestamp, with or without a fraction of a
// second. An empty or malformed one is the zero time, so it sorts as the
// oldest.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}

	return t
}

// namespaceReadError names the kind and the namespace of a failed read.
func namespaceReadError(rc restapi.K8sListJSONBody, namespace string, err error) error {
	return fmt.Errorf("listing %s in namespace %q: %w", rc.ResourceConfig.PluralName, namespace, err)
}
