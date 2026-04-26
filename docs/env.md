# `krci env` — Environments (Stages)

Inspect KRCI environments — the `Stage` resources surfaced as
"environments" in the Portal — without leaving the terminal. Lists every
stage in the configured namespace or shows full detail for one
(deployment, env) pair, including infrastructure, quality gates, and the
projects deployed there with health, sync, version, image digest, and
ingress URLs.

**Alias:** `e`

## Subcommands

| Command                                     | Purpose                                                                |
|---------------------------------------------|------------------------------------------------------------------------|
| `env list` (`ls`)                           | List every environment in the namespace, optionally filtered           |
| `env get <deployment> <env>`                | Full detail for one environment, including the projects deployed there |

Both accept `-o, --output` with `table` (default) or `json`.

`<env>` everywhere refers to `Stage.spec.name` (the short user-facing
identifier `dev`, `stage`, `prod`, `qa`), not the compound K8s resource
name.

## `env list`

```bash
krci env list
```

```
DEPLOYMENT     ENV     CLUSTER       NAMESPACE             TRIGGER   STATUS
my-pipeline    dev     in-cluster    my-pipeline-dev       Auto      created
my-pipeline    stage   in-cluster    my-pipeline-stage     Manual    created
my-pipeline    prod    in-cluster    my-pipeline-prod      Manual    created
other-pipe     dev     in-cluster    other-pipe-dev        Auto      in_progress
```

Sort order: `deployment` ascending, then `Stage.spec.order` ascending.

### Filters

```bash
# Only one deployment
krci env list --deployment my-pipeline

# Only one cluster
krci env list --cluster in-cluster

# Combined
krci env list --deployment my-pipeline --cluster in-cluster
```

| Flag            | Purpose                                                       |
|-----------------|---------------------------------------------------------------|
| `--deployment`  | Filter by parent CDPipeline name (DNS-1123 label)             |
| `--cluster`     | Filter by `Stage.spec.clusterName` (DNS-1123 label)           |
| `-o, --output`  | `table` (default) or `json`                                   |

Empty result is success: `data.stages: []`, exit `0`, with
`No environments found.` to stderr in table mode.

### JSON envelope

```bash
krci env list -o json
```

```json
{
  "schemaVersion": "1",
  "data": {
    "stages": [
      {
        "deployment": "my-pipeline",
        "env": "dev",
        "cluster": "in-cluster",
        "namespace": "my-pipeline-dev",
        "triggerType": "Auto",
        "status": "created",
        "order": 0
      }
    ]
  }
}
```

Scripting — group environments by status:

```bash
krci env list -o json | jq -r '.data.stages[] | "\(.deployment)/\(.env): \(.status)"'
```

## `env get`

```bash
krci env get my-pipeline prod
```

```
Environment:  prod
Deployment:   my-pipeline
Status:       created
Description:  Production environment
Order:        2

Infrastructure:
  Cluster:           in-cluster
  Namespace:         my-pipeline-prod
  Trigger Type:      Manual
  Deploy Pipeline:   deploy-with-helm
  Clean Pipeline:    clean-helm

Quality Gates (2):
  - manual: stage-approval
  - autotests: smoke-tests (branch: main)

Projects (3):
PROJECT  STATUS    SYNC        VERSION   IMAGE_SHA          INGRESS
foo      healthy   synced      1.2.0     sha256:abc12345    foo.prod.example.com
bar      healthy   synced      2.0.1     sha256:def34567    bar.prod.example.com
                                                            bar-admin.prod.example.com
baz      -         -           -         -                  -
```

`<deployment>` and `<env>` are **both positional and both required**. Both
must be DNS-1123 labels (lowercase alphanumerics + hyphens, ≤ 63 chars).
Invalid input fails with exit `1` before contacting the Portal.

### Output blocks

The TTY view is layered top-to-bottom:

1. **Header** — `Environment / Deployment / Status / Description / Order`. Status
   uses `output.StatusColor` (`created` → green, `failed` → red, `in_progress` → yellow).
2. **Infrastructure** — indented block with the Stage's static placement.
   `Clean Pipeline: —` when no `cleanTemplate` is set.
3. **Quality Gates** — one bullet per gate; `autotests` gates show their
   autotest name and branch (e.g. `autotests: smoke-tests (branch: main)`).
4. **Projects sub-table** — column order: `PROJECT`, `STATUS`, `SYNC`,
   `VERSION`, `IMAGE_SHA`, `INGRESS`. Sorted by project name ascending.

Projects sub-table semantics:

