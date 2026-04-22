# `krci pipelinerun` — e2e test cases

Covers the `pipelinerun` command group (alias `run`) and its two
subcommands `list` (alias `ls`) and `get`. Source: `pkg/cmd/pipelinerun/`
and `docs/pipelinerun.md`.

Every row is a self-contained contract a Haiku agent can execute. See
`../runner.md` for the agent brief and the **expect grammar** reference.

## Placeholders resolved per run

| Placeholder  | Meaning                                           | Example               |
|--------------|---------------------------------------------------|-----------------------|
| `{{PROJECT}}` | An existing codebase name visible to the caller. | `keycloak-operator`   |
| `{{PR}}`      | A PR number with at least one run for PROJECT.   | `336`                 |
| `{{AUTHOR}}`  | An author with at least one run.                 | `jane-doe`            |
| `{{BRANCH}}`  | A source branch with at least one run.           | `refactor-kc-client`  |
| `{{RUN_NAME}}` | A real run name (copy from `list -o json`).     | `build-…-m8z4m`       |
| `{{FAILED_RUN_NAME}}` | A run whose status is Failed.               | `review-…-a1b2c3`     |

The orchestrator fills these; the table never hard-codes them.

---

## 1. Help & discovery (env: `offline`)

Fast, idempotent, no portal — these are the first line of defence.

| ID       | Command                                 | Env     | Setup | Expect                                                                                                                  |
|----------|------------------------------------------|---------|-------|-------------------------------------------------------------------------------------------------------------------------|
| PR-H-01  | `krci pipelinerun --help`                | offline | —     | `exit=0; stdout~/Manage pipeline runs/; stdout~/^Aliases:$/; stdout~/^\s+pipelinerun, run$/; stdout~/^\s+list\s/; stdout~/^\s+get\s/` |
| PR-H-02  | `krci run --help`                        | offline | —     | `exit=0; stdout~/Manage pipeline runs/`                                                                                 |
| PR-H-03  | `krci pipelinerun list --help`           | offline | —     | `exit=0; stdout~/--project string/; stdout~/--pr int/; stdout~/--author string/; stdout~/--branch string/; stdout~/--type string/; stdout~/--status string/; stdout~/--logs/; stdout~/--reason/; stdout~/-o, --output string/; stdout~/^Aliases:$/; stdout~/^\s+list, ls$/` |
| PR-H-04  | `krci pipelinerun ls --help`             | offline | —     | `exit=0; stdout~/List pipeline runs/`                                                                                   |
| PR-H-05  | `krci pipelinerun get --help`            | offline | —     | `exit=0; stdout~/^Usage:$/; stdout~/^\s+krci pipelinerun get <name> \[flags\]$/; stdout~/--logs/; stdout~/--reason/; stdout~/-o, --output string/; stdout!~/--project/` |
| PR-H-06  | `krci run get --help`                    | offline | —     | `exit=0; stdout~/Get pipeline run details/`                                                                             |
| PR-H-07  | `krci pipelinerun`                       | offline | —     | `exit=0; stdout~/^Available Commands:$/; stdout~/^\s+list\s/; stdout~/^\s+get\s/`                                        |

## 2. Argument validation (env: `offline`)

Wrong shape of invocation must fail fast with a helpful message and a
non-zero exit. These catch cobra/pflag wiring regressions and the custom
`ValidateStringFlags` check.

| ID       | Command                                                         | Env     | Setup | Expect                                                                                           |
|----------|------------------------------------------------------------------|---------|-------|--------------------------------------------------------------------------------------------------|
| PR-V-01  | `krci pipelinerun get`                                          | offline | —     | `exit=1; stderr~/requires a pipeline run name/; stderr~/Hint: to find run names: krci pipelinerun list/` |
| PR-V-02  | `krci pipelinerun get one two`                                  | offline | —     | `exit=1; stderr~/requires a pipeline run name/`                                                  |
| PR-V-03  | `krci pipelinerun list --unknown-flag`                          | offline | —     | `exit=1; stderr~/unknown flag: --unknown-flag/`                                                  |
| PR-V-04  | `krci pipelinerun get --unknown-flag some-name`                 | offline | —     | `exit=1; stderr~/unknown flag: --unknown-flag/`                                                  |
| PR-V-05  | `krci pipelinerun list --project --pr 53`                       | offline | —     | `exit=1; stderr~/flag needs an argument: --project/`                                             |
| PR-V-06  | `krci pipelinerun list --status --pr 53`                        | offline | —     | `exit=1; stderr~/flag needs an argument: --status/`                                              |
| PR-V-07  | `krci pipelinerun list --author --type review`                  | offline | —     | `exit=1; stderr~/flag needs an argument: --author/`                                              |
| PR-V-08  | `krci pipelinerun list --branch --project app`                  | offline | —     | `exit=1; stderr~/flag needs an argument: --branch/`                                              |
| PR-V-09  | `krci pipelinerun list --type --status failed`                  | offline | —     | `exit=1; stderr~/flag needs an argument: --type/`                                                |
| PR-V-10  | `krci pipelinerun list --pr abc`                                | offline | —     | `exit=1; stderr~/invalid argument "abc" for "--pr" flag/`                                        |
| PR-V-11  | `krci pipelinerun list -o`                                      | offline | —     | `exit=1; stderr~/flag needs an argument/`                                                        |

