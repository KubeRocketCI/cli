# KubeRocketCI CLI

| :heavy_exclamation_mark: Please refer to the [KubeRocketCI documentation](https://docs.kuberocketci.io/) to get the notion of the main concepts and guidelines. |
| --- |

`krci` is the command-line companion for the [KubeRocketCI](https://kuberocketci.io)
platform — inspect projects, deployments, pipeline runs, and SonarQube state
straight from the terminal, with JSON output ready for agent workflows.

## From chat to fix — CLI + AI agent

`krci` is built to pair with an AI coding assistant (Claude Code, Cursor,
Copilot CLI, etc.). Every data command emits predictable JSON under
`-o json`, so an agent can call `krci` as a tool and reason about the
results without scraping tables.

A typical day:

1. **Triage** — *"Any red PRs for me today?"*
   → agent runs `krci run list --author jane --status failed -o json`
2. **Diagnose** — *"Why did `build-api-master-x1y2` fail?"*
   → agent runs `krci run get build-api-master-x1y2 --reason -o json`,
     reads the task tree, the failed step, and the relevant log lines
3. **Gate-check** — *"Is PR 44 ready to merge?"*
   → agent runs `krci sonar gate my-app --pull-request 44 -o json` and
     `krci sonar issues my-app --pull-request 44 --severity BLOCKER,CRITICAL -o json`
4. **Fix & verify** — agent proposes a patch from the evidence, you commit,
   CI re-runs, agent re-checks the new run

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
