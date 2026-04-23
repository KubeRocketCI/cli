# `krci sca` — Software Composition Analysis from the terminal

Inspect the Dependency-Track (DT) state surfaced by the KubeRocketCI Portal —
projects, dependencies, and vulnerability findings — without leaving the
terminal. The CLI reuses the Dep-Track binding already configured on the
Portal (`DEPENDENCY_TRACK_URL` / `DEPENDENCY_TRACK_API_KEY`), so no extra
credentials are required.

## Subcommands

| Command                          | Purpose                                                                   |
|----------------------------------|---------------------------------------------------------------------------|
| `sca list`                       | List SCA projects known to Dependency-Track                               |
| `sca get <codebase>`             | Project overview: risk score, severity counts, last BOM import           |
| `sca components <codebase>`      | Dependencies with outdated / direct / severity filters                    |
| `sca findings <codebase>`        | Flat vulnerability listing (CVE-level) with severity filter + truncation |

All commands accept `-o, --output` with `table` (default) or `json`.

### Codebase + branch addressing

Every per-codebase verb takes a positional `<codebase>` and an optional
`--branch`. When `--branch` is omitted, the Portal reads
`Codebase.spec.defaultBranch` from Kubernetes and uses that value as the
Dep-Track project "version". You can also supply an explicit branch name or
release tag.

### Severity filter (inclusive)

`--severity` accepts case-insensitive values `critical | high | medium | low | info`.
The filter is **inclusive**: `--severity=high` returns rows at HIGH or above;
`--severity=medium` returns CRITICAL + HIGH + MEDIUM. `INFO` implicitly
includes `UNASSIGNED`.

## `sca list`

```bash
krci sca list
```

```
NAME             VERSION    CLASSIFIER     ACTIVE   LAST_BOM      RISK   VULNS (C/H/M/L)
payments-api     main       APPLICATION    yes      2h ago        12.5   1/3/8/2
loyalty-api      develop    APPLICATION    yes      6h ago        0.0    0/0/0/0

2 projects, page 1 of 1 (page-size 25)
```

Flags: `--search`, `--page`, `--page-size` (max 500), `--include-inactive`,
`--include-children`. Defaults exclude inactive projects and child versions
to match the Portal UI.

Pick dirty projects from a script:

```bash
krci sca list -o json | jq -r '.data.items[] | select(.metrics.critical>0) | .name'
```

## `sca get`

```bash
krci sca get payments-api
```

```
payments-api @ main

Codebase         payments-api
Branch           main
Classifier       APPLICATION
Active           yes
Is latest        yes
Risk score       12.5
Last BOM         2026-04-18 09:12 UTC (CycloneDX 1.4), 2h ago
Last vuln scan   1h ago

Vulnerabilities:
  Critical          1
  High              3
  Medium            8
  Low               2
  Info/Unassigned   0
  Total             14

Components:
  Total        120
  Vulnerable   12
```

Target an explicit branch or pull a single metric:

```bash
krci sca get payments-api --branch=release/1.0
krci sca get payments-api -o json | jq -r '.data.project.riskScore'
```

When the codebase has no Dep-Track binding:

```bash
$ krci sca get infra-gitops
status: NONE — no SCA scanner bound for infra-gitops @ main
```

`status=NONE` always exits `0` — scripting consumers can branch on
`.data.status`.

## `sca components`

```bash
krci sca components payments-api --only-outdated
```

```
COMPONENT                CURRENT    LATEST    OUTDATED   LICENSE      RISK   VULNS (C/H/M/L)
log4j-core               2.11.2     2.24.0    yes        Apache-2.0   9.8    1/0/0/0
commons-text             1.9        1.13.1    yes        Apache-2.0   5.5    0/1/0/0

2 components, page 1 of 1 (page-size 50)
```

Combined filters are **AND** — server-side `--only-outdated` / `--only-direct`
are forwarded to Dep-Track, and `--severity` narrows client-side afterwards:

```bash
# Outdated direct dependencies with at least one HIGH or CRITICAL finding
krci sca components payments-api --only-direct --only-outdated --severity=high

# Explicit branch
krci sca components payments-api --branch=release/1.0
```

## `sca findings`

```bash
krci sca findings payments-api --severity=high
```

```
COMPONENT       VERSION    VULN_ID          SRC   SEVERITY   CVSS        ANALYSIS   SUPPRESSED
log4j-core      2.11.2     CVE-2021-44228   NVD   CRITICAL   10.0 (v3)   NOT_SET    no
commons-text    1.9        CVE-2022-42889   NVD   CRITICAL   9.8 (v3)    NOT_SET    no
```

Sort: severity desc → component.name asc → vulnerability.vulnId asc.
Default excludes suppressed findings (`--include-suppressed` to opt in).

Filter by upstream vulnerability source (e.g. `NVD`, `GITHUB`, `OSV`):

```bash
krci sca findings payments-api --source=NVD
```

Very large projects are capped server-side at 1000 rows with a footer hint:

```
(findings truncated to 1000 rows — narrow the query via --severity or --source)
```

Script-friendly CVE extraction:

```bash
krci sca findings payments-api -o json | jq -r '.data.items[].vulnerability.vulnId'
```

## Exit codes

| Exit | Meaning                                                                   |
|------|---------------------------------------------------------------------------|
| `0`  | Command succeeded (including `status=NONE` payloads)                      |
| `1`  | Any failure — validation, auth, not-found, upstream unavailable, bad flag |

## JSON output

Every command emits a stable `{ "schemaVersion": "1", "data": { … } }`
envelope under `-o json`. See `docs/json-schemas.md` for the exact payload
for each verb.

## Typical workflows

```bash
# Find the dirtiest projects
krci sca list -o json \
  | jq -r '.data.items | sort_by(-(.riskScore // 0)) | .[0:5][] | "\(.name)@\(.version)  risk=\(.riskScore // 0)"'

# Zoom into one
krci sca get payments-api
krci sca components payments-api --only-outdated
krci sca findings payments-api --severity=high

# CI gate — fail when CRITICALs exist on main
if [ "$(krci sca findings payments-api --severity=critical -o json \
        | jq '.data.items | length')" -gt 0 ]; then
  echo "new criticals"; exit 1
fi
```
