# `krci project` — Codebase resources

Inspect the `Codebase` resources registered in the Portal — applications,
libraries, autotests, and infrastructure repos — and trace where each one
is currently deployed.

**Alias:** `proj`

## Subcommands

| Command                              | Purpose                                                                              |
|--------------------------------------|--------------------------------------------------------------------------------------|
| `project list` (`ls`)                | List all projects                                                                    |
| `project get <name>`                 | Show a single project                                                                |
| `project deployments <project>`      | Every (deployment, env) row where the project is registered, with health/version    |
| `project build <name>`               | Start the build pipeline of a project branch — the Portal's Build button           |

All accept `-o, --output` with `table` (default) or `json`; `build` also takes
`yaml` under `--dry-run`.

## `project list`

```bash
krci project list
```

```
NAME                             TYPE          LANGUAGE     BUILD TOOL        STATUS
keycloak-operator                application   other        go                created
payments-api                     application   java         maven             created
orders-ui                        application   javascript   npm               created
shared-libs                      library       other        kaniko            created
smoke-tests                      autotest      other        gradle            created
```

Scripting — pick all failed-status projects:

```bash
krci project list -o json | jq -r '.[] | select(.status=="failed") | .name'
```

## `project get`

```bash
krci project get keycloak-operator
```

```
Name:         keycloak-operator
Namespace:    team-a
Type:         application
Language:     other
Build Tool:   go
Framework:    keycloak-operator
Git Server:   github
Git URL:      https://github.com/example-org/keycloak-operator
Status:       created
Available:    true
```

JSON envelope (full output):

```json
{
  "name": "keycloak-operator",
  "namespace": "team-a",
  "type": "application",
  "language": "other",
  "buildTool": "go",
  "framework": "keycloak-operator",
  "gitServer": "github",
  "gitUrl": "https://github.com/example-org/keycloak-operator",
  "status": "created",
  "available": true
}
```

Pull a single field from an agent workflow:

```bash
krci project get keycloak-operator -o json | jq -r '.gitUrl'
```

## `project deployments`

Answers "where is my project deployed, and at what version?" — one row per
(deployment, env) pair where the project is registered, with current
health, sync, version, image digest, cluster, namespace, and ingress URLs.

```bash
krci project deployments payments-api
```

```
DEPLOYMENT    ENV     STATUS        SYNC        VERSION   IMAGE_SHA          CLUSTER       NAMESPACE             INGRESS
my-pipeline   dev     healthy       synced      1.2.3     sha256:abc12345    cluster-a     my-pipeline-dev       payments-api.dev.example.com
my-pipeline   stage   healthy       synced      1.2.3     sha256:abc12345    cluster-a     my-pipeline-stage     payments-api.stage.example.com
my-pipeline   prod    progressing   synced      1.2.3     sha256:abc12345    cluster-a     my-pipeline-prod      payments-api.prod.example.com
other-pipe    dev     degraded      outofsync   1.2.0     sha256:def34567    cluster-b     other-pipe-dev        payments-api.dev2.example.com
legacy        dev     -             -           -         -                  cluster-a     legacy-dev            -
```

Columns (in order, `INGRESS` is always last):

- **DEPLOYMENT / ENV** — the (CDPipeline, Stage) pair
- **STATUS** — ArgoCD health: `healthy` (green), `degraded` / `missing` (red),
  `progressing` (blue, same color as a running pipeline run), `-` when the project is
  registered but no `Application` exists yet
- **SYNC** — ArgoCD sync state (`synced`, `outofsync`, `unknown`); `-` when not deployed
- **VERSION** — derived from the Argo Application's helm `image.tag` parameter (or
  `targetRevision` when no helm parameter is set)
- **IMAGE_SHA** — short content digest (`sha256:` + first 8 hex chars = 15 visible chars)
  matched from `status.summary.images`. Full digest in `-o json`
- **CLUSTER / NAMESPACE / TRIGGER TYPE** — Stage's static placement, populated even on
  `deployed: false` rows so you see the full footprint
- **INGRESS** — hostnames from the Application's `status.summary.externalURLs`. Multiple
  URLs stack across visual rows (project name appears only on the first row); hostnames
  are truncated at 50 chars but the OSC 8 hyperlink target keeps the full URL —
  `cmd-click` opens it

Sort order: `deployment` ascending, then `Stage.spec.order` ascending.

