# CLI JSON Output Schemas

`krci` commands with `-o json` print one of two shapes.

**Envelope verbs** wrap the payload:

```json
{ "schemaVersion": "1", "data": <payload> }
```

On error they print the error envelope on stdout:

```json
{ "schemaVersion": "1", "error": { "message": "<human-readable>" } }
```

The error envelope is printed for an error met after the flags and arguments
are validated: no portal configured, no valid session, a Portal error.

**Bare verbs** print the payload itself and never print an error envelope; on
error stdout is empty, except `pipelinerun get --wait` for a run that did not
succeed, which prints the run and then exits `1`. The bare verbs are
[`pipelinerun list`](#krci-pipelinerun-list),
[`pipelinerun get`](#krci-pipelinerun-get-name),
[`project list`](#krci-project-list),
[`project get`](#krci-project-get-name),
[`deployment list`](#krci-deployment-list), and
[`deployment get`](#krci-deployment-get-name). Moving them to the envelope is
a breaking change.

Every error exits `1` and writes `Error: <message>` to stderr. A rejected flag
or argument is reported on stderr only. Each verb's cases, exit codes, and
error text are listed in
[json-contract-inventory.md](json-contract-inventory.md).

An interrupted command (SIGINT or SIGTERM) cancels in-flight work and dies by
that signal (shell status 130 for SIGINT, 143 for SIGTERM). As PID 1 it exits
with that status; on Windows it exits `130`. No `Error:` line is written. Under
`-o json` the error envelope is omitted when the error reports the
cancellation.

`schemaVersion` is a fixed string (`"1"` for every envelope verb) so scripts
can detect future breaking changes by checking the version.

## `krci sonar list`

```json
{
  "schemaVersion": "1",
  "data": {
    "projects": [
      {
        "key": "payments-api",
        "name": "payments-api",
        "qualityGateStatus": "OK"
      }
    ],
    "paging": { "pageIndex": 1, "pageSize": 50, "total": 1 }
  }
}
```

`visibility`, `lastAnalysisDate`, and `revision` are **optional** — SonarQube's `/api/components/search` (the upstream used by the non-admin flow) does not return them, so they are typically absent from the `list` response. `qualityGateStatus` is omitted for a project without a gate result. Use `krci sonar get <project> -o json` for full per-project metadata.

## `krci sonar get <project>`

Measures are keyed by Sonar-native metric names. **All measure values are
JSON strings, not numbers** — this mirrors SonarQube's own Web API. A string
contract is preferred because the measures map holds a mix of numeric values
(`"0"`, `"83.6"`) and enum strings (`alert_status: "OK"`, ratings `"1.0"`..`"5.0"`).
Scripts that need numeric comparison should coerce explicitly, e.g.
`jq '(.data.measures.coverage | tonumber) >= 80'`. Ratings are translated to
letter grades `A..E` by the CLI table renderer only; `-o json` preserves the
raw numeric string.

```json
{
  "schemaVersion": "1",
  "data": {
    "key": "payments-api",
    "name": "payments-api",
    "visibility": "private",
    "lastAnalysisDate": "2026-04-18T09:12:44+0000",
    "revision": "a1b2c3d",
    "qualityGateStatus": "OK",
    "measures": {
      "alert_status": "OK",
      "bugs": "2",
      "reliability_rating": "1.0",
      "vulnerabilities": "0",
      "security_rating": "1.0",
      "security_hotspots": "3",
      "security_hotspots_reviewed": "100.0",
      "security_review_rating": "1.0",
      "code_smells": "147",
      "sqale_rating": "1.0",
      "coverage": "84.2",
      "duplicated_lines_density": "1.8",
      "ncloc": "12304"
    }
  }
}
```

`visibility`, `lastAnalysisDate`, `revision`, and `qualityGateStatus` are always
present, `""` when SonarQube reports none. Dates keep SonarQube's offset format
(`+0000`), not RFC3339 `Z`.

## `krci sonar gate <project>`

```json
{
  "schemaVersion": "1",
  "data": {
    "projectStatus": {
      "status": "ERROR",
      "conditions": [
        {
          "metricKey": "new_coverage",
          "comparator": "LT",
          "errorThreshold": "80.0",
          "actualValue": "61.4",
          "status": "ERROR"
        }
      ]
    }
  }
}
```

`status` is always one of `OK | WARN | ERROR | NONE`.
`NONE` indicates the project is Sonar-bound but has never been analyzed;
`conditions` is `[]` in that case (never omitted); exit code is still `0`.
`comparator`, `errorThreshold`, and `actualValue` are omitted when SonarQube
reports none.

**JSON-to-table field-name mapping.** The table headers are intentionally
short while the JSON keys follow SonarQube's native names:

| Table column | JSON key         |
| ------------ | ---------------- |
| METRIC       | `metricKey`      |
| OPERATOR     | `comparator`     |
| THRESHOLD    | `errorThreshold` |
| ACTUAL       | `actualValue`    |
| STATUS       | `status`         |

## `krci sonar issues <project>`

Pagination is only carried on the structured `paging` object — the flat
`total`/`p`/`ps` fields from SonarQube's raw response are intentionally hidden.

```json
{
  "schemaVersion": "1",
  "data": {
    "paging": { "pageIndex": 1, "pageSize": 25, "total": 87 },
    "issues": [
      {
        "key": "AX8t9-abcdef-9K1PzA01",
        "rule": "java:S4426",
        "severity": "BLOCKER",
        "type": "VULNERABILITY",
        "status": "OPEN",
        "component": "payments-api:src/main/Payments.java",
        "project": "payments-api",
        "line": 142,
        "message": "Use a stronger algorithm — SHA-1 is cryptographically broken.",
        "effort": "10min",
        "debt": "10min",
        "creationDate": "2026-04-12T08:44:10+0000",
        "updateDate": "2026-04-12T08:44:10+0000",
        "tags": ["cwe"]
      }
    ]
  }
}
```

`line`, `effort`, `debt`, `updateDate`, and `tags` are omitted when SonarQube
reports none. `author` is omitted: the Portal does not pass it through.

Enum values:

- `severity`: `BLOCKER | CRITICAL | MAJOR | MINOR | INFO`
- `type`: `BUG | VULNERABILITY | CODE_SMELL`
- `status`: `OPEN | CONFIRMED | REOPENED | RESOLVED | CLOSED`

## `krci sca list`

```json
{
  "schemaVersion": "1",
  "data": {
    "items": [
      {
        "uuid": "550e8400-e29b-41d4-a716-446655440000",
        "name": "payments-api",
        "version": "main",
        "classifier": "APPLICATION",
        "active": true,
        "isLatest": true,
        "lastBomImport": 1713456000000,
        "lastBomImportFormat": "CycloneDX 1.4",
        "lastVulnerabilityAnalysis": 1713459600000,
        "metrics": {
          "critical": 1,
          "high": 3,
          "medium": 8,
          "low": 2,
          "unassigned": 0,
          "vulnerabilities": 14
        }
      }
    ],
    "totalCount": 42
  }
}
```

`lastBomImport` and `lastVulnerabilityAnalysis` are Unix milliseconds timestamps. `metrics` is optional — the upstream
payload omits it for projects that have never had a BOM uploaded; downstream consumers
should guard on `.data.items[].metrics != null` before indexing counts. `metrics` also
carries `components` and `vulnerableComponents` when non-zero.

Omitted when empty, zero, or false: `classifier`, `active`, `isLatest`,
`lastBomImport`, `lastBomImportFormat`, `lastVulnerabilityAnalysis`, `riskScore`.
`riskScore` is currently always omitted: the Portal does not map
Dependency-Track's `lastInheritedRiskScore`.

## `krci sca get <codebase>`

```json
{
  "schemaVersion": "1",
  "data": {
    "status": "OK",
    "project": {
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "name": "payments-api",
      "version": "main",
      "classifier": "APPLICATION",
      "active": true,
      "isLatest": true,
      "lastBomImport": 1713456000000,
      "lastBomImportFormat": "CycloneDX 1.4",
      "lastVulnerabilityAnalysis": 1713459600000,
      "metrics": {
        "critical": 1,
        "high": 3,
        "medium": 8,
        "low": 2,
        "unassigned": 0,
        "vulnerabilities": 14
      }
    },
    "metrics": {
      "critical": 1,
      "high": 3,
      "medium": 8,
      "low": 2,
      "unassigned": 0,
      "vulnerabilities": 14,
      "components": 120,
      "vulnerableComponents": 12
    }
  }
}
```

`project` has the fields of an [`sca list`](#krci-sca-list) item, `metrics`
included; the top-level `metrics` adds `components` and `vulnerableComponents`.

`status` is `"OK"`. When Dependency-Track has no project for the resolved
`(name, branch)` pair, `get`, `components`, and `findings` exit `1` with the
[error envelope](#error-envelope):

```json
{ "schemaVersion": "1", "error": { "message": "project payments-api not found — use 'krci sca list --search=payments-api' to find projects known to Dep-Track" } }
```

With `--branch` the message is `project <codebase> not found`.

## `krci sca components <codebase>`

```json
{
  "schemaVersion": "1",
  "data": {
    "status": "OK",
    "items": [
      {
        "uuid": "5c0e...",
        "name": "log4j-core",
        "version": "2.11.2",
        "group": "org.apache.logging.log4j",
        "license": "Apache-2.0",
        "metrics": {
          "critical": 1,
          "high": 0,
          "medium": 0,
          "low": 0,
          "unassigned": 0,
          "vulnerabilities": 1
        }
      }
    ],
    "totalCount": 120,
    "truncated": false
  }
}
```

`status` is `"OK"`; a codebase and branch unknown to Dependency-Track is an error, as for
[`sca get`](#krci-sca-get-codebase). `truncated` is always present.
`latestVersion`, `outdated`, `group`, `license`, `isInternal`, and `riskScore` are
omitted when empty, zero, or false; `latestVersion`, `outdated`, and `riskScore` are
currently always omitted because the Portal does not map them. `outdated` is a
server-side flag from Dep-Track — no client-side semver comparison is performed. When `--severity=<min>` is passed,
the CLI additionally filters client-side to rows whose metrics contain at least
one finding of severity `>= min` (inclusive).

## `krci sca findings <codebase>`

```json
{
  "schemaVersion": "1",
  "data": {
    "status": "OK",
    "items": [
      {
        "component": {
          "uuid": "c1",
          "name": "log4j-core",
          "version": "2.11.2"
        },
        "vulnerability": {
          "vulnId": "CVE-2021-44228",
          "source": "NVD",
          "severity": "CRITICAL",
          "cvssV3BaseScore": 10.0
        },
        "analysis": { "state": "NOT_SET", "isSuppressed": false },
        "attribution": {
          "analyzerIdentity": "OSSINDEX_ANALYZER",
          "attributedOn": 1713456000000
        }
      }
    ],
    "truncated": false
  }
}
```

`component.version`, `component.group`, `vulnerability.cvssV3BaseScore`, and
`vulnerability.cvssV2BaseScore` are omitted when empty or zero.
`analysis.state` is `""` for an unaudited finding. `attribution` is `{}` when
Dependency-Track reports no attribution.

Results are sorted by `vulnerability.severity` descending (CRITICAL first), then
`component.name` ascending, then `vulnerability.vulnId` ascending. Default
excludes suppressed findings; `--include-suppressed` opts in.

The Portal caps the response at 1000 rows per request. When the upstream returned
more, the CLI sets `truncated=true` and prints a footer hint in table output;
scripts should branch on `.data.truncated` rather than counting rows.

Severity enum: `CRITICAL | HIGH | MEDIUM | LOW | INFO | UNASSIGNED`.

## `krci env list`

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

Rows are sorted by `deployment` ascending, then by `Stage.spec.order` ascending.
Empty result returns `data.stages: []` and exit 0; the table view writes a
single `No environments found.` line to stderr.

## `krci env get <deployment> <env>`

```json
{
  "schemaVersion": "1",
  "data": {
    "deployment": "my-pipeline",
    "env": "prod",
    "status": "created",
    "detailedMessage": null,
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
      {
        "type": "manual",
        "stepName": "stage-approval",
        "autotestName": null,
        "branchName": null
      }
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
        "argocdUrl": "/applications/my-pipeline-prod-foo",
        "deployedAt": "2026-04-25T08:00:00Z",
        "valuesOverride": false,
        "conditions": [
          {
            "type": "ComparisonError",
            "message": "Failed to load target state: unable to resolve 'build/1.2.0' to a commit SHA",
            "lastTransitionTime": "2026-04-25T08:03:12Z"
          }
        ],
        "operation": {
          "phase": "Error",
          "message": "ComparisonError: Failed to load target state",
          "startedAt": "2026-04-25T08:03:00Z",
          "finishedAt": "2026-04-25T08:03:12Z"
        }
      }
    ]
  }
}
```

Field absence rules:

- `description`, `cleanPipeline` may be `null` when absent on the Stage spec.
- `detailedMessage` is the Stage's `status.detailed_message`, `null` when the
  operator reported none.
- Per-project dynamic fields (`status`, `sync`, `version`, `imageTag`,
  `imageDigest`, `argocdUrl`, `deployedAt`, `valuesOverride`) are `null` for
  projects registered in `CDPipeline.spec.applications` but without a matching
  `Application` resource. `ingressUrls` is always an array (`[]` when none).
- `conditions[]` is always an array (`[]` when none); `lastTransitionTime` is
  `null` when Argo CD omits it. `operation` is `null` for
  registered-but-not-deployed projects and for Applications never synced;
  `message`, `startedAt`, `finishedAt` are `null` when absent.
- In `-o json`, `imageDigest` carries the FULL `sha256:...` digest. The table
  view shortens it to `sha256:` plus the first 8 hex chars (15 visible chars)
  under the `IMAGE_SHA` column.
- `qualityGates[]` is always an array (empty when none).
- `projects[]` is sorted by name ascending.
- In table mode, the `INGRESS` column stacks one URL per visual row when a
  project carries multiple ingresses (project name appears only on the first
  row); each hostname is truncated to 50 chars with a trailing `...` when
  longer, but the OSC 8 hyperlink target retains the full URL. JSON output is
  unaffected by this rendering — `ingressUrls` is always the full array.

## `krci env pods <deployment> <env>`

```json
{
  "schemaVersion": "1",
  "data": {
    "deployment": "my-pipeline",
    "env": "dev",
    "cluster": "in-cluster",
    "namespace": "my-pipeline-dev",
    "pods": [
      {
        "name": "bar-7b9c8d7f6c-fghij",
        "project": "bar",
        "status": "Pending",
        "phase": "Pending",
        "readyContainers": 0,
        "totalContainers": 1,
        "restarts": 0,
        "lastRestartAt": null,
        "createdAt": "2026-04-25T08:00:00Z",
        "node": null,
        "owner": { "kind": "ReplicaSet", "name": "bar-7b9c8d7f6c" },
        "reason": null,
        "message": null,
        "conditions": [
          {
            "type": "PodScheduled",
            "status": "False",
            "reason": "Unschedulable",
            "message": "0/3 nodes are available: 3 Insufficient memory.",
            "lastTransitionTime": "2026-04-25T08:00:00Z"
          }
        ],
        "containers": []
      },
      {
        "name": "foo-6c9f7d9b8-x2x9k",
        "project": "foo",
        "status": "CrashLoopBackOff",
        "phase": "Running",
        "readyContainers": 0,
        "totalContainers": 1,
        "restarts": 7,
        "lastRestartAt": "2026-04-25T08:10:02Z",
        "createdAt": "2026-04-25T07:40:11Z",
        "node": "node-1",
        "owner": { "kind": "ReplicaSet", "name": "foo-6c9f7d9b8" },
        "reason": null,
        "message": null,
        "conditions": [
          {
            "type": "Ready",
            "status": "False",
            "reason": "ContainersNotReady",
            "message": "containers with unready status: [foo]",
            "lastTransitionTime": "2026-04-25T07:41:00Z"
          }
        ],
        "containers": [
          {
            "name": "foo",
            "init": false,
            "sidecar": false,
            "image": "registry.example.com/ns/foo:1.2.0",
            "imageID": "registry.example.com/ns/foo@sha256:abc12345...",
            "imageDigest": "sha256:abc12345...",
            "ready": false,
            "restarts": 7,
            "state": "waiting",
            "reason": "CrashLoopBackOff",
            "message": "back-off 5m0s restarting failed container=foo",
            "exitCode": null,
            "startedAt": null,
            "finishedAt": null,
            "lastTermination": {
              "reason": "Error",
              "message": null,
              "exitCode": 1,
              "startedAt": "2026-04-25T08:09:58Z",
              "finishedAt": "2026-04-25T08:10:02Z"
            }
          }
        ]
      }
    ]
  }
}
```

Rules:

- `pods[]` is every pod in the namespace of the environment, sorted by
  `name`. An empty namespace is success: `data.pods: []`, exit 0, with
  `No pods found in namespace <namespace>.` written to stderr in table mode.
- `status`, `readyContainers`, `totalContainers` and `restarts` are the
  `STATUS`, `READY` and `RESTARTS` values of `kubectl get pods`. `phase` is
  the Kubernetes pod phase. `lastRestartAt` is the time of the newest restart
  `restarts` counts, `null` without one.
- `project` is the project of the deployment whose name the pod carries in
  its `app.kubernetes.io/instance` label, `null` for any other pod.
- `node` is `null` until the pod is scheduled, `owner` is `null` for a pod
  without owners, `reason` and `message` are `null` unless the pod sets its
  own `status.reason` and `status.message`.
- `conditions[]` is always an array and carries only a condition that is
  not `True`, or a `DisruptionTarget` that is; `reason`, `message` and
  `lastTransitionTime` are `null` when absent.
- `containers[]` is always an array, init containers first, `[]` for a pod
  that reports no container yet. `sidecar` is `true` for an init container
  with `restartPolicy: Always`, `false` otherwise. `image` is the image the
  pod asks for. `imageID` is the `imageID` the container runtime reports,
  `null` until the image is present. `imageDigest` is the full `sha256:...`
  registry digest in it, `null` when it names none: a bare `sha256:...`
  `imageID` is the image's local ID, not a registry digest. `state` is one of
  `waiting`, `running`, `terminated`; `reason`, `message`, `exitCode`,
  `startedAt` and `finishedAt` are `null` when the state does not carry
  them. `lastTermination` is `null` for a container that has not been
  restarted.

Errors (exit 1, the error envelope under `-o json`):

| Condition                                  | Message                                                                                                                                  |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Unknown deployment                         | `deployment "<deployment>" not found`                                                                                                    |
| Unknown environment                        | `environment "<env>" not found in deployment "<deployment>"`                                                                             |
| Environment on another cluster             | `environment "<env>" of deployment "<deployment>" runs on cluster "<cluster>"; the Portal reads pods and events only on its own cluster` |
| The Stage names no namespace               | `environment "<env>" of deployment "<deployment>" has no namespace`                                                                      |
| No right to list pods in the namespace     | `listing pods in namespace "<namespace>": permission denied`                                                                             |

## `krci env events <deployment> <env>`

```json
{
  "schemaVersion": "1",
  "data": {
    "deployment": "my-pipeline",
    "env": "dev",
    "cluster": "in-cluster",
    "namespace": "my-pipeline-dev",
    "events": [
      {
        "type": "Warning",
        "reason": "BackOff",
        "message": "Back-off restarting failed container foo in pod foo-6c9f7d9b8-x2x9k",
        "involvedObject": { "kind": "Pod", "name": "foo-6c9f7d9b8-x2x9k" },
        "count": 42,
        "firstSeen": "2026-04-25T07:41:00Z",
        "lastSeen": "2026-04-25T08:10:02Z",
        "source": "kubelet"
      }
    ]
  }
}
```

Rules:

- `events[]` is the Kubernetes events in the namespace of the environment,
  newest first by `lastSeen`; with `--pod` only the events about that pod,
  with `--warnings` only the events of type `Warning`. No event is success:
  `data.events: []`, exit 0, with `No events found in namespace <namespace>.`
  written to stderr in table mode.
- `count`, `firstSeen` and `lastSeen` are the values `kubectl get events`
  derives; `source` is `null` when the event names no component.
- Kubernetes keeps an event for a limited time, one hour by default.
- The errors are those of `krci env pods`; the permission error names the
  events: `listing events in namespace "<namespace>": permission denied`.

## `krci project list`

Bare array, no envelope.

```json
[
  {
    "name": "my-app",
    "namespace": "ns",
    "type": "application",
    "language": "go",
    "buildTool": "go",
    "framework": "gin",
    "gitServer": "gitlab",
    "gitUrl": "https://git.example.com/my-org/my-app",
    "status": "created",
    "available": true
  },
  {
    "name": "my-lib",
    "namespace": "ns",
    "type": "library",
    "language": "python",
    "buildTool": "python",
    "framework": "python-3.13",
    "gitServer": "gitlab",
    "gitUrl": "https://git.example.com/my-org/my-lib",
    "status": "failed",
    "available": false
  }
]
```

Rules:

- One entry per `Codebase`, in the order the Portal returns them; no projects is
  `[]`.
- Always present: `name`, `namespace`, `type`, `language`, `buildTool`,
  `gitServer`, `status` (`""` when unknown), and `available` (`false` when
  unknown). `framework` and `gitUrl` are omitted when empty.
- `type` is `application`, `library`, `autotest`, `infrastructure`, or
  `system`. `status` is the operator's `created`, `in progress`, or `failed`.
- `-o` is checked after the Portal call: an unknown format with a failing
  Portal reports the Portal error.
- Errors are reported on stderr only, e.g. `authentication required: listing
  projects: unauthorized: please run 'krci auth login'`.

## `krci project get <name>`

Bare object, no envelope: one [`project list`](#krci-project-list) entry.

```json
{
  "name": "my-app",
  "namespace": "ns",
  "type": "application",
  "language": "go",
  "buildTool": "go",
  "framework": "gin",
  "gitServer": "gitlab",
  "gitUrl": "https://git.example.com/my-org/my-app",
  "status": "created",
  "available": true
}
```

Rules:

- Errors are reported on stderr only: `project "<name>" not found`, or e.g.
  `authentication required: getting project "<name>": unauthorized: please run
  'krci auth login'`.
- `-o` is checked after the Portal call.

## `krci project build <name>`

Envelope verb. `data` is the run the Portal created, the same row as
[`pipelinerun start`](#krci-pipelinerun-start):

```json
{
  "schemaVersion": "1",
  "data": {
    "name": "build-my-app-main-x9k2p",
    "status": "Pending",
    "project": "my-app",
    "pr": "",
    "author": "dev-user",
    "type": "build",
    "started": "2026-01-02T10:02:00Z",
    "duration": ""
  }
}
```

Rules:

- Every field is always present, `""` when unknown; a new run is usually
  `Pending` with an empty `duration`.
- With `--dry-run`, `data` is the rendered `PipelineRun`, as for
  `pipelinerun start`.
- Errors print the error envelope; the messages are listed in
  [project.md](project.md#errors). Rejected flags (`-o`, `--param`, `--branch`)
  are reported on stderr only.

## `krci project deployments <project>`

```json
{
  "schemaVersion": "1",
  "data": {
    "project": "foo",
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
        "cluster": "in-cluster",
        "namespace": "my-pipeline-dev",
        "triggerType": "Auto",
        "deployedAt": "2026-04-25T08:00:00Z",
        "ingressUrls": ["https://foo.dev.example.com"],
        "argocdUrl": "/applications/my-pipeline-dev-foo",
        "conditions": [],
        "operation": {
          "phase": "Succeeded",
          "message": "successfully synced (all tasks run)",
          "startedAt": "2026-04-25T07:59:40Z",
          "finishedAt": "2026-04-25T08:00:00Z"
        }
      },
      {
        "deployment": "other-pipe",
        "env": "dev",
        "deployed": false,
        "status": null,
        "sync": null,
        "version": null,
        "imageTag": null,
        "imageDigest": null,
        "cluster": "in-cluster",
        "namespace": "other-pipe-dev",
        "triggerType": "Auto",
        "deployedAt": null,
        "ingressUrls": [],
        "argocdUrl": null,
        "conditions": [],
        "operation": null
      }
    ]
  }
}
```

Rules:

- Rows are sorted by `deployment` ascending, then by `Stage.spec.order` ascending.
- `deployed: false` rows still carry `cluster`, `namespace`, and `triggerType`
  from the Stage so the user sees the full footprint of the project even when
  no `Application` resource exists yet. Dynamic fields are `null`,
  `ingressUrls` and `conditions` are `[]`, `operation` is `null`.
- `conditions[]` and `operation` follow the `krci env get` rules above.
- An empty result is success: `data.rows: []`, exit 0, with
  `No deployments found for project <name>.` written to stderr in table mode.
- "Project missing" and "project deployed nowhere" are indistinguishable from
  the user's perspective: both return `data.rows: []` with exit 0.
- In table mode, the `INGRESS` column stacks one URL per visual row when a
  (deployment, env) row carries multiple ingresses (deployment/env/etc.
  appear only on the first row). Hostnames are truncated to 50 visible chars;
  the OSC 8 hyperlink target keeps the full URL. JSON output is unaffected.

## `krci project versions <project>`

```json
{
  "schemaVersion": "1",
  "data": {
    "project": "payments-api",
    "streams": [
      {
        "branch": "main",
        "image": "registry.example.com/ns/payments-api",
        "versions": [
          { "name": "0.1.0-SNAPSHOT.14", "created": "2026-09-08T06:12:40Z", "digest": "sha256:9f1c02ab..." },
          { "name": "0.1.0-SNAPSHOT.13", "created": "2026-09-05T11:47:02Z" }
        ]
      },
      {
        "branch": "release/1.2",
        "image": "registry.example.com/ns/payments-api",
        "versions": []
      }
    ]
  }
}
```

Rules:

- `streams[]` is always an array, one entry per `CodebaseImageStream` of the
  project, sorted by `branch`; with `--branch` at most one entry.
- `branch` is the git branch name (`release/1.2`, not the operator's
  `release-1-2-<hash>` resource name).
- `versions[]` is always an array (`[]` for a branch never built), newest
  first by `created`. `digest` is omitted when the registry reported none;
  in JSON it is the full `sha256:...`.
- An empty `streams` is success (exit `0`); an unknown project is an error
  envelope with `project '<name>' not found` and exit `1`.

## `krci deployment list`

Bare array, no envelope.

```json
[
  {
    "name": "my-pipeline",
    "namespace": "ns",
    "applications": ["my-app", "my-api"],
    "stages": ["dev", "qa"],
    "status": "created",
    "available": true
  },
  {
    "name": "api-pipeline",
    "namespace": "ns",
    "applications": ["my-api"],
    "stages": [],
    "description": "API pipeline",
    "status": "created",
    "available": true
  }
]
```

Rules:

- One entry per `CDPipeline`, in the order the Portal returns them; no
  deployments is `[]`.
- `stages[]` holds the environment names in promotion order (`spec.order`),
  `[]` for a deployment without environments. `applications` is `null` when the
  deployment lists none.
- Always present: `name`, `namespace`, `applications`, `stages`, `status`,
  `available`. `description` and `detailedMessage` are omitted when empty;
  `detailedMessage` is set by the operator on a failure.
- `-o` is checked after the Portal call.
- Errors are reported on stderr only, e.g. `authentication required:
  unauthorized: please run 'krci auth login'`.

## `krci deployment get <name>`

Bare object, no envelope.

```json
{
  "name": "my-pipeline",
  "namespace": "ns",
  "applications": ["my-app", "my-api"],
  "status": "created",
  "available": true,
  "stages": [
    {
      "name": "dev",
      "order": 0,
      "triggerType": "Manual",
      "qualityGates": [
        { "name": "approve", "type": "manual" },
        { "name": "smoke", "type": "autotests" }
      ],
      "namespace": "my-pipeline-dev",
      "clusterName": "in-cluster",
      "description": "Development environment",
      "status": "created",
      "detailedMessage": "environment is ready",
      "available": true
    }
  ]
}
```

Rules:

- `stages[]` is sorted by `order`. It is `null`, not `[]`, for a deployment
  without environments: guard `jq` paths with `.stages // []`.
- Stage `qualityGates[]` is `null` when the stage spec has no `qualityGates`.
- Always present on a stage: `name`, `order`, `triggerType`, `qualityGates`,
  `namespace`, `status`, `available`. `clusterName`, `description`, and
  `detailedMessage` are omitted when empty.
- `description` and `detailedMessage` of the deployment are omitted when empty.
- `-o` is checked after the Portal call.
- Errors are reported on stderr only: `deployment "<name>" not found`, or e.g.
  `authentication required: unauthorized: please run 'krci auth login'`.

## `krci auth status`

```json
{
  "schemaVersion": "1",
  "data": {
    "authenticated": true,
    "user": "user@example.com",
    "name": "User Name",
    "groups": ["admin", "developers"],
    "expiresAt": "2026-04-22T08:22:00Z"
  }
}
```

Rules:

- `authenticated` is always `true` in a success envelope: a missing,
  expired, or rejected session is an error (exit `1`) and produces the error
  envelope below with one of:

  | Condition                           | Message                                              |
  | ----------------------------------- | ---------------------------------------------------- |
  | No stored session                   | `not authenticated: run 'krci auth login'`           |
  | Stored session expired, no refresh  | `session expired: run 'krci auth login'`             |
  | Expired `KRCI_TOKEN`                | `KRCI_TOKEN has expired: supply a fresh token`       |
  | Portal rejects the token (401)      | `not authenticated: the portal rejected the token`   |
  | Portal unreachable or other failure | `verifying the token with the portal: <cause>`       |

- `groups` is always an array (`[]` when none).
- `expiresAt` is RFC3339 in UTC, `null` when the stored token has no expiry.
- `user` and `name` are omitted when the token claims cannot be decoded; the
  session is still valid in that case.

## Error envelope

```json
{
  "schemaVersion": "1",
  "error": { "message": "project 'payments-api' not found" }
}
```

The same message follows `Error: ` on stderr. The bare verbs print only the
stderr line. A message can carry the verb's context, e.g. `listing stages: `,
before the cause. Messages are pinned per verb in
[json-contract-inventory.md](json-contract-inventory.md).

Common messages:

| Condition                                    | Message                                                                                  |
| -------------------------------------------- | ---------------------------------------------------------------------------------------- |
| No session, or the stored session is unusable | `not authenticated: run 'krci auth login'`, after the verb's context                    |
| Portal rejects the token (401)               | `authentication required: [<context>: ]unauthorized: please run 'krci auth login'`, then a blank line and `Run: krci auth login` |
| RBAC denied (403)                            | `permission denied`                                                                      |
| Resource missing (404), no verb-specific text | `resource not found`                                                                    |
| Portal too old for the command               | `portal has no endpoint for this command (<route>); upgrade the portal`                  |
| Other status                                 | `portal returned HTTP <code>: <body>`; a long body is cut and ends with `...`            |
| Network failure                              | the verb's context, then the transport error                                             |

Not-found messages name the resource and differ per verb; see each verb's
section.

## `krci pipelinerun list`

Bare object, no envelope.

```json
{
  "pipelineRuns": [
    {
      "name": "build-my-app-main-r2b3c",
      "portalUrl": "https://portal.example.com/c/in-cluster/cicd/pipelineruns/ns/build-my-app-main-r2b3c",
      "status": "Running",
      "pipeline": "github-my-app-app-build-default",
      "project": "my-app",
      "branch": "main",
      "author": "dev-user",
      "type": "build",
      "startTime": "2026-01-02T10:00:00Z",
      "duration": "2m 3s",
      "commitSha": "1f2e3d4c5b6a79881726354a5b6c7d8e9f012345"
    },
    {
      "name": "review-my-app-main-s1a2b",
      "portalUrl": "https://portal.example.com/c/in-cluster/cicd/pipelineruns/ns/review-my-app-main-s1a2b",
      "status": "Succeeded",
      "pipeline": "github-my-app-app-review",
      "project": "my-app",
      "branch": "feature-login",
      "prNumber": "42",
      "prUrl": "https://git.example.com/my-org/my-app/pull/42",
      "author": "dev-user",
      "type": "review",
      "startTime": "2026-01-02T09:50:00Z",
      "duration": "3m 10s",
      "targetBranch": "main",
      "commitSha": "0a1b2c3d4e5f60718293a4b5c6d7e8f901234567",
      "results": {
        "IMAGE_TAGS": ["1.0.0-SNAPSHOT.4", "latest"],
        "VCS_TAG": "review/1.0.0-SNAPSHOT.4"
      }
    },
    {
      "name": "deploy-my-pipeline-dev-d4e5f",
      "portalUrl": "https://portal.example.com/c/in-cluster/cicd/pipelineruns/ns/deploy-my-pipeline-dev-d4e5f",
      "status": "Failed",
      "pipeline": "deploy",
      "project": "",
      "author": "release-bot",
      "type": "deploy",
      "startTime": "2026-01-02T09:30:00Z",
      "duration": "1m 5s",
      "deployment": "my-pipeline",
      "env": "dev"
    }
  ]
}
```

Rules:

- `pipelineRuns[]` is always an array; no runs is `{"pipelineRuns": []}` and
  exit `0`.
- Runs come from the cluster plus one page of Tekton Results records matching
  the filters, in the backend's order, not by time. A run in both is listed
  once, from the cluster. Sorted by `startTime`, newest
  first; a run without a parseable `startTime` sorts last. `--status running`
  reads the cluster only.
- Always present: `name`, `status`, `pipeline`, `project`, `startTime`, each
  `""` when unknown. `project` is `""` for deploy and clean runs.
- Omitted when empty: `portalUrl`, `branch`, `prNumber`, `prUrl`, `author`,
  `type`, `duration`, `targetBranch`, `commitSha`, `deployment`, `env`,
  `results`.
- `status` is `Succeeded`, `Failed`, `Running`, `Timeout`, or `Cancelled`; `""`
  for a run in the cluster without conditions.
- `results` maps the pipeline results by name, each value with its Tekton type
  (string, array, or object). Only a run still in the cluster carries them.
- `portalUrl` is omitted when the portal URL or the cluster name is not
  configured.
- `duration` is computed by the CLI, never read from the Portal: completion
  time minus start time for a finished run, the host clock minus start time
  for a running run, so a running run's value grows between calls. Format:
  `30s`, `4m 20s`, `1h 2m 3s`. The completion time itself is not emitted.
  `pipelinerun start` reports the Portal's own format (`2m3s`).
- `--logs` and `--reason` apply to `pipelineRuns[0]` only and add the fields
  described under [`pipelinerun get`](#krci-pipelinerun-get-name).
- `-o` is checked after the Portal call: an unknown format with runs is an
  error, with no runs the table's `No pipeline runs found` line is printed and
  the exit code is `0`.
- Errors are reported on stderr only, e.g. `authentication required: fetching
  live pipeline runs: unauthorized: please run 'krci auth login'`.

## `krci pipelinerun get <name>`

Bare object, no envelope: the [`pipelinerun list`](#krci-pipelinerun-list)
object with exactly one entry in `pipelineRuns`. The run is looked up in the
cluster first, then in Tekton Results.

```json
{
  "pipelineRuns": [
    {
      "name": "build-my-app-main-f9x8y",
      "portalUrl": "https://portal.example.com/c/in-cluster/cicd/pipelineruns/ns/build-my-app-main-f9x8y",
      "status": "Failed",
      "pipeline": "github-my-app-app-build-default",
      "project": "my-app",
      "branch": "main",
      "author": "dev-user",
      "type": "build",
      "startTime": "2026-01-02T09:40:00Z",
      "duration": "5m 30s",
      "commitSha": "9f8e7d6c5b4a39281706f5e4d3c2b1a098765432"
    }
  ],
  "tasks": [
    {
      "name": "fetch-repository",
      "status": "Succeeded",
      "duration": "30s",
      "message": "All Steps have completed executing"
    },
    {
      "name": "compile",
      "status": "Failed",
      "duration": "4m 20s",
      "failedStep": "compile",
      "exitCode": 2,
      "message": "\"step-compile\" exited with code 2",
      "logs": "... (5 lines truncated)\ncompiling package example.com/my-app/internal/pkg06\n...\nbuild failed\nexit status 2\n"
    }
  ]
}
```

Rules:

- `--logs` adds `logs`, the full log of the run, not truncated.
- `--reason` adds `tasks[]`, in start order, task names without the run name
  prefix. `name` and `status` are always present; `duration`, `failedStep`,
  `exitCode`, `message`, and `logs` are omitted when empty or zero. Tasks carry
  no timestamps. `logs` is set for failed tasks only, ANSI codes removed, and
  keeps the last 25 lines: a longer log starts with `... (<n> lines
  truncated)`. Without task data, `tasks` is omitted and `tasksUnavailable`
  gives the reason; the values are listed in
  [pipelinerun.md](pipelinerun.md#json-output-list--get). `--reason` wins over
  `--logs`.
- `--wait` waits for the run to finish: polling interval, timeout, and stderr
  text are in [Waiting for a run](pipelinerun.md#waiting-for-a-run---wait). A
  run that does not succeed prints its JSON, then exits `1`.
- `-o` is not validated: any value other than `json` prints the table.
- Errors are reported on stderr only: `pipeline run "<name>" not found`; with
  no session, `fetching live pipeline runs: obtaining auth token: not
  authenticated: run 'krci auth login'`.

## `krci pipelinerun start`

The start verb prints the run the Portal created. Its table columns match
`krci pipelinerun list`; its JSON keys do not (`pr` and `started` here,
`prNumber` and `startTime` in list). Every field is always present, `""` when
unknown; empty table cells render as `-`. `krci project build` prints the same
envelope.

### Success envelope

```json
{
  "schemaVersion": "1",
  "data": {
    "name":     "<apiserver-assigned name, e.g. foo-build-run-x9k2p>",
    "status":   "Pending|Running|Succeeded|Failed|Cancelled|Timeout",
    "project":  "<codebase or empty>",
    "pr":       "<pr number or empty>",
    "author":   "<git author or empty>",
    "type":     "<pipelinetype label or empty>",
    "started":  "<RFC3339 or empty>",
    "duration": "<Portal duration, e.g. 2m3s, or empty>"
  }
}
```

### Error envelope

```json
{
  "schemaVersion": "1",
  "error": { "message": "pipeline 'ghost' not found" }
}
```

### Dry-run envelope (-o json)

`data` carries the rendered `PipelineRun` resource as a parsed JSON object —
not a string. Default and `-o yaml` modes emit the same resource as YAML
(suitable for piping to `kubectl apply -f -`).

```json
{
  "schemaVersion": "1",
  "data": {
    "apiVersion": "tekton.dev/v1",
    "kind": "PipelineRun",
    "metadata": {
      "generateName": "foo-build-run-",
      "labels": { "app.edp.epam.com/codebase": "my-app" }
    },
    "spec": {
      "params": [ { "name": "git-revision", "value": "main" } ]
    }
  }
}
```

### Common messages

User-facing messages on the not-found path are synthesised CLI-side from a
stable `error.reason` tag the Portal returns. The Portal deliberately does
not put resource-identifying text in `error.message` (cluster-hardening
policy applied uniformly to all REST routes), so the CLI builds the user
message from the pipeline name it already has plus the reason it received.

All errors exit `1` (per the global rule at the top of this document).

| Condition                                   | Message                                                                                       |
| ------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Pipeline not found                          | `pipeline '<name>' not found`                                                                 |
| TriggerTemplate referenced but missing      | `pipeline '<name>' references a TriggerTemplate that does not exist`                          |
| Malformed TriggerTemplate label             | `pipeline '<name>' has malformed TriggerTemplate label`                                       |
| Platform rejection (400/408/409/422/429)    | `platform rejected request: <Portal message or HTTP status phrase>`                           |
| RBAC denied                                 | `permission denied`                                                                           |
| Portal upstream 500/502/503/504             | `upstream service unavailable: <body>`, cut as above                                          |
| Duplicate / malformed `--param` / `--label` | `duplicate parameter '<k>'` / `parameter must be key=value` / `parameter key must not be empty`, and the same with `label` |
| `--dry-run` with `-o table`                 | `--dry-run cannot use -o table (use -o json or -o yaml)`                                      |
| `-o yaml` without `--dry-run`               | `-o yaml requires --dry-run`                                                                  |
| Unknown `-o`                                | `unknown output format: <format> (use 'json', 'yaml', or 'table')`                            |

Rejected flags (`--param` / `--label` errors, `-o` errors) are reported on stderr only; no envelope.

