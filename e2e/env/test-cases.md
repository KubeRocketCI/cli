# `krci env` — e2e test cases

Covers the `env` command group (alias `e`) and its two subcommands `list`
(alias `ls`) and `get`. Source: `pkg/cmd/env/`,
`pkg/cmd/internal/discovery/`, and `docs/json-schemas.md`.

The `env` group surfaces KRCI `Stage` resources as user-facing
"environments". `list` returns every stage in the configured namespace
(optionally filtered by parent deployment / cluster). `get` takes two
positionals (`<deployment> <env>`) and returns the full detail block plus
a per-project sub-table with health/sync/version/image-digest/ingress.

Every row is a self-contained contract a Haiku agent can execute. See
`../runner.md` for the agent brief and the **expect grammar** reference.

## Placeholders resolved per run

| Placeholder            | Meaning                                                                                              | Example         |
|------------------------|------------------------------------------------------------------------------------------------------|-----------------|
| `{{DEPLOYMENT_OK}}`    | A deployment (CDPipeline) that exists and has at least one Stage in the configured namespace.        | `tekton`        |
| `{{ENV_OK}}`           | A `Stage.spec.name` that exists under `{{DEPLOYMENT_OK}}` (e.g. `dev`, `stage`, `prod`, `qa`).       | `dev`           |
| `{{ENV_MULTI_INGRESS}}` | An env on `{{DEPLOYMENT_OK}}` whose Projects sub-table includes a project with **>=2 ingress URLs**. | `dev`           |
| `{{DEPLOYMENT_MISSING}}` | A deployment name that does NOT exist in the cluster.                                              | `does-not-exist`|
| `{{ENV_MISSING}}`      | An env name that does NOT exist on `{{DEPLOYMENT_OK}}`.                                              | `nope`          |
| `{{CLUSTER_OK}}`       | A cluster name that at least one Stage references (`Stage.spec.clusterName`).                        | `in-cluster`    |
| `{{PROJECT_DEPLOYED}}` | A project (Codebase) registered in `{{DEPLOYMENT_OK}}.spec.applications` with a real Application.   | `krci-portal`   |

The orchestrator fills these; the table never hard-codes them.

---

## 1. Help & discovery (env: `offline`)

Fast, idempotent, no portal — these are the first line of defence.

| ID       | Command                          | Env     | Setup | Expect                                                                                                                                  |
|----------|----------------------------------|---------|-------|-----------------------------------------------------------------------------------------------------------------------------------------|
| ENV-H-01 | `krci env --help`                | offline | —     | `exit=0; stdout~/Inspect KRCI environments/; stdout~/^Aliases:$/; stdout~/^\s+env, e$/; stdout~/^\s+list\s/; stdout~/^\s+get\s/`        |
| ENV-H-02 | `krci e --help`                  | offline | —     | `exit=0; stdout~/Inspect KRCI environments/`                                                                                            |
| ENV-H-03 | `krci env list --help`           | offline | —     | `exit=0; stdout~/--deployment string/; stdout~/--cluster string/; stdout~/-o, --output string/; stdout~/^Aliases:$/; stdout~/^\s+list, ls$/` |
| ENV-H-04 | `krci env ls --help`             | offline | —     | `exit=0; stdout~/List environments/`                                                                                                    |
| ENV-H-05 | `krci env get --help`            | offline | —     | `exit=0; stdout~/^Usage:$/; stdout~/^\s+krci env get <deployment> <env> \[flags\]$/; stdout~/-o, --output string/; stdout!~/--pr/`     |
| ENV-H-06 | `krci env`                       | offline | —     | `exit=0; stdout~/^Available Commands:$/; stdout~/^\s+list\s/; stdout~/^\s+get\s/`                                                       |

## 2. Argument validation (env: `offline`)

Wrong shape of invocation must fail fast with a helpful message and a
non-zero exit. Catches DNS-1123, output-format, and arg-count regressions.

