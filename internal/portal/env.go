package portal

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

// EnvListFilters bounds an `env list` query.
type EnvListFilters struct {
	// Deployment is the parent CDPipeline name to filter by, or "" for all.
	Deployment string
	// Cluster is the post-mapping cluster filter, or "" for all.
	Cluster string
}

// EnvService surfaces KRCI Stages as user-facing "environments".
type EnvService struct {
	client      *restapi.ClientWithResponses
	clusterName string
	namespace   string
}

// NewEnvService creates an EnvService for the given cluster and namespace.
func NewEnvService(client *restapi.ClientWithResponses, clusterName, namespace string) *EnvService {
	return &EnvService{client: client, clusterName: clusterName, namespace: namespace}
}

func (s *EnvService) listBody(rc restapi.K8sListJSONBody, labels map[string]string) restapi.K8sListJSONRequestBody {
	return buildK8sListBody(s.clusterName, s.namespace, rc, labels)
}

// List returns every KRCI Stage in the configured namespace, optionally
// filtered by parent deployment and/or cluster. The returned slice is always
// non-nil so callers can JSON-marshal it as `[]` rather than `null`.
func (s *EnvService) List(ctx context.Context, f EnvListFilters) ([]EnvSummary, error) {
	var labels map[string]string
	if f.Deployment != "" {
		labels = map[string]string{labelCDPipelineName: f.Deployment}
	}

	resp, err := s.client.K8sListWithResponse(ctx, s.listBody(stageResourceConfig, labels))
	if err != nil {
		return nil, fmt.Errorf("listing stages: %w", err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		return nil, fmt.Errorf("listing stages: %w", err)
	}

	if resp.JSON200 == nil {
		return []EnvSummary{}, nil
	}

	rows := make([]EnvSummary, 0, len(resp.JSON200.Items))
	for _, item := range resp.JSON200.Items {
		row := mapEnvSummary(item)

		if f.Cluster != "" && row.Cluster != f.Cluster {
			continue
		}

		rows = append(rows, row)
	}

	sortEnvSummaries(rows)

	return rows, nil
}

// Get returns the full detail of one stage. Fans out three k8s calls in
// parallel: list Stages (to resolve env-name → metadata.name), get parent
// CDPipeline (to enumerate registered apps), list Applications scoped to the
// (pipeline, stage) pair. The Application label `app.edp.epam.com/stage`
// carries `stage.spec.name` (the short identifier "dev"/"stage"/"prod"), not
// the compound resource name.
func (s *EnvService) Get(ctx context.Context, deployment, env string) (*EnvDetail, error) {
	var (
		stageResp    *restapi.K8sListResponse
		pipelineResp *restapi.K8sGetResponse
		appResp      *restapi.K8sListResponse
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		stageResp, err = s.client.K8sListWithResponse(gctx, s.listBody(stageResourceConfig, map[string]string{
			labelCDPipelineName: deployment,
		}))
		if err != nil {
			return fmt.Errorf("listing stages for deployment %q: %w", deployment, err)
		}

		return checkResponse(stageResp.StatusCode(), stageResp.Body)
	})

	g.Go(func() error {
		ns := s.namespace
		body := restapi.K8sGetJSONRequestBody{
			ClusterName:    s.clusterName,
			Namespace:      &ns,
			Name:           deployment,
			ResourceConfig: cdPipelineResourceConfig.ResourceConfig,
		}

		var err error
		pipelineResp, err = s.client.K8sGetWithResponse(gctx, body)
		if err != nil {
			return fmt.Errorf("getting deployment %q: %w", deployment, err)
		}

		if err := checkResponse(pipelineResp.StatusCode(), pipelineResp.Body); err != nil {
			if errors.Is(err, ErrNotFound) {
				return ErrDeploymentNotFound
			}
			return err
		}

		return nil
	})

	g.Go(func() error {
		var err error
		appResp, err = s.client.K8sListWithResponse(gctx, s.listBody(applicationResourceConfig, map[string]string{
			labelPipeline: deployment,
			labelStage:    env,
		}))
		if err != nil {
			return fmt.Errorf("listing applications for env %q in deployment %q: %w", env, deployment, err)
		}

		return checkResponse(appResp.StatusCode(), appResp.Body)
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	var stageItem *k8sItem
	if stageResp.JSON200 != nil {
		for i := range stageResp.JSON200.Items {
			it := stageResp.JSON200.Items[i]
			spec := ptr.Deref(it.Spec, nil)
			if stringVal(spec, "name") == env {
				stageItem = &it
				break
			}
		}
	}

	if stageItem == nil {
		return nil, ErrEnvNotFound
	}

	var registered []string
	if pipelineResp.JSON200 != nil {
		registered = stringSliceVal(ptr.Deref(pipelineResp.JSON200.Spec, nil), "applications")
	}

	var appItems []k8sItem
	if appResp.JSON200 != nil {
		appItems = appResp.JSON200.Items
	}

	detail := buildEnvDetail(deployment, *stageItem, registered, appItems)

	return &detail, nil
}

// mapEnvSummary projects one Stage k8s item to an EnvSummary row.
func mapEnvSummary(item k8sItem) EnvSummary {
	spec := ptr.Deref(item.Spec, nil)
	status := ptr.Deref(item.Status, nil)

	return EnvSummary{
		Deployment:  stringVal(spec, "cdPipeline"),
		Env:         stringVal(spec, "name"),
		Cluster:     stringVal(spec, "clusterName"),
		Namespace:   stringVal(spec, "namespace"),
		TriggerType: stringVal(spec, "triggerType"),
		Status:      stringVal(status, "status"),
		Order:       stageOrder(spec),
	}
}

// sortEnvSummaries orders rows by deployment name then by Stage.spec.order.
func sortEnvSummaries(rows []EnvSummary) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Deployment != rows[j].Deployment {
			return rows[i].Deployment < rows[j].Deployment
		}

		return rows[i].Order < rows[j].Order
	})
}