## 3. Global flag wiring (env: `offline`)

The only persistent flag is `--portal-url`, and it must also bind to
`KRCI_PORTAL_URL`. We don't need a real portal here — any reachable URL
that returns non-2xx is enough to prove the flag landed.

| ID       | Command                                                                 | Env     | Setup                                     | Expect                                                                 |
|----------|--------------------------------------------------------------------------|---------|--------------------------------------------|------------------------------------------------------------------------|
| PR-G-01  | `krci --help`                                                           | offline | —                                          | `exit=0; stdout~/--portal-url string/`                                 |
| PR-G-02  | `krci --portal-url https://invalid.example.invalid pipelinerun list`    | offline | —                                          | `exit=1; stderr~/invalid\.example\.invalid/`                           |
| PR-G-03  | `KRCI_PORTAL_URL=https://invalid.example.invalid krci pipelinerun list` | offline | env var exported                           | `exit=1; stderr~/invalid\.example\.invalid/`                           |
| PR-G-04  | `KRCI_PORTAL_URL=https://env.example.invalid krci --portal-url https://flag.example.invalid pipelinerun list` | offline | env var exported | `exit=1; stderr~/flag\.example\.invalid/; stderr!~/env\.example\.invalid/` |

## 4. `list` — filters against a real portal (env: `portal`)

These exercise each filter flag in isolation and in combination. Every
row asserts two things: the exit code is 0 and the JSON envelope matches
the documented shape.

| ID       | Command                                                                                 | Env    | Setup                             | Expect                                                                                                   |
|----------|------------------------------------------------------------------------------------------|--------|-----------------------------------|----------------------------------------------------------------------------------------------------------|
| PR-L-01  | `krci pipelinerun list -o json`                                                          | portal | authenticated                     | `exit=0; stdout_json.pipelineRuns:exists`                                                               |
| PR-L-02  | `krci pipelinerun list --project {{PROJECT}} -o json`                                    | portal | project has runs                  | `exit=0; stdout_json.pipelineRuns.0.project={{PROJECT}}`                                                 |
| PR-L-03  | `krci pipelinerun list --project {{PROJECT}} --pr {{PR}} -o json`                        | portal | PR has runs                       | `exit=0; stdout_json.pipelineRuns.0.prNumber={{PR}}`                                                     |
| PR-L-04  | `krci pipelinerun list --author "{{AUTHOR}}" -o json`                                    | portal | author has runs                   | `exit=0; stdout_json.pipelineRuns.0.author={{AUTHOR}}`                                                   |
| PR-L-05  | `krci pipelinerun list --branch {{BRANCH}} -o json`                                      | portal | branch has runs                   | `exit=0; stdout_json.pipelineRuns.0.branch={{BRANCH}}`                                                   |
| PR-L-06  | `krci pipelinerun list --type review -o json`                                            | portal | any review run exists             | `exit=0; stdout_json.pipelineRuns.0.type=review`                                                         |
| PR-L-07  | `krci pipelinerun list --type build -o json`                                             | portal | any build run exists              | `exit=0; stdout_json.pipelineRuns.0.type=build`                                                          |
| PR-L-08  | `krci pipelinerun list --type deploy -o json`                                            | portal | any deploy run exists             | `exit=0; stdout_json.pipelineRuns:exists`                                                                |
| PR-L-09  | `krci pipelinerun list --type release -o json`                                           | portal | any release run exists            | `exit=0; stdout_json.pipelineRuns:exists`                                                                |
| PR-L-10  | `krci pipelinerun list --status succeeded -o json`                                       | portal | any succeeded run exists          | `exit=0; stdout_json.pipelineRuns.0.status=Succeeded`                                                    |
| PR-L-11  | `krci pipelinerun list --status failed -o json`                                          | portal | any failed run exists             | `exit=0; stdout_json.pipelineRuns.0.status=Failed`                                                       |
| PR-L-12  | `krci pipelinerun list --status running -o json`                                         | portal | (may be empty; pass if exit=0)    | `exit=0; stdout_json.pipelineRuns:exists`                                                                |
| PR-L-13  | `krci pipelinerun list --status timeout -o json`                                         | portal | (may be empty)                    | `exit=0`                                                                                                 |
| PR-L-14  | `krci pipelinerun list --status cancelled -o json`                                       | portal | (may be empty)                    | `exit=0`                                                                                                 |
| PR-L-15  | `krci pipelinerun list --project {{PROJECT}} --status failed --type review -o json`      | portal | combined filter has data          | `exit=0; stdout_json.pipelineRuns:exists`                                                                |
| PR-L-16  | `krci pipelinerun list --project does-not-exist-{{NONCE}} -o json`                       | portal | nonexistent project               | `exit=0; stdout~/^\{/; stdout_json.pipelineRuns:len=0` _(empty result MUST still be a JSON envelope)_    |
| PR-L-17  | `krci pipelinerun list --project does-not-exist-{{NONCE}}`                               | portal | nonexistent project, table mode   | `exit=0; stdout~/No pipeline runs found/`                                                                |