| ID       | Command                                              | Env     | Setup | Expect                                                                  |
|----------|------------------------------------------------------|---------|-------|-------------------------------------------------------------------------|
| ENV-V-01 | `krci env get`                                       | offline | —     | `exit=1; stderr~/requires a deployment and an env/`                     |
| ENV-V-02 | `krci env get only-one`                              | offline | —     | `exit=1; stderr~/requires a deployment and an env/`                     |
| ENV-V-03 | `krci env get a b c`                                 | offline | —     | `exit=1; stderr~/requires a deployment and an env/`                     |
| ENV-V-04 | `krci env get BAD_NAME prod`                         | offline | —     | `exit=1; stderr~/<deployment> must be a valid DNS-1123 label/`          |
| ENV-V-05 | `krci env get my-pipeline Bad_Env`                   | offline | —     | `exit=1; stderr~/<env> must be a valid DNS-1123 label/`                 |
| ENV-V-06 | `krci env list --deployment Bad_Name`                | offline | —     | `exit=1; stderr~/<deployment> must be a valid DNS-1123 label/`          |
| ENV-V-07 | `krci env list --cluster Bad_Cluster`                | offline | —     | `exit=1; stderr~/--cluster must be a valid DNS-1123 label/`             |
| ENV-V-08 | `krci env list -o yaml`                              | offline | —     | `exit=1; stderr~/unknown output format/`                                |
| ENV-V-09 | `krci env get my-pipeline prod -o yaml`              | offline | —     | `exit=1; stderr~/unknown output format/`                                |
| ENV-V-10 | `krci env list --unknown-flag`                       | offline | —     | `exit=1; stderr~/unknown flag: --unknown-flag/`                         |
| ENV-V-11 | `krci env get --unknown-flag a b`                    | offline | —     | `exit=1; stderr~/unknown flag: --unknown-flag/`                         |
| ENV-V-12 | `krci env list stray-positional`                     | offline | —     | `exit=1; stderr~/unknown command "stray-positional"/`                   |
| ENV-V-13 | `krci env list --deployment --cluster x`             | offline | —     | `exit=1; stderr~/flag needs an argument: --deployment/`                 |
| ENV-V-14 | `krci env list --cluster --deployment x`             | offline | —     | `exit=1; stderr~/flag needs an argument: --cluster/`                    |

## 3. `list` — happy paths (env: `portal`)

Requires `krci auth status` → Authenticated. Each row asserts the exit
code is 0 and the JSON envelope matches the documented shape (`schemaVersion=1`,
`data.stages[]`).

| ID       | Command                                                                   | Env    | Setup                            | Expect                                                                                                                              |
|----------|---------------------------------------------------------------------------|--------|----------------------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| ENV-L-01 | `krci env list`                                                           | portal | any stage exists                 | `exit=0; stdout~/^DEPLOYMENT/; stdout~/ENV/; stdout~/CLUSTER/; stdout~/NAMESPACE/; stdout~/TRIGGER/; stdout~/STATUS/`               |
| ENV-L-02 | `krci env list -o json`                                                   | portal | any stage exists                 | `exit=0; stdout_json.schemaVersion=1; stdout_json.data.stages:exists`                                                               |
| ENV-L-03 | `krci env list --deployment {{DEPLOYMENT_OK}} -o json`                    | portal | DEPLOYMENT_OK has stages         | `exit=0; stdout_json.data.stages.0.deployment={{DEPLOYMENT_OK}}`                                                                    |
| ENV-L-04 | `krci env list --cluster {{CLUSTER_OK}} -o json`                          | portal | CLUSTER_OK has stages            | `exit=0; stdout_json.data.stages.0.cluster={{CLUSTER_OK}}`                                                                          |
| ENV-L-05 | `krci env list --deployment {{DEPLOYMENT_OK}} --cluster {{CLUSTER_OK}} -o json` | portal | both filters match a stage | `exit=0; stdout_json.data.stages.0.deployment={{DEPLOYMENT_OK}}; stdout_json.data.stages.0.cluster={{CLUSTER_OK}}`                  |
| ENV-L-06 | `krci env list --deployment {{DEPLOYMENT_MISSING}} -o json`               | portal | no matching stages               | `exit=0; stdout_json.data.stages:len=0`                                                                                             |
| ENV-L-07 | `krci env ls -o json`                                                     | portal | any stage exists                 | `exit=0; stdout_json.data.stages:exists`                                                                                            |

## 4. `list` — JSON envelope shape (env: `portal`)

Verifies every documented field is present on at least one row.

| ID       | Command                  | Env    | Setup            | Expect                                                                                                                                                                                    |
|----------|--------------------------|--------|------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| ENV-J-01 | `krci env list -o json`  | portal | any stage exists | `exit=0; stdout_json.schemaVersion=1; stdout_json.data.stages.0.deployment:exists; stdout_json.data.stages.0.env:exists; stdout_json.data.stages.0.cluster:exists; stdout_json.data.stages.0.namespace:exists; stdout_json.data.stages.0.triggerType:exists; stdout_json.data.stages.0.status:exists; stdout_json.data.stages.0.order:exists` |