// buildEnvDetail composes EnvDetail from the parent Stage item, the registered
// project list (CDPipeline.spec.applications), and the Application list scoped
// to this (deployment, env) pair.
func buildEnvDetail(deployment string, stageItem k8sItem, registered []string, apps []k8sItem) EnvDetail {
	spec := ptr.Deref(stageItem.Spec, nil)
	status := ptr.Deref(stageItem.Status, nil)

	desc := stringVal(spec, "description")
	var descPtr *string
	if desc != "" {
		descPtr = &desc
	}

	cleanTpl := stringVal(spec, "cleanTemplate")
	var cleanPtr *string
	if cleanTpl != "" {
		cleanPtr = &cleanTpl
	}

	infra := Infrastructure{
		Cluster:        stringVal(spec, "clusterName"),
		Namespace:      stringVal(spec, "namespace"),
		TriggerType:    stringVal(spec, "triggerType"),
		DeployPipeline: stringVal(spec, "triggerTemplate"),
		CleanPipeline:  cleanPtr,
	}

	gates := mapQualityGateDetails(spec)

	appsByProject := indexAppsByProjectName(apps)

	projects := make([]EnvProject, 0, len(registered))
	for _, name := range registered {
		p := EnvProject{Name: name, IngressURLs: []string{}}

		if appItem, ok := appsByProject[name]; ok {
			fillEnvProjectFromApp(&p, appItem)
		}

		projects = append(projects, p)
	}

	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	return EnvDetail{
		Deployment:     deployment,
		Env:            stringVal(spec, "name"),
		Status:         stringVal(status, "status"),
		Description:    descPtr,
		Order:          stageOrder(spec),
		Infrastructure: infra,
		QualityGates:   gates,
		Projects:       projects,
	}
}

