# `krci pipelinerun` — Tekton pipeline runs

List, filter, and diagnose Tekton `PipelineRun` executions (review, build,
deploy, release). Also surfaces logs and a focused failure-diagnosis view.

**Alias:** `run`

## Subcommands

| Command                       | Purpose                                       |
|-------------------------------|-----------------------------------------------|
| `pipelinerun list` (`ls`)     | List and filter runs                          |
| `pipelinerun get <name>`      | Inspect a specific run                        |
| `pipelinerun start <pipeline>` | Create a new run from a Tekton pipeline name |

`list` and `get` accept `-o, --output` (`table` | `json`), `--logs`, and
`--reason`; `get` also takes `--wait` and `--timeout`. `start` has its own
flag set (`--param`, `--label`, `--dry-run`, `-o`) — see below.

## `pipelinerun list`

```bash
krci pipelinerun list --project keycloak-operator
```

```
NAME                             STATUS      PROJECT          PR    AUTHOR     TYPE     STARTED        DURATION
build-keycloak-operator-mas...   Succeeded   keycloak-op...   336   jane-doe   build    1h ago         9m 49s
review-keycloak-operator-ma...   Failed      keycloak-op...   336   jane-doe   review   21h ago        5m 6s
review-keycloak-operator-ma...   Failed      keycloak-op...   335   jane-doe   review   22h ago        5m 47s
build-keycloak-operator-mas...   Succeeded   keycloak-op...   334   jane-doe   build    2d ago         8m 48s
build-keycloak-operator-mas...   Succeeded   keycloak-op...   331   bot        build    Apr 09 13:43   8m 40s
```

### Filters

All filters combine with AND logic.

| Flag           | Description                                                                          |
|----------------|--------------------------------------------------------------------------------------|
| `--project`    | Filter by project name                                                               |
| `--pr`         | Filter by pull request number                                                        |
| `--author`     | Filter by author name                                                                |
| `--branch`     | Filter by source branch                                                              |
| `--type`       | `review`, `build`, `deploy`, `clean`, `release`, `security`, `tests`                 |
| `--status`     | `succeeded`, `failed`, `running`, `timeout`, `cancelled`                             |
| `--deployment` | Runs of a deployment (deploy and clean runs)                                         |
| `--env`        | Runs of one environment of `--deployment`                                            |
| `--logs`       | Append logs for the most recent matching run (none until it finishes)                |
| `--reason`     | Show task tree + failed step + logs for the most recent run (none until it finishes) |

Deploy and clean runs carry no project: select them with `--deployment` and
`--env`, the names `krci env get <deployment> <env>` takes, and add
`--type deploy` for deploy runs only. `--env` needs `--deployment`. To find
where a project is deployed, run `krci project deployments <project>`.

History comes from Tekton Results and matches on the
`app.edp.epam.com/cdpipeline` and `app.edp.epam.com/cdstage` annotations. The
Tekton Results watcher records them only when its `-summary_labels` flag lists
both labels. Runs archived before that are skipped by `--deployment` and
`--env`; other filters return them with empty `deployment` and `env`. Live runs
are matched by label and always included. Fallback for older history: list with
`--type deploy` and match the run name `deploy-<deployment>-<env>-<suffix>`.

Examples:

```bash
# Runs for a specific PR
krci run list --project keycloak-operator --pr 336

# Only failed reviews
krci run list --project keycloak-operator --status failed --type review

# Diagnose the latest failing run for a PR
krci run list --project keycloak-operator --pr 336 --reason

# Deploy runs of environment dev of deployment demo, and why the latest failed
krci run list --deployment demo --env dev --type deploy
krci run list --deployment demo --env dev --type deploy --status failed --reason
```

## `pipelinerun get`

```bash
krci run get build-keycloak-operator-master-m8z4m
```

```
Pipeline: build-keycloak-operator-master-m8z4m
Status:      Succeeded
Duration:    9m 49s
Pipeline:    github-go-keycloak-operator-app-build-semver
Project:     keycloak-operator
Results:     VCS_TAG=build/1.30.0-SNAPSHOT.12
```

