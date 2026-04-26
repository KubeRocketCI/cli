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

All accept `-o, --output` with `table` (default) or `json`.

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

`<project>` is positional, required, single, and must be a DNS-1123 label
(lowercase alphanumerics + hyphens, ≤ 63 chars). Invalid input fails with
exit `1` before contacting the Portal.
