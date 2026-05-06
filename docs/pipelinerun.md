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
`--reason`. `start` has its own flag set (`--param`, `--label`, `--dry-run`,
`-o`) — see below.

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

| Flag         | Description                                                    |
|--------------|----------------------------------------------------------------|
| `--project`  | Filter by project name                                         |
| `--pr`       | Filter by pull request number                                  |
| `--author`   | Filter by author name                                          |
| `--branch`   | Filter by source branch                                        |
| `--type`     | `review`, `build`, `deploy`, `release`                         |
| `--status`   | `succeeded`, `failed`, `running`, `timeout`, `cancelled`       |
| `--logs`     | Append logs for the most recent matching run                   |
| `--reason`   | Show task tree + failed step + logs for the most recent run    |

Examples:

```bash
# Runs for a specific PR
krci run list --project keycloak-operator --pr 336

# Only failed reviews
krci run list --project keycloak-operator --status failed --type review

# Diagnose the latest failing run for a PR
krci run list --project keycloak-operator --pr 336 --reason
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
```

Add `--logs` for full logs or `--reason` for focused failure diagnosis.

## `pipelinerun start`

Create a new run of a Tekton pipeline by name. The pipeline name is the
required argument; everything else is optional. The new run uses Kubernetes
`metadata.generateName`, so the apiserver assigns the random suffix and the
resolved name is read back and printed.

```bash
krci pipelinerun start foo-build
```

```
NAME                  STATUS    PROJECT   PR   AUTHOR   TYPE    STARTED                DURATION
foo-build-run-zhqvj   Pending   -         -    -        build   2026-05-07T06:14:04Z   -
```

### Flags

| Flag           | Description                                                                  |
|----------------|------------------------------------------------------------------------------|
| `--param`      | Pipeline parameter as `key=value` (repeatable; split on first `=`)           |
| `--label`      | Label to attach to the resulting PipelineRun as `key=value` (repeatable)     |
| `--dry-run`    | Render the would-be PipelineRun without creating it (needs `-o json`/`yaml`) |
| `-o, --output` | `table` (default), `json`, or `yaml` (yaml only with `--dry-run`)            |

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
of the result row.

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
      "commitSha": "43820618bd016654dc81e198fe5fac95a0e87fc2"
    }
  ]
}
```

`get` returns the same `pipelineRuns` envelope with a single-element array.

Agent workflow — extract only failed tasks from a diagnosis:

```bash
krci run get review-my-app-main-a1b2c3 --reason -o json \
  | jq '.tasks[] | select(.failedStep)'
```