// indexAppsByProjectName indexes ArgoCD Application items by the
// `app.edp.epam.com/app-name` label so the env-detail builder can look up
// state for each registered project.
func indexAppsByProjectName(items []k8sItem) map[string]k8sItem {
	out := make(map[string]k8sItem, len(items))

	for _, item := range items {
		labels := ptr.Deref(item.Metadata.Labels, nil)
		if labels == nil {
			continue
		}

		name, ok := labels[labelAppName]
		if !ok || name == "" {
			continue
		}

		out[name] = item
	}

	return out
}

// appFields carries the dynamic fields extracted from one ArgoCD Application
// item, shared by EnvProject and ProjectDeploymentRow.
type appFields struct {
	Status      *string
	Sync        *string
	Version     *string
	ImageTag    *string
	ImageDigest *string
	IngressURLs []string
	ArgocdURL   *string
	DeployedAt  *string
}

// extractAppFields shapes one Application's spec/status/labels into an
// appFields value. Single source of truth for fields shared by EnvProject and
// ProjectDeploymentRow; consumer-specific derivations (e.g. ValuesOverride for
// EnvProject) live alongside their consumer.
func extractAppFields(item k8sItem) appFields {
	spec := ptr.Deref(item.Spec, nil)
	status := ptr.Deref(item.Status, nil)
	labels := ptr.Deref(item.Metadata.Labels, nil)
	summary := deepGet(status, "summary")

	out := appFields{IngressURLs: []string{}}

	if v := stringVal(deepGet(status, "health"), "status"); v != "" {
		l := strings.ToLower(v)
		out.Status = &l
	}

	if v := stringVal(deepGet(status, "sync"), "status"); v != "" {
		l := strings.ToLower(v)
		out.Sync = &l
	}

	tag, repo := helmImageFields(spec)

	if v, ok := deployedVersion(spec, tag); ok {
		out.Version = &v
	}

	if tag != "" {
		out.ImageTag = &tag
	}

	if v, ok := imageDigest(repo, summary); ok {
		out.ImageDigest = &v
	}

	if urls := stringSliceVal(summary, "externalURLs"); urls != nil {
		out.IngressURLs = urls
	}

	if u, ok := argocdURL(labels); ok {
		out.ArgocdURL = &u
	}

	if v := stringVal(deepGet(status, "operationState"), "finishedAt"); v != "" {
		out.DeployedAt = &v
	}

	return out
}

// fillEnvProjectFromApp populates an EnvProject from one Application item.
// ValuesOverride is EnvProject-only (no analogue on ProjectDeploymentRow), so
// it is derived here rather than in the shared extractAppFields.
func fillEnvProjectFromApp(p *EnvProject, item k8sItem) {
	f := extractAppFields(item)

	p.Status = f.Status
	p.Sync = f.Sync
	p.Version = f.Version
	p.ImageTag = f.ImageTag
	p.ImageDigest = f.ImageDigest
	p.IngressURLs = f.IngressURLs
	p.ArgocdURL = f.ArgocdURL
	p.DeployedAt = f.DeployedAt

	spec := ptr.Deref(item.Spec, nil)
	_, hasSources := spec["sources"]
	p.ValuesOverride = &hasSources
}

// mapQualityGateDetails projects Stage.spec.qualityGates into the QualityGateDetail
// shape used by `env get`. Empty input returns an empty (non-nil) slice so the
// JSON envelope emits `[]` rather than `null`.
func mapQualityGateDetails(spec map[string]any) []QualityGateDetail {
	raw := sliceVal(spec, "qualityGates")

	gates := make([]QualityGateDetail, 0, len(raw))

	for _, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		var autotestPtr, branchPtr *string
		if v := stringVal(m, "autotestName"); v != "" {
			autotestPtr = &v
		}

		if v := stringVal(m, "branchName"); v != "" {
			branchPtr = &v
		}

		gates = append(gates, QualityGateDetail{
			Type:         stringVal(m, "qualityGateType"),
			StepName:     stringVal(m, "stepName"),
			AutotestName: autotestPtr,
			BranchName:   branchPtr,
		})
	}

	return gates
}

