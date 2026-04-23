# `krci sca` — e2e test cases

Covers the `sca` command group and its four subcommands `list`, `get`,
`components`, `findings`. Source: `pkg/cmd/sca/` and `docs/sca.md`.

Every row is a self-contained contract a Haiku agent can execute. See
`../runner.md` for the agent brief and the **expect grammar** reference.

## Placeholders resolved per run

| Placeholder          | Meaning                                                                   | Example             |
|----------------------|---------------------------------------------------------------------------|---------------------|
| `{{CODEBASE_OK}}`    | A codebase that has a Dep-Track project bound to its default branch.     | `payments-api`      |
| `{{CODEBASE_NONE}}`  | A codebase that exists but has no Dep-Track scanner binding.             | `infra-gitops`      |
| `{{CODEBASE_MISSING}}` | A codebase name that does not exist in the cluster.                    | `does-not-exist`    |
| `{{BRANCH}}`         | A branch/version known to Dep-Track for `{{CODEBASE_OK}}`.               | `main`              |
| `{{RELEASE_BRANCH}}` | A release tag/branch of `{{CODEBASE_OK}}` with at least one finding.     | `release/1.0`       |

The orchestrator fills these; the table never hard-codes them.

---

## 1. Help & discovery (env: `offline`)

Fast, idempotent, no portal — these are the first line of defence.

| ID         | Command                                  | Env     | Setup | Expect                                                                                                                        |
|------------|------------------------------------------|---------|-------|-------------------------------------------------------------------------------------------------------------------------------|
| SCA-H-01   | `krci sca --help`                        | offline | —     | `exit=0; stdout~/Inspect Software Composition Analysis/; stdout~/^\s+list\s/; stdout~/^\s+get\s/; stdout~/^\s+components\s/; stdout~/^\s+findings\s/` |
| SCA-H-02   | `krci sca list --help`                   | offline | —     | `exit=0; stdout~/--search string/; stdout~/--page int/; stdout~/--page-size int/; stdout~/--include-inactive/; stdout~/--include-children/; stdout~/-o, --output string/` |
| SCA-H-03   | `krci sca get --help`                    | offline | —     | `exit=0; stdout~/Dep-Track project 'version' field/; stdout~/--branch string/; stdout!~/--pr/`                                |
| SCA-H-04   | `krci sca components --help`             | offline | —     | `exit=0; stdout~/--only-outdated/; stdout~/--only-direct/; stdout~/--severity/; stdout~/inclusive/; stdout!~/--pr/`            |
| SCA-H-05   | `krci sca findings --help`               | offline | —     | `exit=0; stdout~/--include-suppressed/; stdout~/--source string/; stdout~/--severity/; stdout!~/--pr/`                         |
| SCA-H-06   | `krci sca`                               | offline | —     | `exit=0; stdout~/^Available Commands:$/; stdout~/^\s+list\s/; stdout~/^\s+get\s/; stdout~/^\s+components\s/; stdout~/^\s+findings\s/` |

## 2. Argument validation (env: `offline`)

Wrong shape of invocation must fail fast with a helpful message and a
non-zero exit.