### "Registered but not deployed" rows

When a project is listed in `CDPipeline.spec.applications` but no
`Application` resource exists yet, the row is emitted with `-` placeholders
in the dynamic columns (table) or `null` in JSON, and `deployed: false`.
`cluster`, `namespace`, and `triggerType` still come through from the Stage
so you see exactly where the project will land once deployed.

### JSON envelope

```bash
krci project deployments payments-api -o json
```

```json
{
  "schemaVersion": "1",
  "data": {
    "project": "payments-api",
    "rows": [
      {
        "deployment": "my-pipeline",
        "env": "dev",
        "deployed": true,
        "status": "healthy",
        "sync": "synced",
        "version": "1.2.3",
        "imageTag": "1.2.3",
        "imageDigest": "sha256:abc12345...",
        "cluster": "cluster-a",
        "namespace": "my-pipeline-dev",
        "triggerType": "Auto",
        "deployedAt": "2026-04-25T08:00:00Z",
        "ingressUrls": ["https://payments-api.dev.example.com"],
        "argocdUrl": "/applications/my-pipeline-dev-payments-api"
      },
      {
        "deployment": "legacy",
        "env": "dev",
        "deployed": false,
        "status": null,
        "sync": null,
        "version": null,
        "imageTag": null,
        "imageDigest": null,
        "cluster": "cluster-a",
        "namespace": "legacy-dev",
        "triggerType": "Auto",
        "deployedAt": null,
        "ingressUrls": [],
        "argocdUrl": null
      }
    ]
  }
}
```

Empty result is success: `data.rows: []`, exit `0`, with
`No deployments found for project <name>.` written to stderr in table mode.
"Project missing" and "project deployed nowhere" are intentionally
indistinguishable — both return empty rows with exit 0.

### Scripting examples

```bash
# Every deployed instance of the project, with version
krci project deployments payments-api -o json |
  jq -r '.data.rows[] | select(.deployed) | "\(.deployment)/\(.env): \(.version)"'

# Find degraded environments
krci project deployments payments-api -o json |
  jq -r '.data.rows[] | select(.status=="degraded") | "\(.deployment)/\(.env)"'

# Registered-but-not-deployed pairs (where the project is expected but missing)
krci project deployments payments-api -o json |
  jq -r '.data.rows[] | select(.deployed==false) | "\(.deployment)/\(.env) on \(.cluster)"'

# Count deployed vs registered-only
krci project deployments payments-api -o json |
  jq '.data.rows | group_by(.deployed) | map({deployed: .[0].deployed, count: length})'
```

`<project>` is positional, required, single, and must be a DNS-1123 name
(lowercase alphanumerics + hyphens, no dots, ≤ 253 chars). Invalid input fails with
exit `1` before contacting the Portal.

## `project build`

Start the build pipeline of a project branch — exactly what the Portal's
**Build** button does. The build pipeline, its params, labels, and service
account are resolved from the project and the branch, so you never name the
Tekton pipeline, the `CodebaseBranch`, or the TriggerTemplate.

```bash
krci project build my-app
```

```
NAME                          STATUS    PROJECT   PR   AUTHOR   TYPE    STARTED                DURATION
build-my-app-main-zhqvj       Pending   my-app    -    -        build   2026-09-14T06:14:04Z   -
```

The columns are the `pipelinerun list` / `pipelinerun start` columns, so the
run is immediately traceable with `krci pipelinerun list --project my-app`.

### Flags

