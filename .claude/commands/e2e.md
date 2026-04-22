---
description: Run e2e tests for a krci command group via haiku agents
argument-hint: [group|all] [offline|portal|ROW_ID]
allowed-tools: Read, Write, Grep, Glob, Agent, AskUserQuestion, Bash(ls:*), Bash(test:*), Bash(cat:*), Bash(grep:*), Bash(find:*), Bash(sed:*), Bash(sort:*), Bash(head:*), Bash(jq:*), Bash(date:*), Bash(echo:*), Bash(make:*), Bash(./dist/krci:*), Bash(dist/krci:*)
---

<!--
Usage:
  /e2e pipelinerun               # every row in one group
  /e2e pipelinerun offline       # only offline rows in one group
  /e2e pipelinerun PR-GE-01      # one specific row
  /e2e all                       # every row across every group
  /e2e all offline               # every offline row across every group
  /e2e all portal                # every portal row across every group
Prerequisites:
  - dist/krci (built via `make build` on demand if missing)
  - krci auth status == Authenticated (for portal rows)
  - e2e/<group>/fixtures.env per group — copy fixtures.env.example

Tip: invoke this command from a Sonnet session — planning, fixture
bootstrap, and JSON aggregation benefit from the headroom. (Worker
agents are Haiku; see step 4.)
-->

# /e2e — run the e2e test harness for one command group

You orchestrate the e2e suite defined in `e2e/<group>/test-cases.md`. Pick
the rows matching the user's filter, dispatch **one Haiku agent per row in
parallel** per `e2e/runner.md`, aggregate the JSON verdicts, print a
summary. Never run the test commands yourself — that is what the agents
are for.

## Inputs

- group filter: `$1` — either a folder name under `e2e/` (e.g. `pipelinerun`) or the literal word `all`
- row/env filter (optional): `$2`

## Preflight context (read-only, gathered automatically)

Available groups (any directory under `e2e/` that has a `test-cases.md`):
!`find e2e -mindepth 2 -maxdepth 2 -name test-cases.md | sed 's|e2e/||; s|/test-cases.md||' | sort`

Binary presence and version:
!`test -x dist/krci && ./dist/krci --version || echo "MISSING_BINARY"`

Auth status (only consulted when portal rows are selected):
!`./dist/krci auth status 2>&1 | head -5 || echo "AUTH_FAILED"`

## What to do

1. **Resolve the group set** from `$1`:
   - `all` → every group listed in the "Available groups" preflight.
   - any other value → a single group; verify it appears in the list.
     If not, print the list and exit.

2. If the binary preflight reports `MISSING_BINARY`, run `make build` once
   (explicit Bash tool call — it's a deliberate mutation, not a passive
   context probe). Fail the run if the build fails.

3. For each group in the resolved set:
   a. Read `e2e/<group>/test-cases.md`.
   b. Select rows by `$2`:
      - empty → all rows
      - `offline` → rows whose `Env` column is `offline`
      - `portal` → rows whose `Env` column is `portal`
      - a row ID like `PR-GE-01` → exactly that row (only valid when
        `$1` is a single group, not `all` — reject otherwise)
      - anything else → explain valid filters and exit
   c. Load fixtures from `e2e/<group>/fixtures.env`. If the file is
      missing **and** at least one selected row references a
      `{{PLACEHOLDER}}`, **bootstrap a candidate**:
      - Skip bootstrap entirely when no portal rows are selected, or
        when the auth preflight is not Authenticated.
      - Run the read-only discovery recipe in §"Fixture discovery" for
        this group. Treat every query as best-effort — a `null` / empty
        / failed query just drops that variable from the proposal.
      - Print the proposed `fixtures.env` as a fenced code block with
        the file path above it, then call `AskUserQuestion` with
        choices `Approve / Reject`.
        - **Approve** → `Write` the file verbatim and continue.
        - **Reject** → continue without writing; placeholder rows will
          SKIP.
      - Never edit values silently; never invent data for variables
        the portal didn't yield.
   d. Substitute `{{PLACEHOLDER}}` values from the loaded fixtures into
      each selected row's command and expect predicates.
   e. If a placeholder is still unset, mark the row `SKIP` with reason
      `fixture {{NAME}} not set` — do not dispatch.
   f. If the row has `Env = portal` and the auth preflight shows
      anything other than `Status: Authenticated`, mark it `SKIP`
      with reason `auth preflight failed`.

4. After iterating all groups, dispatch **every surviving row across
   every group in a single assistant turn** as parallel Agent tool
   calls. Row IDs already encode the group (e.g. `PR-GE-01` =
   pipelinerun; `AU-V-03` = auth), so there's no collision risk.
   - `subagent_type: general-purpose`
   - `model: haiku`
   - brief built from the template in `e2e/runner.md` §"Agent brief
     template" — include ID, the substituted command, the expect
     predicates, the strict JSON output rule (first byte must be `{`)

5. Collect each agent's reply. Before parsing JSON, strip every line
   up to the last line that starts with `{` — Haiku occasionally leaks
   prose despite the rule. If parsing still fails, classify that row
   `AGENT_PROTOCOL_ERROR`.

## Output format

Exactly three (or four, for multi-group runs) sections, nothing else:

1. **Headline** — one line, totals across all groups:
   `<N> PASS · <M> FAIL · <K> SKIP · <P> PROTOCOL_ERROR`

2. **Per-group breakdown** — only when the resolved group set has more
   than one group. Markdown table with columns `group`, `PASS`, `FAIL`,
   `SKIP`, `PROTOCOL_ERROR`.

3. **Failures table** — omit if `M + P == 0`. Markdown table with
   columns `ID`, `first failing predicate`, `stderr excerpt (≤120 chars)`.

4. **Skip ledger** — omit if `K == 0`. Markdown table with columns
   `ID`, `reason`.

A red row is a bug to fix, not a state to catalogue. Never dump the full
stdout/stderr of every agent. Never invent values to hide a SKIP. Never
run CLI test commands yourself outside preflight.

## Fixture discovery

When `fixtures.env` is missing, run these **read-only** queries to
propose candidate values. Each query is best-effort: `null` / missing
output means "leave this variable out of the proposal". Only use live
portal data — never fabricate.

### pipelinerun

```bash
# One call gives PROJECT + PR + AUTHOR + BRANCH from the first complete row.
./dist/krci pipelinerun list -o json \
  | jq -r '.pipelineRuns
      | map(select(.project != "" and .prNumber != null
                   and .branch != null and .author != null))
      | .[0]
      | "PROJECT=\(.project)\nPR=\(.prNumber)\nAUTHOR=\(.author)\nBRANCH=\(.branch)"'

# Most recent successful and failed runs for get-by-name / --reason rows.
./dist/krci pipelinerun list --status succeeded -o json \
  | jq -r '.pipelineRuns[0].name // empty'
./dist/krci pipelinerun list --status failed -o json \
  | jq -r '.pipelineRuns[0].name // empty'

# NONCE is for negative-path rows (nonexistent name); a fresh timestamp
# is sufficient — no portal call needed.
echo "test-$(date +%s)"
```

Emit the proposal in this shape before calling `AskUserQuestion`:

```text
Proposed e2e/pipelinerun/fixtures.env (from live portal):

    PROJECT=<discovered>
    PR=<discovered>
    AUTHOR=<discovered>
    BRANCH=<discovered>
    RUN_NAME=<discovered>
    FAILED_RUN_NAME=<discovered>
    NONCE=test-<epoch>
```

If discovery yields nothing for a variable, omit that line entirely —
rows that reference it will SKIP with `fixture {{NAME}} not set`.