| ID         | Command                                                             | Env     | Setup | Expect                                                                                                             |
|------------|----------------------------------------------------------------------|---------|-------|--------------------------------------------------------------------------------------------------------------------|
| SCA-V-01   | `krci sca get`                                                       | offline | —     | `exit=1; stderr~/requires a KubeRocketCI codebase name/`                                                           |
| SCA-V-02   | `krci sca components`                                                | offline | —     | `exit=1; stderr~/requires a KubeRocketCI codebase name/`                                                           |
| SCA-V-03   | `krci sca findings`                                                  | offline | —     | `exit=1; stderr~/requires a KubeRocketCI codebase name/`                                                           |
| SCA-V-04   | `krci sca get svc --pr 42`                                           | offline | —     | `exit=1; stderr~/unknown flag: --pr/`                                                                              |
| SCA-V-05   | `krci sca components svc --pr 42`                                    | offline | —     | `exit=1; stderr~/unknown flag: --pr/`                                                                              |
| SCA-V-06   | `krci sca findings svc --pr 42`                                      | offline | —     | `exit=1; stderr~/unknown flag: --pr/`                                                                              |
| SCA-V-07   | `krci sca list -o yaml`                                              | offline | —     | `exit=1; stderr~/unknown output format/`                                                                           |
| SCA-V-08   | `krci sca list --page 0`                                             | offline | —     | `exit=1; stderr~/--page must be >= 1/`                                                                             |
| SCA-V-09   | `krci sca list --page-size 1000`                                     | offline | —     | `exit=1; stderr~/--page-size must be between 1 and 500/`                                                           |
| SCA-V-10   | `krci sca components svc --severity=garbage`                         | offline | —     | `exit=1; stderr~/invalid --severity/; stderr~/CRITICAL/; stderr~/HIGH/; stderr~/UNASSIGNED/`                       |
| SCA-V-11   | `krci sca findings svc --severity=garbage`                           | offline | —     | `exit=1; stderr~/invalid --severity/`                                                                              |
| SCA-V-12   | `krci sca get Not_A_DNS_Label`                                       | offline | —     | `exit=1; stderr~/DNS-1123/`                                                                                        |
| SCA-V-13   | `krci sca --unknown-flag`                                            | offline | —     | `exit=1; stderr~/unknown flag: --unknown-flag/`                                                                    |

## 3. Happy paths (env: `portal`)

Requires `krci auth status` → Authenticated. Exercises the golden path for
each verb.

| ID         | Command                                                                | Env    | Setup | Expect                                                                                                                  |
|------------|------------------------------------------------------------------------|--------|-------|-------------------------------------------------------------------------------------------------------------------------|
| SCA-P-01   | `krci sca list`                                                        | portal | —     | `exit=0; stdout~/NAME/; stdout~/VERSION/; stdout~/CLASSIFIER/; stdout~/LAST_BOM/; stdout~/VULNS \(C\/H\/M\/L\)/`          |
| SCA-P-02   | `krci sca list -o json`                                                | portal | —     | `exit=0; stdout-json~/\.data\.items is array/; stdout-json~/\.data\.totalCount is number/; stdout-json~/\.schemaVersion == "1"/` |
| SCA-P-03   | `krci sca list --page 1 --page-size 10 --search={{CODEBASE_OK}}`       | portal | —     | `exit=0; stdout~/{{CODEBASE_OK}}/`                                                                                      |
| SCA-P-04   | `krci sca list --include-inactive --include-children -o json`          | portal | —     | `exit=0; stdout-json~/\.data\.items is array/`                                                                          |
| SCA-P-05   | `krci sca get {{CODEBASE_OK}}`                                         | portal | —     | `exit=0; stdout~/{{CODEBASE_OK}} @ /; stdout~/Classifier/; stdout~/Risk score/; stdout~/Vulnerabilities/`                |
| SCA-P-06   | `krci sca get {{CODEBASE_OK}} -o json`                                 | portal | —     | `exit=0; stdout-json~/\.data\.status == "OK"/; stdout-json~/\.data\.project\.name == "{{CODEBASE_OK}}"/`                |
| SCA-P-07   | `krci sca get {{CODEBASE_OK}} --branch={{RELEASE_BRANCH}} -o json`     | portal | —     | `exit=0; stdout-json~/\.data\.status in ["OK","NONE"]/`                                                                |
| SCA-P-08   | `krci sca components {{CODEBASE_OK}} --page-size 10`                   | portal | —     | `exit=0; stdout~/COMPONENT/; stdout~/CURRENT/; stdout~/LATEST/; stdout~/OUTDATED/; stdout~/VULNS \(C\/H\/M\/L\)/`         |
| SCA-P-09   | `krci sca components {{CODEBASE_OK}} --only-outdated -o json`          | portal | —     | `exit=0; stdout-json~/\.data\.status == "OK"/; stdout-json~/\.data\.items is array/`                                    |
| SCA-P-10   | `krci sca findings {{CODEBASE_OK}} -o json`                            | portal | —     | `exit=0; stdout-json~/\.data\.status == "OK"/; stdout-json~/\.data\.items is array/; stdout-json~/\.data\.truncated is boolean/` |

## 4. NONE state (env: `portal`)

Unbound codebases produce a `status=NONE` payload with exit `0`.