| Flag           | Description                                                                  |
|----------------|------------------------------------------------------------------------------|
| `--branch`     | Git branch to build (default: the project's `spec.defaultBranch`)            |
| `--param`      | Pipeline parameter as `key=value` (repeatable; split on first `=`)           |
| `--dry-run`    | Render the would-be PipelineRun without creating it (needs `-o json`/`yaml`) |
| `-o, --output` | `table` (default), `json`, or `yaml` (yaml only with `--dry-run`)            |

There is no `--label`: labels are what this command resolves for you. Use
[`krci pipelinerun start`](pipelinerun.md#pipelinerun-start) when you need
raw control over a run.

### Managed params

These params are derived from the project and its branch and cannot be
overridden — `--param` rejects them before any network call:

`git-source-url`, `git-source-revision`, `targetBranch`, `CODEBASE_NAME`,
`CODEBASEBRANCH_NAME`, `gitfullrepositoryname`, `changeNumber`,
`patchsetNumber`

Everything else is fair game, including `COMMIT_MESSAGE`,
`COMMIT_MESSAGE_PATTERN`, and any pipeline-specific param.

### Examples

```bash
# Build the project's default branch
krci project build my-app

# Build a specific branch
krci project build my-app --branch feat/x

# Override a non-managed param
krci project build my-app --param COMMIT_MESSAGE="rebuild after config change"

# Render the would-be PipelineRun without creating it
krci project build my-app --dry-run -o yaml

# JSON output (for AI agents / scripting)
krci project build my-app -o json
```

### JSON output

`build` uses the same wrapped envelope as `pipelinerun start`:

```json
{
  "schemaVersion": "1",
  "data": {
    "name": "build-my-app-main-zhqvj",
    "status": "Pending",
    "project": "my-app",
    "pr": "",
    "author": "",
    "type": "build",
    "started": "2026-09-14T06:14:04Z",
    "duration": ""
  }
}
```

For `--dry-run`, `data` is the rendered PipelineRun manifest instead of the
result row.

### Dry run

`--dry-run` renders the manifest and creates nothing; it is answered even when
the branch is not ready or a build is already running. The manifest matches the
Portal's "Build with params" editor with three deliberate differences:

- `metadata.generateName: build-<codebaseBranch>-` instead of `metadata.name`,
  so the apiserver assigns the suffix (same as the webhook path and
  `pipelinerun start`); the CodebaseBranch name is cut to 51 characters so
  the final name stays within Kubernetes' 63-character limit;
- `metadata.namespace` is pinned to the configured namespace;
- `spec.params` is sorted alphabetically.

### One build at a time — best effort

A build that is already running for the branch is rejected. Every build run of
the branch is checked, not only the latest one, so an older run that is still
active blocks a new build as well; the Portal's Build button looks only at the
latest run. The check is list-then-create, not a platform guarantee: two
callers firing at the same moment can both get a run. Nothing serializes
builds per branch today, and this command does not claim to.

### Errors

All failures exit `1` and write one line to stderr.

| Situation (portal reason)                        | Message                                                                                                                       |
|--------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| Reserved param (`managed_param_override`)        | `parameter 'git-source-url' is set from the project and branch by 'project build'; use 'krci pipelinerun start' for raw overrides` |
| Project missing (`codebase_not_found`)           | `project 'my-app' not found`                                                                                                  |
| GitLab CI project (`gitlab_ci_not_supported`)    | `project 'my-app' builds via GitLab CI; 'krci project build' does not support it yet, trigger it from the portal`              |
| No default branch (`branch_not_specified`)       | `project 'my-app' has no default branch; pass --branch`                                                                       |
| Branch missing (`codebase_branch_not_found`)     | `branch 'feat/x' of project 'my-app' not found`                                                                               |
| Duplicate branches (`codebase_branch_ambiguous`) | `branch 'feat/x' of project 'my-app' matches more than one CodebaseBranch`                                                     |
| No build pipeline (`build_pipeline_not_configured`) | `branch 'feat/x' of project 'my-app' has no build pipeline configured`                                                      |
| Git server missing (`git_server_not_found`)      | `project 'my-app' references a git server that does not exist`                                                                |
| Template missing or broken (`trigger_template_not_found`, `build_template_misconfigured`) | `no usable build TriggerTemplate for project 'my-app' (platform misconfiguration)`   |
| Branch not ready (`codebase_branch_not_ready`)   | `branch 'feat/x' of project 'my-app' is not ready (status must be 'created'); check: krci project get my-app`                  |
| Build running (`build_in_progress`)              | `a build is already running for branch 'feat/x' of project 'my-app'; check: krci pipelinerun list --project my-app --status running` |
| Portal could not evaluate state (`list_truncated`) | `portal could not safely evaluate the build state for project 'my-app' (resource list truncated); retry or contact an operator` |
| Portal too old (route missing)                   | `portal has no endpoint for this command (Route POST:/rest/v1/pipelineruns/build not found); upgrade the portal`              |
| Not signed in / no RBAC                          | `unauthorized: please run 'krci auth login'` / `permission denied`                                                            |

With `--branch` omitted the CLI cannot learn which branch the portal resolved,
so every branch-specific message reads `the default branch of project 'my-app'`
instead of naming it.
