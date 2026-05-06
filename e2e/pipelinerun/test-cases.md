# `krci pipelinerun` — e2e test cases

Covers the `pipelinerun` command group (alias `run`) and its two
subcommands `list` (alias `ls`) and `get`. Source: `pkg/cmd/pipelinerun/`
and `docs/pipelinerun.md`.

Every row is a self-contained contract a Haiku agent can execute. See
`../runner.md` for the agent brief and the **expect grammar** reference.

## Placeholders resolved per run

The orchestrator loads `fixtures.env` and substitutes every `{{NAME}}` in the
tables below before dispatching agents. Rows whose placeholders are unset are
SKIPped, not failed.

**Generic discovery placeholders** (used by `list` / `get` rows; populate from
your own portal):

| Placeholder  | Meaning                                           | Example               |
|--------------|---------------------------------------------------|-----------------------|
| `{{PROJECT}}` | A codebase name visible to the caller.           | `krci-cli-e2e`        |
| `{{PR}}`      | A PR number with at least one run for PROJECT.   | `1`                   |
| `{{AUTHOR}}`  | An author with at least one run.                 | `krci-cli-e2e-bot`    |
| `{{BRANCH}}`  | A source branch with at least one run.           | `main`                |
| `{{RUN_NAME}}` | A real run name (copy from `list -o json`).     | `krci-cli-e2e-noop-run-m8z4m` |
| `{{FAILED_RUN_NAME}}` | A run whose status is Failed.            | `krci-cli-e2e-noop-run-a1b2c` |
| `{{NONCE}}`   | Random suffix for "definitely-doesn't-exist" assertions. | `test-999zzz` |

**Start-feature placeholders** (point at the manifests in `fixtures/*.yaml`;
apply once with `kubectl apply -n <namespace> -f fixtures/`):

> Note: pipelines whose params lack defaults are submitted with `value: ""`
> (or `[]` for arrays), so "missing required param" is not a reachable
> failure mode through this client — do not write rows expecting one.

| Placeholder                          | Meaning                                                  | Example                       |
|--------------------------------------|----------------------------------------------------------|-------------------------------|
| `{{PIPELINE_OK}}`                    | Pipeline that runs cleanly with all-defaulted params.    | `krci-cli-e2e-noop`           |
| `{{PIPELINE_REQUIRED_PARAM}}`        | Pipeline declaring a required param (no default).        | `krci-cli-e2e-required`       |
| `{{PIPELINE_REQUIRED_PARAM_NAME}}`   | Name of that required param.                             | `message`                     |
| `{{PIPELINE_BROKEN_TT}}`             | Pipeline whose TT label points at a missing TT.          | `krci-cli-e2e-broken-tt`      |
| `{{PIPELINE_BARE}}`                  | Tekton Pipeline carrying no KRCI labels at all.          | `krci-cli-e2e-bare`           |
| `{{PIPELINE_WITH_TT}}`               | Pipeline paired with a real, resolvable TriggerTemplate. | `krci-cli-e2e-with-tt`        |
| `{{TRIGGER_TEMPLATE}}`               | The TT referenced by `{{PIPELINE_WITH_TT}}`.             | `krci-cli-e2e-tt`             |
| `{{PARAM_KNOWN_KEY}}`                | Param accepted by `{{PIPELINE_OK}}`.                     | `git-revision`                |
| `{{PARAM_KNOWN_VALUE}}`              | Override value for `{{PARAM_KNOWN_KEY}}`.                | `develop`                     |
| `{{PIPELINE_LABEL_KEY}}`             | Codebase label key used by `--label`.                    | `app.edp.epam.com/codebase`   |
| `{{PIPELINE_LABEL_VALUE}}`           | Codebase label value used by `--label`.                  | `krci-cli-e2e`                |

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

### `pipelinerun start`

Source: `pkg/cmd/pipelinerun/start/start.go` and `internal/portal/start.go`.
Fixtures: `fixtures/*.yaml` (Pipelines + TriggerTemplate); see
`fixtures/README.md` for the apply procedure.