Add `--logs` for full logs or `--reason` for focused failure diagnosis.

`Results` lists the run's pipeline results, such as `VCS_TAG`, the version a
build produced. Only a run still in the cluster carries them: runs read back
from Tekton Results history show none.

A run still in the cluster is reported with its cluster status even when its
task tree is read from history. A run read back from history that ended with
a reason Tekton Results does not classify (`CouldntGetPipeline`,
`PipelineValidationFailed`, ...) is reported as `Failed`. `--status failed`
does not select it; the history filter matches the stored status.

### Waiting for a run (`--wait`)

`--wait` blocks until the run finishes, then prints it the way `get` does,
including `--logs` and `--reason`. The CLI polls every 10 seconds; `--timeout`
(default `1h`) limits the wait.

The exit code is `0` only when the run succeeded. A failed, cancelled or
timed-out run is still printed, and the command exits `1` with
`pipeline run "<name>" finished with status <status>` on stderr.

```bash
# Build a branch and read the version it produced
run=$(krci project build my-app -o json | jq -r '.data.name')
krci run get "$run" --wait -o json | jq -r '.pipelineRuns[0].results.VCS_TAG'

# Wait for a review run and get the failed step if it fails
krci run get review-my-app-main-a1b2c3 --wait --reason -o json
```

## `pipelinerun start`

Create a new run of a Tekton pipeline by name. The pipeline name is the
required argument; everything else is optional. The new run uses Kubernetes
`metadata.generateName`, so the apiserver assigns the random suffix and the
resolved name is read back and printed.

