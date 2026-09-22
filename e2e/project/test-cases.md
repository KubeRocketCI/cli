# `krci project` — e2e test cases

Covers the `project` command group (alias `proj`). Source:
`pkg/cmd/project/`. This file exercises the **`deployments`** and
**`build`** verbs — `list` and `get` are out of scope here and may be added
in a future iteration. Row IDs use a verb-segmented prefix
(`PROJ-D-NN` = project / deployments / NN, `PROJ-B-NN` = project / build /
NN) so siblings can co-exist.

`krci project deployments <project>` returns every (deployment, env) row
in the configured namespace where the given project is registered, with
current health/sync/version/image-digest/cluster/namespace/ingress.
Rows for stages where the project is registered (`CDPipeline.spec.applications`)
but no `Application` exists yet are emitted with `-` placeholders (table)
or `null` values (JSON, with `deployed: false`).

`krci project build <name>` starts the build pipeline of a project branch,
resolving the pipeline, params, labels, and service account from the project
and the branch. `--branch` defaults to the project's default branch, the
project-derived params are reserved, and `--dry-run` renders the manifest
without creating anything.

`krci project versions <project>` lists the image versions the build
pipeline pushed, one row per branch (`PROJ-V-NN`); `--branch` lists every
version of one git branch, newest first. Read-only.

Every row is a self-contained contract a Haiku agent can execute. See
`../runner.md` for the agent brief and the **expect grammar** reference.

## Placeholders resolved per run

| Placeholder            | Meaning                                                                                     | Example         |
|------------------------|---------------------------------------------------------------------------------------------|-----------------|
| `{{PROJECT_DEPLOYED}}` | A codebase listed in at least one `CDPipeline.spec.applications` AND with a real Application. | `krci-portal`   |
| `{{PROJECT_REGISTERED}}` | A codebase listed in `CDPipeline.spec.applications` but with no matching Application.     | `pending-svc`   |
| `{{PROJECT_NOWHERE}}`  | A real codebase not listed in any CDPipeline (returns empty rows, exit 0).                  | `infra-gitops`  |
| `{{PROJECT_MISSING}}`  | A name that does not exist anywhere in the cluster.                                         | `does-not-exist`|
| `{{DEPLOYMENT_OK}}`    | A deployment that registers `{{PROJECT_DEPLOYED}}` in `spec.applications`.                  | `krci-portal`   |
| `{{ENV_OK}}`           | An env on `{{DEPLOYMENT_OK}}` where `{{PROJECT_DEPLOYED}}` has a deployed Application.       | `dev`           |
| `{{PROJECT_BUILD}}`    | A codebase with a build pipeline whose branch status is `created` (buildable).               | `test-go-app`   |
| `{{PROJECT_BUILD_BRANCH}}` | The git branch of `{{PROJECT_BUILD}}` to build.                                         | `main`          |
| `{{PROJECT_NOT_READY}}` | A codebase whose CodebaseBranch status is not `created`.                                    | `test-dotnet-app` |
| `{{PROJECT_WITH_VERSIONS}}` | A codebase with at least one CodebaseImageStream that carries tags (a built branch).    | `payments-api`  |
| `{{PROJECT_VERSIONS_BRANCH}}` | A git branch of `{{PROJECT_WITH_VERSIONS}}` that has been built at least once.        | `main`          |

The orchestrator fills these; the table never hard-codes them.

---

## 1. Help & discovery (env: `offline`)

Fast, idempotent, no portal — these are the first line of defence.

