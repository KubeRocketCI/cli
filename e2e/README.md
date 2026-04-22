# krci CLI — end-to-end test harness

This folder consolidates **all** user-facing scenarios for the `krci` CLI so
that every command, flag, alias, and output contract is exercised at least
once against a built binary.

## Goals

1. **Discover drift early.** Every documented flag in `--help`, every example
   in `docs/`, and every public JSON field is covered by at least one case.
2. **Parallelizable by construction.** Each row in `test-cases.md` is
   self-contained: given the `Command`, `Setup`, and `Expect` columns a
   single worker can run it without touching shared state.
3. **Agent-executable.** Rows are formatted so that a lightweight agent
   (Haiku) can take one row, run it, and emit a structured pass/fail report
   — no human interpretation required.

## Layout

```
e2e/
├── README.md                 — this file
├── runner.md                 — how to dispatch agents against a test file
└── <command>/
    └── test-cases.md         — one table per top-level command group
```

We use the following pattern: each top-level CLI command group owns a
folder — `pipelinerun/`, `auth/`, `project/`, `deployment/`, `sonar/`,
`version/` — and every folder contains exactly one `test-cases.md`. The
folder name matches the command group name verbatim, so
`e2e/<group>/test-cases.md` maps one-to-one to `krci <group> …`. Row IDs
inside each table follow `<GROUP-SHORT>-<SECTION>-<NN>` (e.g. `PR-H-01`
= pipelinerun / help / 01; `AU-V-03` = auth / validation / 03). Adding a
new CLI command group means creating a new folder with this shape — no
changes to `README.md` or `runner.md` required.

## How a run works

The project-scoped slash command `/e2e` wraps the whole flow. From a
Claude Code session in the repo root:

```
/e2e pipelinerun              # every row in one group
/e2e pipelinerun offline      # only offline rows
/e2e pipelinerun portal       # only portal rows (needs auth)
/e2e pipelinerun PR-GE-01     # one specific row
/e2e all                      # every row across every group under e2e/
/e2e all offline              # every offline row across every group
```

The command:

1. Builds the binary if `dist/krci` is missing.
2. Loads placeholders (`PROJECT`, `PR`, `RUN_NAME`, …) from
   `e2e/<group>/fixtures.env` — gitignored, per-developer. Copy
   `fixtures.env.example` to start.
3. Runs `./dist/krci auth status` once as a preflight for portal rows.
4. Dispatches one Haiku agent per row in parallel per
   [`runner.md`](./runner.md).
5. Aggregates the JSON verdicts and prints
   `N PASS · M FAIL · K SKIP`.

Any `FAIL` is a real deviation between what the CLI does and what the
docs (or `--help`, or the `Example:` blocks) promise.

## Environment classes

The user is responsible for configuring the target environment (portal URL,
credentials). The harness does exactly one preflight check before any
`portal` rows run:

```
./dist/krci auth status
```

If that prints `Status: Authenticated` (exit 0), all `portal` rows are
dispatched. Otherwise they're skipped with a single reason. No mocks, no
fallbacks, no cluster-URL juggling — whatever the user's config points at
is what gets tested.

| Class    | Needs                                                | Examples                       |
|----------|------------------------------------------------------|--------------------------------|
| `offline` | Built binary only. No network, no auth, no portal. | `--help`, unknown-flag errors  |
| `portal`  | `krci auth status` returns Authenticated.          | `pipelinerun list --project …` |

`offline` cases are the backbone: they run anywhere, they're fast, and they
catch the bulk of regressions (argument parsing, flag wiring, help text,
alias plumbing). `portal` cases prove the end-to-end contract.

## Adding a new command group

1. Copy `pipelinerun/test-cases.md` into `<new-group>/test-cases.md`.
2. Rewrite the rows — keep column order and ID scheme.
3. Re-run `runner.md` against the new file.