| ID                  | Class      | Run as  | Title                                                                                                                                                                | Expect                                                                                                                |
|---------------------|------------|---------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------|
| PR-S-HELP           | help       | offline | `start --help` lists `--param`, `--label`, `--dry-run`, `-o`                                                                                                          | exit 0; help text mentions `escape hatch`                                                                             |
| PR-S-DNS-1          | validation | offline | reject uppercase positional `Foo_Build`                                                                                                                              | exit 1; stderr `must be a valid DNS-1123 name`                                                                        |
| PR-S-OUT-1          | validation | offline | reject `-o xml`                                                                                                                                                      | exit 1; stderr `unknown output format`                                                                                |
| PR-S-DRY-MUTEX      | validation | offline | reject `--dry-run -o table`                                                                                                                                          | exit 1; stderr `--dry-run cannot use -o table`                                                                        |
| PR-S-PARAM-DUP      | validation | offline | reject `--param k=v1 --param k=v2`                                                                                                                                   | exit 1; stderr `duplicate parameter 'k'`                                                                              |
| PR-S-PARAM-EMPTY    | validation | offline | reject `--param =value`                                                                                                                                              | exit 1; stderr `parameter key must not be empty`                                                                      |
| PR-S-PARAM-MAL      | validation | offline | reject `--param keywithoutvalue`                                                                                                                                     | exit 1; stderr `parameter must be key=value`                                                                          |
| PR-S-LABEL-DUP      | validation | offline | reject `--label k=v1 --label k=v2`                                                                                                                                   | exit 1; stderr `duplicate label 'k'`                                                                                  |
| PR-S-PARAM-EQ       | parser     | offline | accept `--param token=abc=def==` (split on first `=`)                                                                                                                | exit 0 (capture); param `token=abc=def==`                                                                             |
| PR-S-PARAM-WS       | parser     | offline | accept `--param "  k  =  v  "` (whitespace trimmed)                                                                                                                  | exit 0 (capture); param `k=v`                                                                                         |
| PR-S-1              | happy      | portal  | `start {{PIPELINE_OK}}` (no params)                                                                                                                                  | exit 0; stdout row with columns `NAME, STATUS, PROJECT, PR, AUTHOR, TYPE, STARTED, DURATION`                          |
| PR-S-2              | happy      | portal  | `start {{PIPELINE_OK}} --param {{PARAM_KNOWN_KEY}}={{PARAM_KNOWN_VALUE}}`                                                                                            | exit 0; resulting PipelineRun's `spec.params` carries the override                                                    |
| PR-S-3              | happy      | portal  | `start {{PIPELINE_OK}} -o json`                                                                                                                                      | exit 0; stdout `{"schemaVersion":"1","data":{"name":"...","status":"...",...}}` (StartResult fields, flat — no `row`) |
| PR-S-LABEL          | happy      | portal  | `start {{PIPELINE_OK}} --label {{PIPELINE_LABEL_KEY}}={{PIPELINE_LABEL_VALUE}}`; then `pipelinerun list --project {{PIPELINE_LABEL_VALUE}}`                          | exit 0 from both; the new run is discoverable in the list output                                                      |
| PR-S-FAIL           | happy      | portal  | `start {{PIPELINE_OK}} --param should-fail=true -o json`                                                                                                             | exit 0; `data.name` populated. Run will eventually fail — useful as deterministic seed for `FAILED_RUN_NAME`.         |
| PR-S-BARE           | happy      | portal  | `start {{PIPELINE_BARE}}` (Pipeline carries zero KRCI labels)                                                                                                        | exit 0; standard row; proves CLI does not require KubeRocketCI labelling convention                                   |
| PR-S-TT-OK          | happy      | portal  | `start {{PIPELINE_WITH_TT}} -o json`; then read back `kubectl get pipelinerun <data.name> -o jsonpath='{.metadata.labels.test\.krci-cli-e2e/seeded-message}'`         | exit 0; label value equals `from-pipeline-default` (proves TriggerTemplate placeholder resolution ran)                |
| PR-S-DRY-YAML       | dry-run    | portal  | `start {{PIPELINE_OK}} --dry-run` (default → YAML manifest)                                                                                                          | exit 0; stdout valid YAML containing `metadata.generateName: {{PIPELINE_OK}}-run-`                                    |
| PR-S-DRY-JSON       | dry-run    | portal  | `start {{PIPELINE_OK}} --dry-run -o json`                                                                                                                            | exit 0; stdout `{"schemaVersion":"1","data":{...PipelineRun manifest...}}`; `data.metadata.generateName` starts with `{{PIPELINE_OK}}-run-` |
| PR-S-TT-DRY         | dry-run    | portal  | `start {{PIPELINE_WITH_TT}} --dry-run -o json`                                                                                                                       | exit 0; `data.metadata.labels."test.krci-cli-e2e/seeded-message"` equals `from-pipeline-default` (resolved before K8s submission) |
| PR-S-NOT-FOUND      | error      | portal  | `start ghost` (no such Pipeline)                                                                                                                                     | exit 1; stderr `pipeline 'ghost' not found`                                                                           |
| PR-S-TT-MISSING     | error      | portal  | `start {{PIPELINE_BROKEN_TT}}`                                                                                                                                       | exit 1; stderr `references a TriggerTemplate that does not exist`                                                     |
| PR-S-PARAM-SYNTHESIZED | regression | portal | `start {{PIPELINE_REQUIRED_PARAM}}` (omit `--param {{PIPELINE_REQUIRED_PARAM_NAME}}=...`); then read back `kubectl get pipelinerun <data.name> -o jsonpath='{.spec.params[0].value}'` | exit 0 from start; the synthesized param value is the empty string. Documents the portal's "always submit a complete manifest" behavior — admission cannot reject for missing required params via this client. |
| PR-S-COL-EQ         | regression | portal  | header row of `start {{PIPELINE_OK}}` matches header row of `pipelinerun list` byte-for-byte                                                                          | identical header strings                                                                                              |
| PR-S-RACE           | edge       | portal  | immediately after `start {{PIPELINE_OK}}`, run `pipelinerun get <data.name>`                                                                                         | start exit 0 with stderr note `controller may briefly 404`; the subsequent `get` may briefly 404                       |
| PR-S-GENNAME        | regression | portal  | `start {{PIPELINE_OK}} -o json`; assert `data.name` matches `^{{PIPELINE_OK}}-run-[a-z0-9]{5}$`                                                                       | name shape conforms (`generateName` round-trip)                                                                       |