| ID         | Command                                  | Env     | Setup | Expect                                                                                                                           |
|------------|------------------------------------------|---------|-------|----------------------------------------------------------------------------------------------------------------------------------|
| PROJ-D-H-01 | `krci project --help`                   | offline | —     | `exit=0; stdout~/Manage projects \(Codebases\)/; stdout~/^Aliases:$/; stdout~/^\s+project, proj$/; stdout~/^\s+deployments\s/`   |
| PROJ-D-H-02 | `krci proj --help`                      | offline | —     | `exit=0; stdout~/Manage projects \(Codebases\)/`                                                                                 |
| PROJ-D-H-03 | `krci project deployments --help`       | offline | —     | `exit=0; stdout~/^Usage:$/; stdout~/^\s+krci project deployments <project> \[flags\]$/; stdout~/-o, --output string/`           |
| PROJ-D-H-04 | `krci project`                          | offline | —     | `exit=0; stdout~/^Available Commands:$/; stdout~/^\s+deployments\s/; stdout~/^\s+get\s/; stdout~/^\s+list\s/`                   |

## 2. Argument validation (env: `offline`)

Wrong shape of invocation must fail fast with a helpful message and a
non-zero exit. Catches DNS-1123, output-format, and arg-count regressions.

| ID         | Command                                              | Env     | Setup | Expect                                                                  |
|------------|------------------------------------------------------|---------|-------|-------------------------------------------------------------------------|
| PROJ-D-V-01 | `krci project deployments`                          | offline | —     | `exit=1; stderr~/requires a project \(codebase\) name/`                 |
| PROJ-D-V-02 | `krci project deployments a b`                      | offline | —     | `exit=1; stderr~/requires a project \(codebase\) name/`                 |
| PROJ-D-V-03 | `krci project deployments BAD_NAME`                 | offline | —     | `exit=1; stderr~/<project> must be a valid DNS-1123 name/`             |
| PROJ-D-V-04 | `krci project deployments UPPER`                    | offline | —     | `exit=1; stderr~/<project> must be a valid DNS-1123 name/`             |
| PROJ-D-V-05 | `krci project deployments my-app -o yaml`           | offline | —     | `exit=1; stderr~/unknown output format/`                                |
| PROJ-D-V-06 | `krci project deployments my-app --unknown-flag`    | offline | —     | `exit=1; stderr~/unknown flag: --unknown-flag/`                         |
| PROJ-D-V-07 | `krci project deployments -o`                       | offline | —     | `exit=1; stderr~/flag needs an argument/`                               |

## 3. Happy paths (env: `portal`)

Requires `krci auth status` → Authenticated. Each row asserts the exit
code is 0 and the JSON envelope matches the documented shape
(`schemaVersion=1`, `data.project`, `data.rows[]`).

| ID         | Command                                                          | Env    | Setup                                          | Expect                                                                                                                                                |
|------------|------------------------------------------------------------------|--------|------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| PROJ-D-P-01 | `krci project deployments {{PROJECT_DEPLOYED}}`                 | portal | project deployed somewhere                     | `exit=0; stdout~/^DEPLOYMENT/; stdout~/ENV/; stdout~/STATUS/; stdout~/SYNC/; stdout~/VERSION/; stdout~/IMAGE_SHA/; stdout~/CLUSTER/; stdout~/NAMESPACE/; stdout~/INGRESS/` |
| PROJ-D-P-02 | `krci project deployments {{PROJECT_DEPLOYED}} -o json`         | portal | project deployed somewhere                     | `exit=0; stdout_json.schemaVersion=1; stdout_json.data.project={{PROJECT_DEPLOYED}}; stdout_json.data.rows:exists`                                    |
| PROJ-D-P-03 | `krci project deployments {{PROJECT_DEPLOYED}} -o json`         | portal | project deployed in DEPLOYMENT_OK/ENV_OK       | `exit=0; stdout_json.data.rows.0.deployment:exists; stdout_json.data.rows.0.env:exists; stdout_json.data.rows.0.deployed:exists; stdout_json.data.rows.0.cluster:exists; stdout_json.data.rows.0.namespace:exists; stdout_json.data.rows.0.triggerType:exists` |
| PROJ-D-P-04 | `krci project deployments {{PROJECT_DEPLOYED}} -o json`         | portal | project deployed somewhere                     | `exit=0; stdout_json.data.rows.0.conditions:exists; stdout_json.data.rows.0.operation:exists`                                                                        |