> To build a project branch, use [`krci project build`](project.md#project-build)
> — it resolves the pipeline, params, and labels from the project itself.
> `start` is the raw escape hatch.

```bash
krci pipelinerun start foo-build
```

```
NAME                  STATUS    PROJECT   PR   AUTHOR   TYPE    STARTED                DURATION
foo-build-run-zhqvj   Pending   -         -    -        build   2026-05-07T06:14:04Z   -
```

### Flags

| Flag           | Description                                                                                               |
|----------------|-----------------------------------------------------------------------------------------------------------|
| `--param`      | Pipeline parameter as `key=value` (repeatable; split on first `=`)                                        |
| `--label`      | Label to attach to the resulting PipelineRun as `key=value` (repeatable)                                  |
| `--dry-run`    | Render the would-be PipelineRun without creating it (YAML by default; `-o json` wraps it in the envelope) |
| `-o, --output` | `table` (default), `json`, or `yaml` (only with `--dry-run`, where it is the default)                     |

> **Params without a default** are submitted with `value: ""` (or `[]` for
> arrays). Pass `--param k=v` for any values your pipeline actually needs.

### Examples

```bash
# Override a single param
krci pipelinerun start foo-build --param git-revision=develop

# Multiple params plus a discoverability label
krci pipelinerun start foo-build --param k=v --param k2=v2 \
  --label app.edp.epam.com/codebase=my-app

# Render the would-be PipelineRun without creating it
krci pipelinerun start foo-build --dry-run -o yaml

# JSON output (for AI agents / scripting)
krci pipelinerun start foo-build -o json
```

### JSON output

`start` uses a wrapped envelope (different from `list` / `get`):

```json
{
  "schemaVersion": "1",
  "data": {
    "name": "foo-build-run-zhqvj",
    "status": "Pending",
    "project": "my-app",
    "pr": "",
    "author": "",
    "type": "build",
    "started": "2026-05-07T06:14:04Z",
    "duration": ""
  }
}
```

For `--dry-run`, `data` is the rendered PipelineRun manifest itself instead
of the result row. An error prints the
[error envelope](json-schemas.md#error-envelope) instead.

### Finding the run you just started

```bash
krci pipelinerun list --project my-app
```

## Failure diagnosis (`--reason`)

Works on both `list` (targets the most recent matching run) and `get`:

```bash
krci run get review-my-app-main-a1b2c3 --reason
```

```
Pipeline: review-my-app-main-a1b2c3
  Status:   Failed
  Duration: 3m 14s

Tasks:
  ✓ fetch-repository             Succeeded   11s
  ✓ build                        Succeeded   2m 8s
  ✗ sonar                        Failed      52s
  ✓ helm-lint                    Succeeded   8s

Failed: sonar
  Step:    sonar-scanner (exit code 2)
  Message: "step-sonar-scanner" exited with code 2

Logs: sonar
  [sonar-scanner] ERROR: QUALITY GATE STATUS: FAILED
```

Task data and logs exist only for a finished run. Until the run finishes,
`--reason` prints a note instead of the task tree, the JSON result carries
`tasksUnavailable` instead of `tasks`, and `--logs` adds nothing. `list` does
not fall back to an older run: to diagnose the last finished failure add
`--status failed`, and to wait for a run use
[`get --wait --reason`](#waiting-for-a-run---wait).

## JSON output (`list` / `get`)

`start` uses a different envelope — see the [`start` section](#pipelinerun-start) above.

```bash
krci run list --project keycloak-operator -o json
```

```json
{
  "pipelineRuns": [
    {
      "name": "build-keycloak-operator-master-m8z4m",
      "portalUrl": "https://portal.example.com/.../build-keycloak-operator-master-m8z4m",
      "status": "Succeeded",
      "pipeline": "github-go-keycloak-operator-app-build-semver",
      "project": "keycloak-operator",
      "branch": "refactor-kc-client",
      "prNumber": "336",
      "prUrl": "https://github.com/epam/edp-keycloak-operator/pull/336",
      "author": "jane-doe",
      "type": "build",
      "startTime": "2026-04-21T08:04:25.424326Z",
      "duration": "9m 49s",
      "targetBranch": "master",
      "commitSha": "43820618bd016654dc81e198fe5fac95a0e87fc2",
      "results": {
        "VCS_TAG": "build/1.30.0-SNAPSHOT.12"
      }
    }
  ]
}
```

`get` returns the same `pipelineRuns` envelope with a single-element array.

`--reason` adds `tasks`, the task tree of the first run. When there is no task
data, `tasks` is omitted and `tasksUnavailable` gives the reason:

| `tasksUnavailable` | Meaning                                                              |
|--------------------|----------------------------------------------------------------------|
| `run_not_finished` | The run is pending or still running; task data is read once it ends  |
| `not_indexed`      | The run has finished and Tekton Results has no task data for it yet  |
| `no_tasks`         | The run has finished and its record lists no TaskRun; it never scheduled a task (for example cancelled while pending, or rejected before the first task) |

```json
{
  "pipelineRuns": [
    {
      "name": "build-keycloak-operator-master-x7k2p",
      "status": "Running",
      "project": "keycloak-operator",
      "type": "build"
    }
  ],
  "tasksUnavailable": "run_not_finished"
}
```

`results` maps the run's pipeline results by name; a value keeps its Tekton
type (string, array or object). The field is omitted when the run has no
results or is read back from Tekton Results history.

A deploy or clean run has an empty `project` and carries `deployment` and
`env`; the table view of `get` shows them as `Deployment:` and `Env:`:

```json
{
  "name": "deploy-demo-dev-x7k2p",
  "portalUrl": "https://portal.example.com/.../deploy-demo-dev-x7k2p",
  "status": "Failed",
  "pipeline": "deploy",
  "project": "",
  "type": "deploy",
  "startTime": "2026-09-30T15:12:04Z",
  "duration": "10m 3s",
  "deployment": "demo",
  "env": "dev"
}
```

Agent workflow — extract only failed tasks from a diagnosis:

```bash
krci run get review-my-app-main-a1b2c3 --reason -o json \
  | jq '.tasks[] | select(.failedStep)'
```