## 5. `list --logs` / `--reason` (env: `portal`)

These flags target the most recent matching run and shape the output.

| ID       | Command                                                                             | Env    | Setup                           | Expect                                                                                                  |
|----------|--------------------------------------------------------------------------------------|--------|---------------------------------|---------------------------------------------------------------------------------------------------------|
| PR-L-20  | `krci pipelinerun list --project {{PROJECT}} --pr {{PR}} --logs -o json`             | portal | PR has runs                     | `exit=0; stdout_json.pipelineRuns:exists; stdout_json.logs:exists`                                      |
| PR-L-21  | `krci pipelinerun list --project {{PROJECT}} --pr {{PR}} --reason -o json`          | portal | PR has runs                     | `exit=0; stdout_json.pipelineRuns:exists; stdout_json.tasks:exists`                                     |
| PR-L-22  | `krci pipelinerun list --project {{PROJECT}} --pr {{PR}} --logs`                     | portal | PR has runs, table mode         | `exit=0; stdout~/Logs:\s+/`                                                                             |
| PR-L-23  | `krci pipelinerun list --project {{PROJECT}} --pr {{PR}} --reason`                   | portal | PR has runs (with task data)    | `exit=0; stdout~/Tasks:/`  _(or stdout~/No task data/ for runs without steps)_                          |

## 6. Output format negotiation (env: `offline` where possible)

`-o` accepts `table` or `json`; `--help` advertises auto-detect when
absent. The flag wiring can be checked offline via a forced-failure path
(unreachable portal) but the format-specific output needs `portal`.

