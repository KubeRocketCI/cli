# `krci project` — e2e test cases

Covers the `project` command group (alias `proj`). Source:
`pkg/cmd/project/`. This file presently exercises the **`deployments`**
verb only — `list` and `get` are out of scope here and may be added in a
future iteration. Row IDs use a verb-segmented prefix
(`PROJ-D-NN` = project / deployments / NN) so siblings can co-exist.

`krci project deployments <project>` returns every (deployment, env) row
in the configured namespace where the given project is registered, with
current health/sync/version/image-digest/cluster/namespace/ingress.
Rows for stages where the project is registered (`CDPipeline.spec.applications`)
but no `Application` exists yet are emitted with `-` placeholders (table)
or `null` values (JSON, with `deployed: false`).

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
| PROJ-D-V-03 | `krci project deployments BAD_NAME`                 | offline | —     | `exit=1; stderr~/<project> must be a valid DNS-1123 label/`             |
| PROJ-D-V-04 | `krci project deployments UPPER`                    | offline | —     | `exit=1; stderr~/<project> must be a valid DNS-1123 label/`             |
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
