# CLI JSON Output Schemas

All JSON output produced by `krci` commands with `-o json` follows a single
envelope shape:

```json
{ "schemaVersion": "1", "data": <payload> }
```

On error, the envelope is:

```json
{ "schemaVersion": "1", "error": { "message": "<human-readable>" } }
```

The exit code is `1` on any error. The plain-text error message is also
written to stderr.

`schemaVersion` is a fixed string (`"1"` for every verb listed below) so
scripts can detect future breaking changes by checking the version.

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

`visibility`, `lastAnalysisDate`, and `revision` are **optional** — SonarQube's `/api/components/search` (the upstream used by the non-admin flow) does not return them, so they are typically absent from the `list` response. Use `krci sonar get <project> -o json` for full per-project metadata.

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
    "lastAnalysisDate": "2026-04-18T09:12:44Z",
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
        "author": "alice@example.com",
        "creationDate": "2026-04-12T08:44:10Z"
      }
    ]
  }
}
```

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
        "riskScore": 12.5,
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

`lastBomImport` is a Unix milliseconds timestamp. `metrics` is optional — the upstream
payload omits it for projects that have never had a BOM uploaded; downstream consumers
should guard on `.data.items[].metrics != null` before indexing counts.

## `krci sca get <codebase>`

Two variants: `status: "OK"` when the codebase has a Dep-Track project; `status: "NONE"`
when the Portal's lookup returned no project for the resolved `(name, branch)` pair.

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
      "riskScore": 12.5
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

```json
{ "schemaVersion": "1", "data": { "status": "NONE" } }
```

When `status` is `"NONE"` the `project` and `metrics` fields are absent. CLI exit is `0`
in both cases.

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
        "latestVersion": "2.24.0",
        "outdated": true,
        "group": "org.apache.logging.log4j",
        "license": "Apache-2.0",
        "isInternal": false,
        "riskScore": 9.8,
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
    "totalCount": 120
  }
}
```

`status` is `"OK"` for a bound codebase and `"NONE"` for an unbound one (with
`items: [], totalCount: 0`). `outdated` is a server-side flag from Dep-Track —
no client-side semver comparison is performed. When `--severity=<min>` is passed,
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

- `authenticated` is always `true` in a success envelope: a missing or
  expired session is an error (exit `1`) and produces the error envelope
  below with `not authenticated: run 'krci auth login'` or
  `session expired: run 'krci auth login'`.
- `groups` is always an array (`[]` when none).
- `expiresAt` is RFC3339 in UTC, `null` when the stored token has no expiry.
- `user` and `name` are omitted when the token claims cannot be decoded; the
  session is still valid in that case.

## Error envelope

```json
{
  "schemaVersion": "1",
  "error": { "message": "project payments-api not found" }
}
```

Common messages:

| Condition                   | Message                                |
| --------------------------- | -------------------------------------- |
| Missing auth token          | `please run 'krci auth login'`         |
| Unknown project (404)       | `project <name> not found`             |
| Unknown pull request (404)  | `pull request <id> not found`          |
| Upstream 5xx / network      | `portal returned HTTP 500: <cause>`    |
| Invalid flag value          | Flag-specific message (e.g. enum list) |


## `krci pipelinerun start`

The start verb reuses the same column shape as `krci pipelinerun list`. Empty
cells render as `-` in table mode and as `""` in JSON mode (matches list).

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
    "duration": "<m+s or empty>"
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
| Platform admission rejection (400/422)      | `platform rejected request: <HTTP status phrase>` (Portal does not echo K8s admission detail) |
| RBAC denied                                 | `permission denied`                                                                           |
| Portal upstream 5xx                         | `upstream service unavailable: <cause>`                                                       |
| Duplicate / malformed `--param` / `--label` | `duplicate parameter '<k>'` / `parameter must be key=value` / `label key must not be empty`   |
| `--dry-run` with `-o table`                 | `--dry-run cannot use -o table (use -o json or -o yaml)`                                      |

