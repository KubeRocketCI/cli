package portal

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/KubeRocketCI/cli/internal/portal/restapi"
	"github.com/KubeRocketCI/cli/internal/ptr"
)

var (
	codebaseImageStreamResourceConfig = newK8sResourceConfig(
		"v2.edp.epam.com", "v1", "CodebaseImageStream", "codebaseimagestream", "codebaseimagestreams")
	codebaseBranchResourceConfig = newK8sResourceConfig(
		"v2.edp.epam.com", "v1", "CodebaseBranch", "codebasebranch", "codebasebranches")
)

// Labels the codebase-operator sets on the per-branch resources of a project.
const (
	labelCodebase       = "app.edp.epam.com/codebase"
	labelCodebaseBranch = "app.edp.epam.com/codebasebranch"
)

// ProjectVersionsService backs `krci project versions <project>`: the image
// versions the build pipeline pushed for each branch, read from the
// project's CodebaseImageStream resources.
type ProjectVersionsService struct {
	client      *restapi.ClientWithResponses
	clusterName string
	namespace   string
}

// NewProjectVersionsService creates a service for the given cluster and namespace.
func NewProjectVersionsService(
	client *restapi.ClientWithResponses,
	clusterName, namespace string,
) *ProjectVersionsService {
	return &ProjectVersionsService{client: client, clusterName: clusterName, namespace: namespace}
}

func (s *ProjectVersionsService) listBody(
	rc restapi.K8sListJSONBody,
	labels map[string]string,
) restapi.K8sListJSONRequestBody {
	return buildK8sListBody(s.clusterName, s.namespace, rc, labels)
}

// List returns one stream per CodebaseImageStream of project, sorted by git
// branch name, versions newest first. branch narrows the result to one git
// branch; "" returns every branch. Three concurrent calls: get the Codebase,
// so an unknown project fails instead of returning nothing; list the image
// streams by the codebase label; list the CodebaseBranches by the same label,
// which map a stream to its git branch name (a stream carries the branch
// resource name, e.g. my-app-release-2-27-781f0, not "release/2.27"). The
// error order is the Codebase get, the image stream list, then the branch
// list.
func (s *ProjectVersionsService) List(ctx context.Context, project, branch string) ([]ProjectVersionStream, error) {
	var streamResp, branchResp *restapi.K8sListResponse

	err := fanOut(ctx,
		func(ctx context.Context) error { return s.checkProjectExists(ctx, project) },
		func(ctx context.Context) (err error) {
			streamResp, err = s.listOfProject(ctx, codebaseImageStreamResourceConfig, project, "image streams")

			return err
		},
		func(ctx context.Context) (err error) {
			branchResp, err = s.listOfProject(ctx, codebaseBranchResourceConfig, project, "branches")

			return err
		},
	)
	if err != nil {
		return nil, err
	}

	return buildProjectVersionStreams(project, branch, streamResp.JSON200.Items, branchResp.JSON200.Items), nil
}

// listOfProject lists one kind by the codebase label of project; what names
// the kind in a transport error.
func (s *ProjectVersionsService) listOfProject(
	ctx context.Context, rc restapi.K8sListJSONBody, project, what string,
) (*restapi.K8sListResponse, error) {
	resp, err := s.client.K8sListWithResponse(ctx, s.listBody(rc, map[string]string{labelCodebase: project}))
	if err != nil {
		return nil, fmt.Errorf("listing %s for project %q: %w", what, project, err)
	}

	return resp, checkResponse(resp.StatusCode(), resp.Body)
}

// checkProjectExists resolves the Codebase so that an unknown project is an
// error rather than an empty result.
func (s *ProjectVersionsService) checkProjectExists(ctx context.Context, project string) error {
	ns := s.namespace

	resp, err := s.client.K8sGetWithResponse(ctx, restapi.K8sGetJSONRequestBody{
		ClusterName:    s.clusterName,
		Namespace:      &ns,
		Name:           project,
		ResourceConfig: codebaseConfig.ResourceConfig,
	})
	if err != nil {
		return fmt.Errorf("getting project %q: %w", project, err)
	}

	if err := checkResponse(resp.StatusCode(), resp.Body); err != nil {
		if errors.Is(err, ErrNotFound) {
			return newRichErr(fmt.Sprintf("project '%s' not found", project), ErrProjectNotFound)
		}

		return fmt.Errorf("getting project %q: %w", project, err)
	}

	return nil
}

// buildProjectVersionStreams joins image streams to git branch names and
// shapes the result, sorted by branch. A stream whose branch resource is
// unknown falls back to the stream name without the "<project>-" prefix. The
// result is never nil so JSON emits [] for a project without streams.
func buildProjectVersionStreams(project, branch string, streams, branches []k8sItem) []ProjectVersionStream {
	branchNames := make(map[string]string, len(branches))

	for _, item := range branches {
		if name := stringVal(ptr.Deref(item.Spec, nil), "branchName"); name != "" {
			branchNames[item.Metadata.Name] = name
		}
	}

	out := make([]ProjectVersionStream, 0, len(streams))

	for _, item := range streams {
		gitBranch := streamBranch(project, item, branchNames)
		if branch != "" && gitBranch != branch {
			continue
		}

		spec := ptr.Deref(item.Spec, nil)

		out = append(out, ProjectVersionStream{
			Branch:   gitBranch,
			Image:    stringVal(spec, "imageName"),
			Versions: imageVersions(spec),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Branch < out[j].Branch
	})

	return out
}

// streamBranch resolves the git branch of an image stream through its
// codebasebranch label and the CodebaseBranch index.
func streamBranch(project string, item k8sItem, branchNames map[string]string) string {
	labels := ptr.Deref(item.Metadata.Labels, nil)

	if name, ok := branchNames[labels[labelCodebaseBranch]]; ok {
		return name
	}

	return strings.TrimPrefix(item.Metadata.Name, project+"-")
}

// imageVersions maps spec.tags[] to ImageVersion entries, newest first. The
// operator appends tags in build order and stamps each with its creation
// time, so the list is reversed and then ordered by created descending.
// Never nil.
func imageVersions(spec map[string]any) []ImageVersion {
	raw := sliceVal(spec, "tags")
	out := make([]ImageVersion, 0, len(raw))

	for i := len(raw) - 1; i >= 0; i-- {
		m, ok := raw[i].(map[string]any)
		if !ok {
			continue
		}

		out = append(out, ImageVersion{
			Name:    stringVal(m, "name"),
			Created: stringVal(m, "created"),
			Digest:  stringVal(m, "digest"),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Created > out[j].Created
	})

	return out
}
