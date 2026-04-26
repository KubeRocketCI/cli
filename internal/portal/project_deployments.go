package portal

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

// DeploymentByProjectService backs `krci project deployments <project>`. It
// fans out three concurrent k8s.list calls (Applications scoped to the
// project, Stages, CDPipelines) then joins them client-side.
type DeploymentByProjectService struct {
	client      *restapi.ClientWithResponses
	clusterName string
	namespace   string
}

// NewDeploymentByProjectService creates a service for the given cluster and
// namespace.
func NewDeploymentByProjectService(
	client *restapi.ClientWithResponses,
	clusterName, namespace string,
) *DeploymentByProjectService {
	return &DeploymentByProjectService{client: client, clusterName: clusterName, namespace: namespace}
}

func (s *DeploymentByProjectService) listBody(
	rc restapi.K8sListJSONBody,
	labels map[string]string,
) restapi.K8sListJSONRequestBody {
	return buildK8sListBody(s.clusterName, s.namespace, rc, labels)
}

// List returns one row per (deployment, env) pair where project is registered
// in the matching CDPipeline's spec.applications. Rows for stages without a
// matching Application carry deployed=false plus null dynamic fields (still
// surfacing static cluster/namespace/triggerType from the Stage so the user
// sees the full footprint). Stages and CDPipelines are fetched unscoped because
// the project-membership filter lives in CDPipeline.spec.applications, which
// has no equivalent label on Stage; the join is performed client-side.
func (s *DeploymentByProjectService) List(ctx context.Context, project string) ([]ProjectDeploymentRow, error) {
	var (
		appResp      *restapi.K8sListResponse
		stageResp    *restapi.K8sListResponse
		pipelineResp *restapi.K8sListResponse
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		body := s.listBody(applicationResourceConfig, map[string]string{labelAppName: project})

		var err error
		appResp, err = s.client.K8sListWithResponse(gctx, body)
		if err != nil {
			return fmt.Errorf("listing applications for project %q: %w", project, err)
		}

		return checkResponse(appResp.StatusCode(), appResp.Body)
	})

	g.Go(func() error {
		var err error
		stageResp, err = s.client.K8sListWithResponse(gctx, s.listBody(stageResourceConfig, nil))
		if err != nil {
			return fmt.Errorf("listing stages: %w", err)
		}

		return checkResponse(stageResp.StatusCode(), stageResp.Body)
	})

	g.Go(func() error {
		var err error
		pipelineResp, err = s.client.K8sListWithResponse(gctx, s.listBody(cdPipelineResourceConfig, nil))
		if err != nil {
			return fmt.Errorf("listing cdpipelines: %w", err)
		}

		return checkResponse(pipelineResp.StatusCode(), pipelineResp.Body)
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	var (
		appItems      []k8sItem
		stageItems    []k8sItem
		pipelineItems []k8sItem
	)

	if appResp.JSON200 != nil {
		appItems = appResp.JSON200.Items
	}

	if stageResp.JSON200 != nil {
		stageItems = stageResp.JSON200.Items
	}

	if pipelineResp.JSON200 != nil {
		pipelineItems = pipelineResp.JSON200.Items
	}

	rows := buildProjectDeploymentRows(project, appItems, stageItems, pipelineItems)

	return rows, nil
}

// buildProjectDeploymentRows enumerates expected (deployment, env) pairs from
// each CDPipeline whose spec.applications contains project, joins each pair
// to its Stage (for cluster/namespace/triggerType/order) and Application
// (for health/sync/version), and emits rows sorted by deployment asc then
// Stage.spec.order asc.
func buildProjectDeploymentRows(
	project string,
	apps, stages, pipelines []k8sItem,
) []ProjectDeploymentRow {
	stagesByDeployAndEnv := indexStagesByDeployAndEnv(stages)
	appsByDeployAndEnv := indexAppsByDeployAndEnv(apps)

	type sortable struct {
		row   ProjectDeploymentRow
		order int
	}

	indexed := make([]sortable, 0, len(stages))

	for _, pipelineItem := range pipelines {
		spec := ptr.Deref(pipelineItem.Spec, nil)

		registered := stringSliceVal(spec, "applications")
		if !slices.Contains(registered, project) {
			continue
		}

		deploymentName := pipelineItem.Metadata.Name

		for _, stageItem := range stagesByDeployAndEnv[deploymentName] {
			stageSpec := ptr.Deref(stageItem.Spec, nil)
			indexed = append(indexed, sortable{
				row:   buildProjectDeploymentRow(deploymentName, stageSpec, appsByDeployAndEnv),
				order: stageOrder(stageSpec),
			})
		}
	}

	sort.SliceStable(indexed, func(i, j int) bool {
		if indexed[i].row.Deployment != indexed[j].row.Deployment {
			return indexed[i].row.Deployment < indexed[j].row.Deployment
		}

		return indexed[i].order < indexed[j].order
	})

	rows := make([]ProjectDeploymentRow, len(indexed))
	for i, s := range indexed {
		rows[i] = s.row
	}

	return rows
}

// deployEnvKey is the composite (deployment, env) lookup key used by the
// stage-order map and the Application index.
type deployEnvKey struct {
	Deployment string
	Env        string
}

// buildProjectDeploymentRow shapes one row from a Stage spec + the
// Application index keyed by (deployment, env).
func buildProjectDeploymentRow(
	deployment string,
	spec map[string]any,
	appsByDeployAndEnv map[deployEnvKey]k8sItem,
) ProjectDeploymentRow {
	row := ProjectDeploymentRow{
		Deployment:  deployment,
		Env:         stringVal(spec, "name"),
		Cluster:     stringVal(spec, "clusterName"),
		Namespace:   stringVal(spec, "namespace"),
		TriggerType: stringVal(spec, "triggerType"),
		IngressURLs: []string{},
	}

	if appItem, ok := appsByDeployAndEnv[deployEnvKey{deployment, row.Env}]; ok {
		row.Deployed = true
		applyAppFieldsToRow(&row, appItem)
	}

	return row
}

// applyAppFieldsToRow sets the dynamic Application-derived fields on a
// ProjectDeploymentRow from one Application item.
func applyAppFieldsToRow(row *ProjectDeploymentRow, item k8sItem) {
	f := extractAppFields(item)

	row.Status = f.Status
	row.Sync = f.Sync
	row.Version = f.Version
	row.ImageTag = f.ImageTag
	row.ImageDigest = f.ImageDigest
	row.IngressURLs = f.IngressURLs
	row.ArgocdURL = f.ArgocdURL
	row.DeployedAt = f.DeployedAt
}

// indexStagesByDeployAndEnv groups Stages by parent CDPipeline
// (spec.cdPipeline). Bucket order is irrelevant; the final row sort in
// buildProjectDeploymentRows is the authoritative ordering.
func indexStagesByDeployAndEnv(items []k8sItem) map[string][]k8sItem {
	out := make(map[string][]k8sItem)

	for _, item := range items {
		spec := ptr.Deref(item.Spec, nil)
		parent := stringVal(spec, "cdPipeline")
		if parent == "" {
			continue
		}

		out[parent] = append(out[parent], item)
	}

	return out
}

// indexAppsByDeployAndEnv indexes Applications by the (pipeline, stage) label
// pair. The `app.edp.epam.com/stage` label carries Stage.spec.name (the short
// env identifier "dev"/"stage"/"prod").
func indexAppsByDeployAndEnv(items []k8sItem) map[deployEnvKey]k8sItem {
	out := make(map[deployEnvKey]k8sItem, len(items))

	for _, item := range items {
		labels := ptr.Deref(item.Metadata.Labels, nil)
		if labels == nil {
			continue
		}

		pipeline := labels[labelPipeline]
		stage := labels[labelStage]
		if pipeline == "" || stage == "" {
			continue
		}

		out[deployEnvKey{pipeline, stage}] = item
	}

	return out
}