#### Coverage notes

- **Validation rows** (`PR-S-DNS-1` through `PR-S-PARAM-WS`) run offline and only need a built `dist/krci` — no portal, no fixtures.
- **Portal rows** require the manifests in `fixtures/*.yaml` to be applied to the cluster, and the fixture-specific placeholders (`PIPELINE_OK`, `PIPELINE_REQUIRED_PARAM`, `PIPELINE_BROKEN_TT`, `PIPELINE_BARE`, `PIPELINE_WITH_TT`, etc.) to be set in `fixtures.env`. Apply with `kubectl apply -n <namespace> -f fixtures/`; copy `fixtures.env.example` and fill in the values for your namespace.
- **Cluster-side assertions** (`PR-S-TT-OK`'s seeded-label check) require the agent to read the resulting PipelineRun back from K8s after `start` returns — the CLI does not surface labels in its output. Use `kubectl` against the same namespace the start used.
- **All `start` errors exit with code 1.** `cmd/krci/main.go` does not map error types to distinct codes; differentiation is via stderr text only. Earlier "exit 2" / "exit 3" rows have been corrected.
- **No "missing required param" admission rejection is reachable via this client.** The portal's `createPipelineRunDraftFromPipeline` synthesizes `value: ""` (or `[]` for arrays) for every Pipeline-declared param without a default, so the K8s apiserver always sees a complete `spec.params` list. PR-S-PARAM-SYNTHESIZED documents this property; we removed the earlier PR-S-PARAM-MISSING row that asserted the opposite. The CLI's 422 → `ErrPlatformReject` mapping is still exercised by other admission failure classes (resource quota, conflicting names, unrelated webhook rejections) — covered by `internal/portal/start_test.go`.
- **Out of scope for e2e, covered by unit tests:** `PR-S-RBAC` (HTTP 403 → `permission denied`) and `PR-S-5XX` (HTTP 502/503/500/504 → `upstream service unavailable`) require infrastructure (low-privilege user, fault-injecting proxy) that the self-contained fixture suite does not provision. The mappings live in `internal/portal/start.go:checkStartResponse` and are validated by `internal/portal/start_test.go`.