## 4. Empty results (env: `portal`)

"Project missing" and "project deployed nowhere" are indistinguishable
from the user's perspective: both exit 0 with `data.rows: []`.

| ID         | Command                                                       | Env    | Setup                              | Expect                                                                                                                          |
|------------|---------------------------------------------------------------|--------|------------------------------------|---------------------------------------------------------------------------------------------------------------------------------|
| PROJ-D-N-01 | `krci project deployments {{PROJECT_NOWHERE}}`               | portal | codebase exists, no CDPipeline references it | `exit=0; stderr~/No deployments found for project {{PROJECT_NOWHERE}}\./`                                              |
| PROJ-D-N-02 | `krci project deployments {{PROJECT_NOWHERE}} -o json`       | portal | —                                  | `exit=0; stdout_json.schemaVersion=1; stdout_json.data.project={{PROJECT_NOWHERE}}; stdout_json.data.rows:len=0`                |
| PROJ-D-N-03 | `krci project deployments {{PROJECT_MISSING}}`               | portal | name does not exist                | `exit=0; stderr~/No deployments found for project {{PROJECT_MISSING}}\./`                                                       |
| PROJ-D-N-04 | `krci project deployments {{PROJECT_MISSING}} -o json`       | portal | —                                  | `exit=0; stdout_json.data.rows:len=0`                                                                                           |

## 5. Registered-but-not-deployed rows (env: `portal`)

Stages where the project is in `CDPipeline.spec.applications` but no
`Application` resource exists yet must still appear with `deployed:false`,
null dynamic fields, and `cluster`/`namespace`/`triggerType` populated
from the Stage. Skip when no such pair exists.

| ID         | Command                                                          | Env    | Setup                                                | Expect                                                                                                                |
|------------|------------------------------------------------------------------|--------|------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------|
| PROJ-D-R-01 | `krci project deployments {{PROJECT_REGISTERED}} -o json`       | portal | project registered without Application               | `exit=0; stdout_json.data.rows.0.deployed=false; stdout_json.data.rows.0.cluster:exists; stdout_json.data.rows.0.namespace:exists; stdout_json.data.rows.0.triggerType:exists` |

## 6. Sub-table layout (env: `portal`)

Verifies the table column order matches `M6`, the `IMAGE_SHA` header is
in place, and multi-ingress rows stack across visual rows.

| ID         | Command                                                          | Env    | Setup                              | Expect                                                                                                                |
|------------|------------------------------------------------------------------|--------|------------------------------------|-----------------------------------------------------------------------------------------------------------------------|
| PROJ-D-T-01 | `krci project deployments {{PROJECT_DEPLOYED}}`                 | portal | project deployed somewhere         | `exit=0; stdout~/IMAGE_SHA/; stdout!~/^IMAGE\s/`                                                                       |
| PROJ-D-T-02 | `krci project deployments {{PROJECT_DEPLOYED}} -o json`         | portal | project has multiple deployed rows | `exit=0; stdout_json.data.rows.0.ingressUrls:exists`                                                                  |

## 7. `project build` (env: `offline`)

Validation runs before any network call, so these rows need only a built
binary. `--label` must stay absent: `build` resolves labels itself.