- **STATUS** — ArgoCD health: `healthy` (green), `degraded` / `missing` (red),
  `progressing` (blue, same color as a running pipeline run); `-` when the
  project is registered in `CDPipeline.spec.applications` but no `Application`
  exists yet
- **VERSION** — helm `image.tag` parameter (or `targetRevision` last segment
  when helm is absent)
- **IMAGE_SHA** — short content digest (`sha256:` + first 8 hex chars =
  15 visible chars). Full digest under `imageDigest` in `-o json`
- **INGRESS** — hostnames from `status.summary.externalURLs`. Multiple URLs
  stack across visual rows (the project name appears only on the first row);
  hostnames are truncated at 50 chars but the OSC 8 hyperlink target keeps
  the full URL — `cmd-click` opens it

### Not-found errors

```bash
$ krci env get nope dev
Error: deployment "nope" not found

$ krci env get my-pipeline wat
Error: environment "wat" not found in deployment "my-pipeline"
```

Both exit `1`. Under `-o json` the message is also written to stdout as
`{"schemaVersion":"1","error":{"message":"…"}}`.

### JSON envelope

```bash
krci env get my-pipeline prod -o json
```

```json
{
  "schemaVersion": "1",
  "data": {
    "deployment": "my-pipeline",
    "env": "prod",
    "status": "created",
    "description": "Production environment",
    "order": 2,
    "infrastructure": {
      "cluster": "in-cluster",
      "namespace": "my-pipeline-prod",
      "triggerType": "Manual",
      "deployPipeline": "deploy-with-helm",
      "cleanPipeline": "clean-helm"
    },
    "qualityGates": [
      { "type": "manual", "stepName": "stage-approval", "autotestName": null, "branchName": null },
      { "type": "autotests", "stepName": "smoke", "autotestName": "smoke-tests", "branchName": "main" }
    ],
    "projects": [
      {
        "name": "foo",
        "status": "healthy",
        "sync": "synced",
        "version": "1.2.0",
        "imageTag": "1.2.0",
        "imageDigest": "sha256:abc12345...",
        "ingressUrls": ["https://foo.prod.example.com"],
        "argocdUrl": "/applications/my-pipeline-my-pipeline-prod-foo",
        "deployedAt": "2026-04-25T08:00:00Z",
        "valuesOverride": false
      },
      {
        "name": "baz",
        "status": null,
        "sync": null,
        "version": null,
        "imageTag": null,
        "imageDigest": null,
        "ingressUrls": [],
        "argocdUrl": null,
        "deployedAt": null,
        "valuesOverride": null
      }
    ]
  }
}
```

Field absence rules:

- `description`, `cleanPipeline` → `null` when the Stage spec omits them.
- Per-project dynamic fields (`status`, `sync`, `version`, `imageTag`,
  `imageDigest`, `argocdUrl`, `deployedAt`, `valuesOverride`) → `null` for
  registered-but-not-deployed projects. `ingressUrls` is always an array,
  `[]` when none.
- `qualityGates[]` is always an array (empty when none).
- In `-o json`, `imageDigest` is the FULL `sha256:...`. The table view
  shortens it to 15 visible characters under `IMAGE_SHA`.

### Scripting examples

```bash
# Projects that aren't healthy in this env
krci env get my-pipeline prod -o json |
  jq -r '.data.projects[] | select(.status != "healthy" and .status != null) | "\(.name): \(.status)"'

# All ingress URLs for one project in this env
krci env get my-pipeline prod -o json |
  jq -r '.data.projects[] | select(.name=="foo") | .ingressUrls[]'

# Quality-gate names + branches
krci env get my-pipeline prod -o json |
  jq -r '.data.qualityGates[] | "\(.type): \(.stepName) (\(.branchName // "no-branch"))"'
```

## Status colors (TTY only)

| Value           | Color   | Same as             |
|-----------------|---------|---------------------|
| `healthy`       | green   | —                   |
| `degraded`      | red     | —                   |
| `missing`       | red     | —                   |
| `progressing`   | blue    | running pipelinerun |
| anything else (`suspended`, `unknown`, …) | unstyled | — |

## Typical workflows

```bash
# Where do I have stages, and what state are they in?
krci env list

# Drill into one environment
krci env get my-pipeline prod

# Quick "what's deployed in dev?" loop, JSON-friendly
krci env list --deployment my-pipeline -o json |
  jq -r '.data.stages[] | "\(.env): \(.status)"'

# Pre-deploy sanity check — every project healthy in prod?
krci env get my-pipeline prod -o json |
  jq -e 'all(.data.projects[]; .status == "healthy" or .status == null)' >/dev/null
```
