# KubeRocketCI CLI

| :heavy_exclamation_mark: Please refer to the [KubeRocketCI documentation](https://docs.kuberocketci.io/) to get the notion of the main concepts and guidelines. |
| --- |

`krci` is the command-line companion for the [KubeRocketCI](https://kuberocketci.io)
platform — inspect projects, deployments, pipeline runs, and SonarQube state
straight from the terminal, with JSON output ready for agent workflows.

## From chat to fix — CLI + AI agent

`krci` is built to pair with an AI coding assistant (Claude Code, Cursor,
Copilot CLI, etc.). Every data command emits predictable JSON under
`-o json`, so an agent can **discover** resources by filter (no hard-coded
names) and then **drill in** — a list → get chain the agent plans on its
own from your natural-language question.

### *"Check all my PRs that have failed and analyze the logs"*

```bash
# 1. Discover failed runs for the author — no run names needed up front
krci run list --author "jane" --status failed -o json

# 2. For each name in .pipelineRuns[].name, fan out a diagnosis:
#    task tree + failed step + relevant log lines
krci run get <name> --reason -o json
```

### *"What's the reason the pipeline is failing on PR 44?"*

```bash
# --reason on list targets the most recent matching run automatically,
# so the agent never has to know the run name
krci run list --project my-app --pr 44 --reason -o json
```

### *"Check quality gates for each of my projects"*

```bash
# sonar list already carries the per-project quality-gate verdict
krci sonar list -o json | jq '.data.projects[] | {key, qualityGateStatus}'

# Drill into any red ones
krci sonar gate <key> -o json
krci sonar issues <key> --severity BLOCKER,CRITICAL -o json
```

### *"Is `keycloak-operator` healthy right now?"*

```bash
# One-shot snapshot: measures + gate + top offenders
krci sonar get keycloak-operator -o json
krci sonar issues keycloak-operator --severity BLOCKER,CRITICAL -o json
```

### Target state

Questions we want the agent to answer next, keeping the same
list-then-drill pattern:

```bash
# "On which environments is version 1.4.2 of payments-api deployed?"
# "Which deployments include the payments-api application?"
# "Show me the promotion history for my-pipeline."
```

Why this works: the CLI only talks to the KubeRocketCI Portal (no direct
cluster access), tokens stay encrypted on disk, and the JSON shapes are
stable enough to treat `krci` as a first-class agent tool.

## What it does

| Area          | What you get                                                                     | Docs                                      |
|---------------|----------------------------------------------------------------------------------|-------------------------------------------|
| Authentication | OIDC + PKCE browser flow, AES-256-GCM token storage, OS keyring integration      | [`docs/auth.md`](docs/auth.md)            |
| Projects       | List and inspect `Codebase` resources                                            | [`docs/project.md`](docs/project.md)      |
| Deployments    | Inspect `CDPipeline` resources, their apps, environments, and promotion gates    | [`docs/deployment.md`](docs/deployment.md)|
| Pipeline runs  | List, filter, stream logs, and diagnose failures across Tekton runs              | [`docs/pipelinerun.md`](docs/pipelinerun.md) |
| SonarQube      | Projects, quality gates, measures, and issues via the Portal's Sonar binding     | [`docs/sonar.md`](docs/sonar.md)          |

Every data command accepts `-o table` (default) or `-o json`. Tables render
nicely on a TTY, plain text into pipes, JSON for automation.

## Install

```bash
brew tap KubeRocketCI/homebrew-tap
brew install krci
```

Or grab a binary from [GitHub Releases](https://github.com/KubeRocketCI/cli/releases).

## 30-second tour

```bash
# Sign in
krci auth login --portal-url https://portal.example.com

# See what's there
krci project list
krci deployment list
krci pipelinerun list --project keycloak-operator

# Diagnose a failing run
krci run list --project keycloak-operator --pr 336 --reason

# Check a SonarQube gate
krci sonar gate keycloak-operator
```

Each command has a `--help` page. Deeper walk-throughs live in `docs/` —
start with the area you care about.

## Commands

```
krci [--portal-url <url>]
  auth        login | status | logout
  project     list | get <name>
  deployment  list | get <name>
  pipelinerun list | get <name>       (also filters, --logs, --reason)
  sonar       list | get | gate | issues <project>
  version
```

**Aliases:** `project` → `proj`, `deployment` → `dp`, `pipelinerun` → `run`

## Configuration

| Flag           | Env               | Description             |
|----------------|-------------------|-------------------------|
| `--portal-url` | `KRCI_PORTAL_URL` | KubeRocketCI Portal URL |

Issuer URL, cluster name, and namespace are auto-discovered from the
Portal on `auth login`.

| Path                         | Purpose                                     |
|------------------------------|---------------------------------------------|
| `~/.config/krci/config.yaml` | Portal URL and discovered metadata          |
| `~/.config/krci/tokens.enc`  | AES-encrypted tokens; key lives in keyring  |

## Build from source

```bash
make build        # → dist/krci
make ci           # lint → test → build
```

Requires Go 1.26+. For running against a local portal, see
[`docs/development.md`](docs/development.md).

## License

[Apache License 2.0](LICENSE)

## Related reading

- [KubeRocketCI Documentation](https://docs.kuberocketci.io/)
- [Developer Guide](https://docs.kuberocketci.io/docs/next/developer-guide)
- [Install KubeRocketCI](https://docs.kuberocketci.io/docs/next/operator-guide/installation-overview)