| ID         | Command                                                     | Env     | Setup | Expect                                                                                                                                  |
|------------|-------------------------------------------------------------|---------|-------|-------------------------------------------------------------------------------------------------------------------------------------------|
| PROJ-B-01  | `krci project build --help`                                 | offline | —     | `exit=0; stdout~/--branch string/; stdout~/--param stringArray/; stdout~/--dry-run/; stdout~/-o, --output string/; stdout!~/--label/`      |
| PROJ-B-02  | `krci project build`                                        | offline | —     | `exit=1; stderr~/requires a project name/`                                                                                               |
| PROJ-B-03  | `krci project build My.App`                                 | offline | —     | `exit=1; stderr~/lowercase alphanumeric/`                                                                                                |
| PROJ-B-04  | `krci project build my-app --param git-source-url=x`        | offline | —     | `exit=1; stderr~/managed|set from the project/`                                                                                          |
| PROJ-B-05  | `krci project build my-app --dry-run -o table`              | offline | —     | `exit=1; stderr~/--dry-run cannot use -o table/`                                                                                         |
| PROJ-B-06  | `krci project build my-app -o yaml`                         | offline | —     | `exit=1; stderr~/-o yaml requires --dry-run/`                                                                                            |

## 8. `project build` (env: `portal`)

Every row here is independent and creates nothing: dry-run rows render
without a create, and the error rows are rejected before a create.

| ID         | Command                                                                                          | Env    | Setup                                                  | Expect                                                                                                    |
|------------|--------------------------------------------------------------------------------------------------|--------|--------------------------------------------------------|---------------------------------------------------------------------------------------------------------|
| PROJ-B-07  | `krci project build {{PROJECT_BUILD}} --branch {{PROJECT_BUILD_BRANCH}} --dry-run -o yaml`      | portal | buildable project                                      | `exit=0; stdout~/generateName: build-/; stdout~/app.edp.epam.com\/codebase: {{PROJECT_BUILD}}/`            |
| PROJ-B-08  | `krci project build {{PROJECT_BUILD}} --dry-run -o json`                                        | portal | buildable project, default branch resolves             | `exit=0; stdout_json.schemaVersion=1; stdout_json.data.metadata.generateName:exists`                      |
| PROJ-B-09  | `krci project build {{PROJECT_MISSING}}`                                                        | portal | name does not exist                                    | `exit=1; stderr~/not found/`                                                                              |
| PROJ-B-10  | `krci project build {{PROJECT_BUILD}} --branch does-not-exist-branch-xyz`                       | portal | project exists, branch does not                        | `exit=1; stderr~/not found/`                                                                              |
| PROJ-B-11  | `krci project build {{PROJECT_NOT_READY}}`                                                      | portal | CodebaseBranch status is not `created`                 | `exit=1; stderr~/not ready/`                                                                              |

## 9. `project build` — create then reject (env: `portal`, serial)

PROJ-B-12 creates a real PipelineRun. PROJ-B-13 runs immediately after it,
while that run is still active. The in-progress check is best effort — a
list-then-create check over every build run of the branch — so PROJ-B-13
asserts the sequential contract only and says nothing about concurrent
callers.

| ID         | Command                                          | Env    | Setup                                                 | Expect                                                                            |
|------------|--------------------------------------------------|--------|-------------------------------------------------------|-----------------------------------------------------------------------------------|
| PROJ-B-12  | `krci project build {{PROJECT_BUILD}} -o json`   | portal | no build running for the branch                       | `exit=0; stdout_json.data.type=build; stdout_json.data.project={{PROJECT_BUILD}}` |
| PROJ-B-13  | `krci project build {{PROJECT_BUILD}}`           | portal | run immediately after PROJ-B-12, its run still active | `exit=1; stderr~/already running/`                                                |

> Rows in section 9 depend on each other and create a PipelineRun — the
> orchestrator must run them serially, in order, **after** all other
> sections.

## 10. `project versions` (env: `offline`)