| ID       | Command                                                                 | Env     | Setup | Expect                                                                                  |
|----------|--------------------------------------------------------------------------|---------|-------|-----------------------------------------------------------------------------------------|
| PR-O-01  | `krci pipelinerun list -o json --project {{PROJECT}}`                   | portal  | —     | `exit=0; stdout~/^\s*\{/` _(starts with JSON object)_                                   |
| PR-O-02  | `krci pipelinerun list -o table --project {{PROJECT}}`                  | portal  | —     | `exit=0; stdout~/NAME\s+STATUS\s+PROJECT/`                                              |
| PR-O-03  | `krci pipelinerun list -o yaml --project {{PROJECT}}`                   | portal  | —     | `exit=1` _(yaml is not in the advertised list; if it silently passes, that's a bug)_    |
| PR-O-04  | `krci pipelinerun list --output json --project {{PROJECT}}`             | portal  | —     | `exit=0; stdout_json.pipelineRuns:exists`                                               |

## 7. `get <name>` (env: `portal`)

| ID       | Command                                                            | Env    | Setup                     | Expect                                                                                   |
|----------|---------------------------------------------------------------------|--------|---------------------------|------------------------------------------------------------------------------------------|
| PR-GE-01 | `krci pipelinerun get {{RUN_NAME}}`                                | portal | run exists                | `exit=0; stdout~/^Pipeline:\s+{{RUN_NAME}}$/; stdout~/^Status:\s+/`                         |
| PR-GE-02 | `krci pipelinerun get {{RUN_NAME}} -o json`                        | portal | run exists                | `exit=0; stdout_json.pipelineRuns.0.name={{RUN_NAME}}`                                   |
| PR-GE-03 | `krci pipelinerun get {{RUN_NAME}} --logs -o json`                 | portal | run exists                | `exit=0; stdout_json.pipelineRuns.0.name={{RUN_NAME}}; stdout_json.logs:exists`          |
| PR-GE-04 | `krci pipelinerun get {{FAILED_RUN_NAME}} --reason -o json`        | portal | failed run exists         | `exit=0; stdout_json.tasks:exists`                                                       |
| PR-GE-05 | `krci pipelinerun get {{RUN_NAME}} --reason`                       | portal | run exists                | `exit=0; stdout~/Tasks:/`  _(or stdout~/No task data/)_                                  |
| PR-GE-06 | `krci pipelinerun get nonexistent-run-{{NONCE}}`                   | portal | nonexistent name          | `exit=1; stderr~/pipeline run "nonexistent-run-{{NONCE}}" not found/`                    |
| PR-GE-07 | `krci run get {{RUN_NAME}}`                                        | portal | alias check               | `exit=0; stdout~/^Pipeline:\s+{{RUN_NAME}}$/`                                               |

## 8. JSON output contract (env: `portal`)

These rows nail down the shape the docs promise in `docs/pipelinerun.md`.
If the OpenAPI client drifts, these break first.

| ID       | Command                                                              | Env    | Setup      | Expect                                                                                                    |
|----------|-----------------------------------------------------------------------|--------|------------|-----------------------------------------------------------------------------------------------------------|
| PR-J-01  | `krci pipelinerun list --project {{PROJECT}} -o json`                | portal | has data   | `exit=0; stdout_json.pipelineRuns.0.name:exists; stdout_json.pipelineRuns.0.status:exists; stdout_json.pipelineRuns.0.project:exists; stdout_json.pipelineRuns.0.pipeline:exists; stdout_json.pipelineRuns.0.startTime:exists; stdout_json.pipelineRuns.0.duration:exists; stdout_json.pipelineRuns.0.type:exists` |
| PR-J-02  | `krci pipelinerun list --project {{PROJECT}} --pr {{PR}} -o json`    | portal | PR run has git metadata | `exit=0; stdout_json.pipelineRuns.0.prNumber:exists; stdout_json.pipelineRuns.0.prUrl:exists; stdout_json.pipelineRuns.0.branch:exists; stdout_json.pipelineRuns.0.commitSha:exists; stdout_json.pipelineRuns.0.author:exists` |
| PR-J-03  | `krci pipelinerun get {{RUN_NAME}} -o json`                          | portal | run exists | `exit=0; stdout_json.pipelineRuns.0.portalUrl:exists`                                                     |
| PR-J-04  | `krci pipelinerun get {{FAILED_RUN_NAME}} --reason -o json`         | portal | failed run | `exit=0; stdout_json.tasks.0.name:exists; stdout_json.tasks.0.status:exists`                              |

## 9. Auth gating (env: `auth` absent)

If the caller isn't logged in and no real data can be fetched, commands
must print a helpful hint pointing at `krci auth login`.

| ID       | Command                                      | Env      | Setup                       | Expect                                                                  |
|----------|-----------------------------------------------|----------|------------------------------|-------------------------------------------------------------------------|
| PR-A-01  | `krci pipelinerun list`                      | no-auth  | tokens removed / expired     | `exit=1; stderr~/authentication required/; stderr~/krci auth login/`    |
| PR-A-02  | `krci pipelinerun get some-run`              | no-auth  | tokens removed / expired     | `exit=1; stderr~/authentication required/; stderr~/krci auth login/`    |

> Rows in section 9 mutate/observe auth state — the orchestrator must run
> them serially, **after** all other sections, and restore the prior
> auth state when finished.

---

## Coverage checklist

Each of the following must be covered by ≥1 row above. Tick as you add.

- [x] `pipelinerun` help
- [x] `run` alias (group level)
- [x] `list` help, `ls` alias
- [x] `get` help
- [x] `get` missing arg, too many args
- [x] unknown flag (list + get)
- [x] string flag consumed by next flag (`--project --pr 53`)
- [x] `--pr` non-integer
- [x] `-o` without value
- [x] `--portal-url` flag, `KRCI_PORTAL_URL` env, precedence
- [x] `list --project`
- [x] `list --pr`
- [x] `list --author`
- [x] `list --branch`
- [x] `list --type` (review / build / deploy / release)
- [x] `list --status` (succeeded / failed / running / timeout / cancelled)
- [x] `list --logs`
- [x] `list --reason`
- [x] `list` combined filters
- [x] `list` empty result (table + JSON)
- [x] `-o json` / `-o table` / unknown `-o`
- [x] `get <name>` default
- [x] `get <name> --logs`
- [x] `get <name> --reason`
- [x] `get <name> -o json`
- [x] `get` nonexistent name
- [x] JSON envelope field contract for `list` and `get`
- [x] auth-required error path
