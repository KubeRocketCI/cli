# `krci sonar` — SonarQube from the terminal

Inspect the SonarQube state surfaced by the KubeRocketCI Portal — projects,
measures, quality gate, and issues — without leaving the terminal. The CLI
reuses the SonarQube binding already configured on the Portal
(`SONAR_HOST_URL` / `SONAR_TOKEN`), so no extra credentials are required.

## Subcommands

| Command                | Purpose                                            |
|------------------------|----------------------------------------------------|
| `sonar list`           | List projects with their quality-gate status       |
| `sonar get <project>`  | Project metadata, measures, and gate summary       |
| `sonar gate <project>` | Quality-gate verdict with per-condition breakdown  |
| `sonar issues <proj>`  | Issue list with severity / type / status filters   |

All commands accept `-o, --output` with `table` (default) or `json`.

## `sonar list`

```bash
krci sonar list --search keycloak
```

```
KEY                 NAME                GATE
keycloak-operator   keycloak-operator   OK

1 project, page 1 of 1 (page-size 50)
```

Flags: `--search`, `--page`, `--page-size` (max 500).

Pick failing projects from a script:

```bash
krci sonar list -o json | jq -r '.data.projects[] | select(.qualityGateStatus=="ERROR") | .key'
```

## `sonar get`

```bash
krci sonar get keycloak-operator
```

```
Project        keycloak-operator
Name           keycloak-operator
Visibility     private
Last run       2026-04-21 08:11 UTC (1h ago)
Revision       —
Quality Gate   OK

Reliability:
  Bugs                 0
  Reliability rating   A

Security:
  Vulnerabilities          0
  Security rating          A
  Security hotspots        0
  Hotspots reviewed        100.0%
  Security review rating   A

Maintainability:
  Code smells              1125
  Maintainability rating   A

Coverage & Size:
  Line coverage      83.6%
  Duplicated lines   2.0%
  Lines of code      28518
```

Scope to a pull request, a named branch, or pull a single metric:

```bash
krci sonar get keycloak-operator --pr 123
krci sonar get keycloak-operator --branch main
krci sonar get keycloak-operator -o json | jq -r '.data.measures.coverage'
```

`--pr` and `--branch` are mutually exclusive — they map to SonarQube's two
scope selectors. Omit both to target the project's default branch.

## `sonar gate`

```bash
krci sonar gate keycloak-operator
```

```
Quality Gate: OK  (keycloak-operator)

METRIC                           OPERATOR   THRESHOLD   ACTUAL    STATUS
new_reliability_rating           GT         1           1         OK
new_security_rating              GT         1           1         OK
new_maintainability_rating       GT         1           1         OK
new_coverage                     LT         80          83.4      OK
new_duplicated_lines_density     GT         6           2.08347   OK
blocker_violations               GT         0           0         OK
critical_violations              GT         0           0         OK
new_security_hotspots_reviewed   LT         100         100.0     OK
```

CI guardrail — fail the build when the gate is red:

```bash
if [ "$(krci sonar gate keycloak-operator -o json | jq -r '.data.projectStatus.status')" = "ERROR" ]; then
  echo "gate failed"; exit 1
fi
```

Flags: `--pr`, `--branch`, `-o`. `--pr` and `--branch` are mutually exclusive.

## `sonar issues`

```bash
krci sonar issues keycloak-operator --page-size 5
```

```
Issues for keycloak-operator  (page 1/225, 1125 total)

KEY        SEVERITY   TYPE         STATUS   RULE                      COMPONENT                                                       LINE   MESSAGE
dcf769f3…  INFO       CODE_SMELL   OPEN     yaml:LineLengthCheck      keycloak-operator:pkg/client/keycloakapi/openapi/oapicfg.yaml   1      line too long (122 > 80 characters)
e0a31af3…  MAJOR      CODE_SMELL   OPEN     yaml:DocumentStartCheck   keycloak-operator:pkg/client/keycloakapi/openapi/oapicfg.yaml   2      missing document start "---"
086584ff…  MINOR      CODE_SMELL   OPEN     yaml:IndentationCheck     keycloak-operator:pkg/client/keycloakapi/openapi/openapi.yaml   8      wrong indentation: expected 2 but found 0
```

Common filters combine with AND:

```bash
# Blockers + criticals
krci sonar issues keycloak-operator --severity BLOCKER,CRITICAL

# Bugs + vulnerabilities, include resolved, sort ascending
krci sonar issues keycloak-operator --type BUG,VULNERABILITY --resolved --sort SEVERITY --asc

# Scope to a pull request
krci sonar issues keycloak-operator --pr 123 --severity BLOCKER

# Scope to a named branch (mirrors Sonar's ?branch= URL param)
krci sonar issues keycloak-operator --branch main --severity BLOCKER
```

| Flag             | Values                                                           |
|------------------|------------------------------------------------------------------|
| `--severity`     | `BLOCKER`, `CRITICAL`, `MAJOR`, `MINOR`, `INFO` (csv/repeatable) |
| `--type`         | `BUG`, `VULNERABILITY`, `CODE_SMELL`                             |
| `--status`       | `OPEN`, `CONFIRMED`, `REOPENED`, `RESOLVED`, `CLOSED`            |
| `--resolved`     | Include resolved issues                                          |
| `--sort / --asc` | Sort field (forwarded to SonarQube) and direction               |
| `--page[-size]`  | Pagination; `--page-size` max 500                                |
| `--pr`           | PR id to scope the view (mutually exclusive with `--branch`)     |
| `--branch`       | Branch name to scope the view (mutually exclusive with `--pr`)   |

## JSON output

Every command emits a stable `{ "schemaVersion": "1", "data": { … } }`
envelope under `-o json`. Example from `sonar gate`:

```json
{
  "schemaVersion": "1",
  "data": {
    "projectStatus": {
      "status": "OK",
      "conditions": [
        { "metricKey": "new_coverage", "comparator": "LT", "errorThreshold": "80", "actualValue": "83.4", "status": "OK" }
      ]
    }
  }
}
```

## Typical workflows

```bash
# Daily triage — list projects, zoom into one that failed
krci sonar list
krci sonar get payments-api
krci sonar issues payments-api --severity BLOCKER,CRITICAL

# PR review
krci sonar gate payments-api --pr 123
krci sonar issues payments-api --pr 123 --type BUG,VULNERABILITY

# Branch review — use the same branch name you see in the Sonar URL
# (?branch=<sha-or-name>), commonly a commit SHA when scans are driven by CI.
krci sonar gate payments-api --branch 29279c5b871fa4966374a7144ed40ff2a52798d2
krci sonar issues payments-api --branch main --type BUG,VULNERABILITY

# CI gate check (exit non-zero on red)
krci sonar gate payments-api -o json \
  | jq -e '.data.projectStatus.status == "OK"' >/dev/null
```