| ID         | Command                                                     | Env     | Setup | Expect                                                                                                              |
|------------|-------------------------------------------------------------|---------|-------|---------------------------------------------------------------------------------------------------------------------|
| PROJ-V-01  | `krci project versions --help`                              | offline | —     | `exit=0; stdout~/^\s+krci project versions <project> \[flags\]$/; stdout~/--branch string/; stdout~/-o, --output string/` |
| PROJ-V-02  | `krci project versions`                                     | offline | —     | `exit=1; stderr~/requires a project name/`                                                                          |
| PROJ-V-03  | `krci project versions a b`                                 | offline | —     | `exit=1; stderr~/requires a project name/`                                                                          |
| PROJ-V-04  | `krci project versions BAD_NAME`                            | offline | —     | `exit=1; stderr~/<project> must be a valid DNS-1123 name/`                                                         |
| PROJ-V-05  | `krci project versions my-app -o yaml`                      | offline | —     | `exit=1; stderr~/unknown output format/`                                                                            |
| PROJ-V-06  | `krci project versions my-app --branch $(printf 'b%.0s' $(seq 254))` | offline | — | `exit=1; stderr~/--branch must be at most 253 characters/`                                                          |
| PROJ-V-07  | `krci project`                                              | offline | —     | `exit=0; stdout~/^\s+versions\s/`                                                                                   |

## 11. `project versions` (env: `portal`)

Read-only: every row lists existing image streams and creates nothing.

| ID         | Command                                                                                               | Env    | Setup                                          | Expect                                                                                                                                                  |
|------------|-------------------------------------------------------------------------------------------------------|--------|------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------|
| PROJ-V-08  | `krci project versions {{PROJECT_WITH_VERSIONS}}`                                                     | portal | project has a built branch                     | `exit=0; stdout~/^BRANCH/; stdout~/VERSIONS/; stdout~/LATEST/; stdout~/CREATED/; stdout~/IMAGE/; stdout~/{{PROJECT_VERSIONS_BRANCH}}/`                  |
| PROJ-V-09  | `krci project versions {{PROJECT_WITH_VERSIONS}} -o json`                                             | portal | project has a built branch                     | `exit=0; stdout_json.schemaVersion=1; stdout_json.data.project={{PROJECT_WITH_VERSIONS}}; stdout_json.data.streams:exists; stdout_json.data.streams.0.branch:exists; stdout_json.data.streams.0.image:exists; stdout_json.data.streams.0.versions:exists` |
| PROJ-V-10  | `krci project versions {{PROJECT_WITH_VERSIONS}} --branch {{PROJECT_VERSIONS_BRANCH}}`                | portal | branch built at least once                     | `exit=0; stdout~/^VERSION/; stdout~/CREATED/; stdout~/DIGEST/; stdout~/IMAGE/`                                                                          |
| PROJ-V-11  | `krci project versions {{PROJECT_WITH_VERSIONS}} --branch {{PROJECT_VERSIONS_BRANCH}} -o json`        | portal | branch built at least once                     | `exit=0; stdout_json.data.streams:len=1; stdout_json.data.streams.0.branch={{PROJECT_VERSIONS_BRANCH}}; stdout_json.data.streams.0.versions.0.name:exists; stdout_json.data.streams.0.versions.0.created:exists` |
| PROJ-V-12  | `krci project versions {{PROJECT_WITH_VERSIONS}} --branch does-not-exist-branch-xyz`                  | portal | project exists, branch does not                | `exit=0; stderr~/No versions found for branch does-not-exist-branch-xyz of project {{PROJECT_WITH_VERSIONS}}\./`                                        |
| PROJ-V-13  | `krci project versions {{PROJECT_WITH_VERSIONS}} --branch does-not-exist-branch-xyz -o json`          | portal | project exists, branch does not                | `exit=0; stdout_json.data.streams:len=0`                                                                                                                |
| PROJ-V-14  | `krci project versions {{PROJECT_MISSING}}`                                                           | portal | name does not exist                            | `exit=1; stderr~/project '{{PROJECT_MISSING}}' not found/`                                                                                              |
| PROJ-V-15  | `krci project versions {{PROJECT_MISSING}} -o json`                                                   | portal | name does not exist                            | `exit=1; stdout_json.schemaVersion=1; stdout_json.error.message:exists`                                                                                 |