## 5. `get` — happy paths (env: `portal`)

| ID       | Command                                                       | Env    | Setup                        | Expect                                                                                                                                              |
|----------|---------------------------------------------------------------|--------|------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------|
| ENV-G-01 | `krci env get {{DEPLOYMENT_OK}} {{ENV_OK}}`                   | portal | env exists                   | `exit=0; stdout~/^Environment:/; stdout~/^Deployment:/; stdout~/^Status:/; stdout~/^Infrastructure/; stdout~/^Quality Gates \([0-9]+\)/; stdout~/^Projects \([0-9]+\)/` |
| ENV-G-02 | `krci env get {{DEPLOYMENT_OK}} {{ENV_OK}} -o json`           | portal | env exists                   | `exit=0; stdout_json.schemaVersion=1; stdout_json.data.deployment={{DEPLOYMENT_OK}}; stdout_json.data.env={{ENV_OK}}; stdout_json.data.infrastructure:exists; stdout_json.data.qualityGates:exists; stdout_json.data.projects:exists` |
| ENV-G-03 | `krci env get {{DEPLOYMENT_OK}} {{ENV_OK}} -o json`           | portal | env exists with >=1 project  | `exit=0; stdout_json.data.projects.0.name:exists; stdout_json.data.projects.0.ingressUrls:exists`                                                   |
| ENV-G-04 | `krci env get {{DEPLOYMENT_OK}} {{ENV_OK}} -o json`           | portal | env exists                   | `exit=0; stdout_json.data.infrastructure.cluster:exists; stdout_json.data.infrastructure.namespace:exists; stdout_json.data.infrastructure.triggerType:exists; stdout_json.data.infrastructure.deployPipeline:exists` |

## 6. `get` — sub-table layout (env: `portal`)

Verifies the Projects sub-table column order matches `M4`, the
`IMAGE_SHA` column header is in place, and multi-ingress projects stack
across rows.

| ID       | Command                                                       | Env    | Setup                                | Expect                                                                                                                                |
|----------|---------------------------------------------------------------|--------|--------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------|
| ENV-T-01 | `krci env get {{DEPLOYMENT_OK}} {{ENV_OK}}`                   | portal | env has projects                     | `exit=0; stdout~/PROJECT/; stdout~/STATUS/; stdout~/SYNC/; stdout~/VERSION/; stdout~/IMAGE_SHA/; stdout~/INGRESS/; stdout!~/^IMAGE\s/` |
| ENV-T-02 | `krci env get {{DEPLOYMENT_OK}} {{ENV_MULTI_INGRESS}} -o json` | portal | project with >=2 ingress URLs       | `exit=0; stdout_json.data.projects.0.ingressUrls:exists`                                                                              |
| ENV-T-03 | `krci env get {{DEPLOYMENT_OK}} {{ENV_OK}} -o json`           | portal | project deployed                     | `exit=0; stdout_json.data.projects.0.status:exists`                                                                                   |

## 7. Not-found errors (env: `portal`)

`get` is detail-style: a missing deployment OR env is exit `1` with a
typed message. Under `-o json` the error envelope is on stdout.

| ID       | Command                                                            | Env    | Setup                              | Expect                                                                                                              |
|----------|--------------------------------------------------------------------|--------|------------------------------------|---------------------------------------------------------------------------------------------------------------------|
| ENV-E-01 | `krci env get {{DEPLOYMENT_MISSING}} {{ENV_OK}}`                   | portal | deployment absent                  | `exit=1; stderr~/deployment "{{DEPLOYMENT_MISSING}}" not found/`                                                    |
| ENV-E-02 | `krci env get {{DEPLOYMENT_OK}} {{ENV_MISSING}}`                   | portal | env absent on deployment           | `exit=1; stderr~/environment "{{ENV_MISSING}}" not found in deployment "{{DEPLOYMENT_OK}}"/`                       |
| ENV-E-03 | `krci env get {{DEPLOYMENT_MISSING}} {{ENV_OK}} -o json`           | portal | —                                  | `exit=1; stdout_json.schemaVersion=1; stdout_json.error.message:exists`                                             |
| ENV-E-04 | `krci env get {{DEPLOYMENT_OK}} {{ENV_MISSING}} -o json`           | portal | —                                  | `exit=1; stdout_json.error.message:exists`                                                                          |
| ENV-E-05 | `krci env list --deployment {{DEPLOYMENT_MISSING}}`                | portal | no matching stages                 | `exit=0; stderr~/No environments found\./`                                                                          |
