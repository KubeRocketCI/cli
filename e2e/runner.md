# Agent runner protocol

Each row in a `test-cases.md` table is an **independent unit of work**. The
orchestrator dispatches one Haiku agent per row in parallel; every agent
gets exactly the same brief shape, runs the command, and returns a
structured verdict.

## Dispatch

From the CLI repo root:

```bash
make build
export KRCI_BIN="$(pwd)/dist/krci"

# Preflight — only needed for portal rows. One call per batch, not per row.
"$KRCI_BIN" auth status
```

The user owns portal configuration (`--portal-url`, token file). The
harness never logs in, logs out, or mutates config. If `auth status` does
not return 0 with `Status: Authenticated`, all portal rows are marked SKIP
with reason `auth preflight failed`.

For each row in `e2e/<group>/test-cases.md`, spawn one agent with:

- **subagent_type**: `general-purpose`
- **model**: `haiku`
- **prompt**: the template below, with `{{…}}` placeholders filled from the
  row.

Run independent rows in a single message (multiple Agent tool calls in one
response) so they execute concurrently.

## Agent brief template

```
You are an end-to-end tester for the krci CLI. Execute exactly one test case
and return a JSON verdict. Do not interpret, explain, or "fix" anything —
run the command, compare to the expectation, report.

## Test case

- id:       {{ID}}
- command:  {{COMMAND}}
- env:      {{ENV_CLASS}}          # offline | auth | portal
- setup:    {{SETUP}}
- expect:   {{EXPECT}}              # the assertion contract — see below

## How to run

1. Use the binary at $KRCI_BIN. Never rebuild, never `go run`.
2. Execute the command exactly as given, including quoting.
3. Capture stdout, stderr, and the exit code **of the CLI itself**, not
   of any pipeline. Write stdout/stderr to separate temp files, then
   check `$?` **immediately** after the CLI command — never after `head`,
   `jq`, or any other filter:
   ```bash
   "$KRCI_BIN" <args> >/tmp/out.$$ 2>/tmp/err.$$; rc=$?
   ```
4. Apply the `expect` rules (see `## Expect grammar` below).

## Expect grammar

The expect field is a semicolon-separated list of predicates. All must hold.

| Predicate                          | Meaning                                     |
|------------------------------------|---------------------------------------------|
| exit=<N>                           | exit code equals N                          |
| stdout~/<regex>/                   | stdout matches regex (see **Regex rules**)  |
| stdout!~/<regex>/                  | stdout does NOT match regex                 |
| stderr~/<regex>/                   | stderr matches regex                        |
| stderr!~/<regex>/                  | stderr does NOT match regex                 |
| stdout_json.<jsonpath>=<literal>   | stdout parses as JSON and field equals      |
| stdout_json.<jsonpath>!=<literal>  | stdout parses as JSON and field differs     |
| stdout_json.<jsonpath>:exists      | jsonpath key exists (any value, incl. null) |
| stdout_json.<jsonpath>:len=<N>     | array at jsonpath has exactly N elements    |
| stdout_empty                       | stdout is zero bytes                        |
| duration_lt=<N>s                   | command finished in under N seconds         |

### Regex rules (important — most false failures start here)

- Regexes are **line-oriented**. They must match on a single line. Evaluate
  with `grep -E`, never with `grep -Pz` or input flattening. If a
  predicate needs to assert that two strings appear on consecutive lines,
  split it into two separate predicates.
- `^` and `$` anchor to line start/end, as usual for `grep -E`.
- `\s` is not POSIX; use `[[:space:]]` when you need whitespace, but
  remember this still does **not** cross newlines in line-oriented mode.
- Test-case authors: when the CLI prints `Usage:\n  krci foo`, write it
  as two predicates — `stdout~/^Usage:$/` AND `stdout~/^\s+krci foo/` —
  rather than trying to span the newline.
- JSON predicates are evaluated after `jq` parses stdout. Use dotted
  paths (`pipelineRuns.0.status`). Literals on the RHS are compared as
  strings unless they parse as numbers.

## Output

Return **only** the JSON object below and nothing else. No prose before
it, no prose after it, no markdown code fence, no leading newline. The
first byte of your final message must be `{`.

```
{
  "id":       "{{ID}}",
  "verdict":  "PASS" | "FAIL" | "SKIP",
  "command":  "<command as executed>",
  "exit":     <int>,
  "duration": "<e.g. 0.42s>",
  "failed":   [ "<predicate>", ... ],
  "stdout":   "<first 2 KiB, trimmed>",
  "stderr":   "<first 2 KiB, trimmed>",
  "skip_reason": "<only when verdict=SKIP>"
}
```

Rules:
- `failed` is always present; `[]` on PASS, non-empty on FAIL, `[]` on SKIP.
- `skip_reason` is present only when `verdict=SKIP`.
- Use verdict=SKIP if the env class is not satisfied (e.g. env=portal but
  the portal is unreachable, or env=auth but `krci auth status` shows
  Unauthenticated). Do NOT invent data or mock responses.
- Never explain your reasoning in the output. The verdict and the
  `failed` array are the entire report.
```

## Orchestrator responsibilities

- **Parse** `test-cases.md` — each non-header row is one work item.
- **Pre-flight** the env class once per batch (`krci auth status` for
  portal rows) so workers can short-circuit to SKIP deterministically.
- **Substitute** placeholders in agent briefs:
  - `{{PROJECT}}`, `{{PR}}`, `{{RUN_NAME}}` come from the batch config
    (not the table) so that the same table drives different environments.
- **Sanitize agent output** — haiku agents sometimes leak prose before
  the JSON despite the strict "first byte must be `{`" rule. Before
  parsing, strip everything up to the last line that begins with `{`.
  If that still fails to parse, mark the row `AGENT_PROTOCOL_ERROR`.
- **Re-run every FAIL once** — under load haiku occasionally hallucinates
  a PASS (reports `grep matched` for regexes that would return non-zero).
  For defense in depth: after collecting verdicts, re-run every FAIL
  *and every PASS whose predicates include a regex or ANSI-sensitive
  match* via a deterministic shell harness (single bash script, no LLM).
  Any disagreement between agent and shell counts as a `FAIL`.
- **Aggregate** the JSON verdicts into a summary:
  `N pass · M fail · K skip · L protocol_error` + a table of failures
  with the first failing predicate and a stderr excerpt.
- **Never** mark a row as passing because of a SKIP.

## Determinism rules

- Commands must not mutate user state. `auth login`, `auth logout`, and
  anything that writes to `~/.config/krci/` is out of scope for these
  tables — cover those in a dedicated, serial suite.
- Time-dependent output (relative timestamps like `1h ago`) is matched
  with loose regexes, not literal strings.
- Name-dependent output (an actual pipeline run name) is always
  substituted through `{{RUN_NAME}}` — never hard-code real names.