// deployedVersion derives the user-facing "deployed version" string from the
// Application spec. Prefers the pre-extracted helm `image.tag` parameter
// (from helmImageFields), then falls back to the first source's
// targetRevision (last path segment).
func deployedVersion(spec map[string]any, tag string) (string, bool) {
	if tag != "" {
		return tag, true
	}

	if sources := sliceVal(spec, "sources"); len(sources) > 0 {
		if first, ok := sources[0].(map[string]any); ok {
			if rev := stringVal(first, "targetRevision"); rev != "" {
				return lastPathSegment(rev), true
			}
		}
	}

	if rev := stringVal(deepGet(spec, "source"), "targetRevision"); rev != "" {
		return lastPathSegment(rev), true
	}

	return "", false
}

// helmImageFields walks `spec.sources[*].helm.parameters` and `spec.source.
// helm.parameters` once, extracting `image.tag` and `image.repository` in a
// single pass. Returns the first non-empty values found per key.
func helmImageFields(spec map[string]any) (tag, repo string) {
	scan := func(helm map[string]any) {
		if helm == nil {
			return
		}

		if tag == "" {
			if v, ok := paramValue(helm, "image.tag"); ok {
				tag = v
			}
		}

		if repo == "" {
			if v, ok := paramValue(helm, "image.repository"); ok {
				repo = v
			}
		}
	}

	if sources := sliceVal(spec, "sources"); len(sources) > 0 {
		for _, raw := range sources {
			src, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			scan(deepGet(src, "helm"))
			if tag != "" && repo != "" {
				break
			}
		}

		if tag != "" && repo != "" {
			return tag, repo
		}
	}

	scan(deepGet(spec, "source", "helm"))

	return tag, repo
}

// imageDigest matches the first entry of `status.summary.images[]` whose
// repository (substring before the digest separator) matches the helm
// `image.repository` parameter. Returns the full `sha256:...` tail and
// ok=true.
func imageDigest(repo string, summary map[string]any) (string, bool) {
	if repo == "" {
		return "", false
	}

	images := stringSliceVal(summary, "images")
	for _, img := range images {
		// Argo summary entries look like "<repo>:<tag>@sha256:<hex>" or just
		// "<repo>@sha256:<hex>". Anchor the prefix match on a separator so
		// "registry/foo" does not falsely match "registry/foo-bar:…".
		if !strings.HasPrefix(img, repo) {
			continue
		}

		rest := img[len(repo):]
		if rest == "" || (rest[0] != ':' && rest[0] != '@') {
			continue
		}

		if idx := strings.Index(rest, "@"); idx >= 0 && idx+1 < len(rest) {
			return rest[idx+1:], true
		}
	}

	return "", false
}

func lastPathSegment(s string) string {
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		return s[idx+1:]
	}
	return s
}

// paramValue looks up a parameter by name in a helm spec
// (`helm.parameters[name == <name>].value`). Returns the value + ok flag.
func paramValue(helm map[string]any, name string) (string, bool) {
	params := sliceVal(helm, "parameters")
	if params == nil {
		return "", false
	}

	m := findSliceItem(params, "name", name)
	if m == nil {
		return "", false
	}

	if v := stringVal(m, "value"); v != "" {
		return v, true
	}

	return "", false
}

// argocdURL constructs the relative path to the Argo Application detail page.
// Returns ("", false) when any of the three required label values are absent,
// so callers can leave the field null instead of emitting a broken URL.
func argocdURL(labels map[string]string) (string, bool) {
	pipeline, ok1 := labels[labelPipeline]
	stage, ok2 := labels[labelStage]
	app, ok3 := labels[labelAppName]

	if !ok1 || !ok2 || !ok3 || pipeline == "" || stage == "" || app == "" {
		return "", false
	}

	// Consumers compose this with their own base URL.
	return fmt.Sprintf("/applications/%s-%s-%s", pipeline, stage, app), true
}