| ID         | Command                                              | Env    | Setup | Expect                                                                                       |
|------------|------------------------------------------------------|--------|-------|----------------------------------------------------------------------------------------------|
| SCA-N-01   | `krci sca get {{CODEBASE_NONE}}`                     | portal | —     | `exit=0; stdout~/status: NONE/; stdout~/no SCA scanner bound for {{CODEBASE_NONE}}/`         |
| SCA-N-02   | `krci sca get {{CODEBASE_NONE}} -o json`             | portal | —     | `exit=0; stdout-json~/\.data\.status == "NONE"/`                                             |
| SCA-N-03   | `krci sca components {{CODEBASE_NONE}} -o json`      | portal | —     | `exit=0; stdout-json~/\.data\.status == "NONE"/; stdout-json~/\.data\.items == []/`          |
| SCA-N-04   | `krci sca findings {{CODEBASE_NONE}} -o json`        | portal | —     | `exit=0; stdout-json~/\.data\.status == "NONE"/; stdout-json~/\.data\.truncated == false/`   |

## 5. Severity filter (env: `portal`)

Filter is inclusive — `--severity=high` includes HIGH + CRITICAL rows.

| ID         | Command                                                            | Env    | Setup | Expect                                                                                               |
|------------|---------------------------------------------------------------------|--------|-------|------------------------------------------------------------------------------------------------------|
| SCA-S-01   | `krci sca findings {{CODEBASE_OK}} --severity=critical -o json`     | portal | —     | `exit=0; stdout-json~/every item vulnerability.severity == "CRITICAL"/`                              |
| SCA-S-02   | `krci sca findings {{CODEBASE_OK}} --severity=high -o json`         | portal | —     | `exit=0; stdout-json~/every item vulnerability.severity in ["CRITICAL","HIGH"]/`                     |
| SCA-S-03   | `krci sca findings {{CODEBASE_OK}} --include-suppressed -o json`    | portal | —     | `exit=0; stdout-json~/\.data\.status == "OK"/`                                                       |

## 6. Truncation (env: `portal`)

Projects with more than 1000 findings should truncate + set flag.
Requires a project with a large CVE footprint — skip when `{{CODEBASE_OK}}`
has fewer than 1000 raw rows.

| ID         | Command                                                               | Env    | Setup                                  | Expect                                                                                              |
|------------|------------------------------------------------------------------------|--------|----------------------------------------|-----------------------------------------------------------------------------------------------------|
| SCA-T-01   | `krci sca findings {{CODEBASE_OK}} -o json`                            | portal | large project (>=1000 raw findings)    | `exit=0; stdout-json~/\.data\.truncated == true/; stdout-json~/\.data\.items length == 1000/`       |
| SCA-T-02   | `krci sca findings {{CODEBASE_OK}}`                                    | portal | large project                          | `exit=0; stdout~/truncated to 1000 rows/`                                                           |

## 7. Default-branch resolution (env: `portal`)

When `--branch` is omitted, the Portal reads `Codebase.spec.defaultBranch`.

| ID         | Command                                                          | Env    | Setup                                        | Expect                                                                                       |
|------------|-------------------------------------------------------------------|--------|----------------------------------------------|----------------------------------------------------------------------------------------------|
| SCA-B-01   | `krci sca get {{CODEBASE_OK}}`                                    | portal | spec.defaultBranch set                       | `exit=0; stdout~/{{CODEBASE_OK}} @ /`                                                        |
| SCA-B-02   | `krci sca get {{CODEBASE_MISSING}}`                               | portal | no Codebase CR                               | `exit=1; stderr~/codebase {{CODEBASE_MISSING}} not found/; stderr~/krci sca list --search=/` |

## 8. Error envelopes (env: `portal`)

Script-friendly error payloads when `-o json` is set.

| ID         | Command                                                          | Env    | Setup | Expect                                                                                                     |
|------------|-------------------------------------------------------------------|--------|-------|------------------------------------------------------------------------------------------------------------|
| SCA-E-01   | `krci sca get {{CODEBASE_MISSING}} -o json`                       | portal | —     | `exit=1; stdout-json~/\.schemaVersion == "1"/; stdout-json~/\.error\.message contains "not found"/`        |
| SCA-E-02   | `krci sca findings {{CODEBASE_MISSING}} -o json`                  | portal | —     | `exit=1; stdout-json~/\.error\.message contains "not found"/`                                              |
