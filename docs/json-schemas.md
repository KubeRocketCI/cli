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
