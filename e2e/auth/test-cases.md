# `krci auth` — e2e test cases

Covers the `auth` command group: `login`, `status`, and `logout`. Source:
`pkg/cmd/auth/` and `docs/auth.md`. This file exercises **`status`** — `login`
needs a browser and `logout` destroys the session other rows depend on, so
both stay out of the parallel run.

`krci auth status` reports the signed-in user and exits `1` without a valid
session, which makes it usable as a shell guard. `-o json` wraps the same
outcome in the `schemaVersion` envelope: `data.authenticated`, `data.user`,
`data.name`, `data.groups`, `data.expiresAt` on success, `error.message` on
failure.

Every row is a self-contained contract a Haiku agent can execute. See
`../runner.md` for the agent brief and the **expect grammar** reference.

## Placeholders resolved per run

| Placeholder      | Meaning                                                        | Example                 |
|------------------|----------------------------------------------------------------|-------------------------|
| `{{USER_EMAIL}}` | The e-mail of the account the current session belongs to.      | `jane.doe@example.com`  |

The orchestrator fills these; the table never hard-codes them.

---

## 1. Help & discovery (env: `offline`)

| ID       | Command                    | Env     | Setup | Expect                                                                                                                    |
|----------|----------------------------|---------|-------|---------------------------------------------------------------------------------------------------------------------------|
| AU-H-01  | `krci auth --help`         | offline | —     | `exit=0; stdout~/Authentication commands/; stdout~/^\s+login\s/; stdout~/^\s+status\s/; stdout~/^\s+logout\s/`           |
| AU-H-02  | `krci auth status --help`  | offline | —     | `exit=0; stdout~/Exits 1 when no session is stored/; stdout~/-o, --output string/; stdout~/krci auth status -o json/`     |

## 2. Argument validation (env: `offline`)

| ID       | Command                          | Env     | Setup | Expect                                          |
|----------|----------------------------------|---------|-------|-------------------------------------------------|
| AU-V-01  | `krci auth status -o yaml`       | offline | —     | `exit=1; stderr~/unknown output format/`        |
| AU-V-02  | `krci auth status --unknown`     | offline | —     | `exit=1; stderr~/unknown flag: --unknown/`      |
| AU-V-03  | `krci auth status extra`         | offline | —     | `exit=1; stderr~/unknown command "extra"/`      |

## 3. No session (env: `offline`)

An empty `HOME` has no `~/.config/krci/tokens.enc`, so the command sees no
session without touching the real one. `KRCI_TOKEN` must be unset for these
rows.

| ID       | Command                                                        | Env     | Setup                              | Expect                                                                                                                   |
|----------|----------------------------------------------------------------|---------|------------------------------------|--------------------------------------------------------------------------------------------------------------------------|
| AU-N-01  | `HOME=$(mktemp -d) krci auth status`                           | offline | `KRCI_TOKEN` unset                 | `exit=1; stderr~/not authenticated: run 'krci auth login'/; stdout_empty`                                                 |
| AU-N-02  | `HOME=$(mktemp -d) krci auth status -o json`                   | offline | `KRCI_TOKEN` unset                 | `exit=1; stdout_json.schemaVersion=1; stdout_json.error.message:exists; stdout~/not authenticated/; stdout!~/"data"/`            |
| AU-N-03  | `HOME=$(mktemp -d) KRCI_TOKEN=eyJhbGciOiJub25lIn0.eyJleHAiOjF9.x krci auth status -o json` | offline | JWT with `exp` = 1 | `exit=1; stdout_json.error.message~/KRCI_TOKEN has expired/; stdout!~/"data"/` |
| AU-N-04  | `HOME=$(mktemp -d) KRCI_TOKEN=not-a-token KRCI_PORTAL_URL=http://127.0.0.1:1 KRCI_CLUSTER_NAME=c KRCI_NAMESPACE=ns krci auth status -o json` | offline | nothing listens on port 1 | `exit=1; stdout_json.error.message~/verifying the token with the portal/; stdout!~/"data"/` |
| AU-N-05  | `HOME=$(mktemp -d) KRCI_TOKEN=eyJhbGciOiJub25lIn0.eyJlbWFpbCI6ImZvcmdlZEBleGFtcGxlLmNvbSIsImV4cCI6NDEwMjQ0NDgwMH0.x KRCI_PORTAL_URL=http://127.0.0.1:1 KRCI_CLUSTER_NAME=c KRCI_NAMESPACE=ns krci auth status -o json` | offline | unsigned JWT, `exp` in 2100; nothing listens on port 1 | `exit=1; stdout_json.error.message~/verifying the token with the portal/; stdout!~/"data"/` |

## 4. Valid session (env: `auth`)

Requires a session created with `krci auth login` beforehand.

| ID       | Command                          | Env  | Setup         | Expect                                                                                                                                             |
|----------|----------------------------------|------|---------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| AU-S-01  | `krci auth status`               | auth | authenticated | `exit=0; stdout~/^User:\s+{{USER_EMAIL}}$/; stdout~/^Status:\s+Authenticated$/`                                                                   |
| AU-S-02  | `krci auth status -o json`       | auth | authenticated | `exit=0; stdout_json.schemaVersion=1; stdout_json.data.authenticated=true; stdout_json.data.user={{USER_EMAIL}}; stdout_json.data.groups:exists`   |
| AU-S-03  | `krci auth status -o json`       | auth | authenticated | `exit=0; stdout_json.data.expiresAt:exists; stdout~/"expiresAt": "[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z"/`                                                    |
| AU-S-04  | `KRCI_TOKEN=not-a-token krci auth status -o json` | auth | portal URL configured by the login | `exit=1; stdout_json.error.message~/the portal rejected the token/; stdout!~/"data"/` |
| AU-S-05  | `KRCI_TOKEN=eyJhbGciOiJub25lIn0.eyJlbWFpbCI6ImZvcmdlZEBleGFtcGxlLmNvbSIsImV4cCI6NDEwMjQ0NDgwMH0.x krci auth status -o json` | auth | unsigned JWT, `exp` in 2100 | `exit=1; stdout_json.error.message~/the portal rejected the token/; stdout!~/"data"/` |
