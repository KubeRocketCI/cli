# `krci pipelinerun` — Tekton pipeline runs

List, filter, and diagnose Tekton `PipelineRun` executions (review, build,
deploy, release). Also surfaces logs and a focused failure-diagnosis view.

**Alias:** `run`

## Subcommands

| Command                       | Purpose                                  |
|-------------------------------|------------------------------------------|
| `pipelinerun list` (`ls`)     | List and filter runs                     |
| `pipelinerun get <name>`      | Inspect a specific run                   |

Both accept `-o, --output` (`table` | `json`), `--logs`, and `--reason`.

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

## JSON output

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
