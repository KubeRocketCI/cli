# KubeRocketCI CLI

| :heavy_exclamation_mark: Please refer to the [KubeRocketCI documentation](https://docs.kuberocketci.io/) to get the notion of the main concepts and guidelines. |
| --- |

Command-line interface for the [KubeRocketCI](https://kuberocketci.io) platform — enables developers and AI agents to manage projects, deployments, and environments from the terminal.

## Overview

`krci` is a CLI client that interacts with KubeRocketCI platform resources via the KubeRocketCI Portal API. It provides secure OIDC-based authentication and styled terminal output for human users, with JSON output for automation and AI agent workflows.

## Features

- **Authentication** — OIDC Authorization Code + PKCE flow, encrypted token storage (AES-256-GCM), OS keyring integration
- **Projects** — List and inspect Codebase resources
- **Deployments** — List and inspect CDPipeline and Stage resources
- **Pipeline Runs** — List, filter, diagnose failures, and stream logs
- **Output** — Styled tables for terminals, plain text for pipes, JSON for automation

## Installation

```bash
brew tap KubeRocketCI/homebrew-tap
brew install krci
```

Or download a binary from [GitHub Releases](https://github.com/KubeRocketCI/cli/releases).

## Quick Start

```bash
# Authenticate with your KubeRocketCI instance
krci auth login --portal-url https://portal.example.com

# Check auth status
krci auth status

# List projects
krci project list

# Get project details
krci project get my-app

# List deployments
krci deployment list

# Get deployment details
krci deployment get my-pipeline

# List recent pipeline runs
krci pipelinerun list

# Filter by project, PR, status, author, type
krci pipelinerun list --project my-app --pr 44
krci pipelinerun list --status failed --type review
krci pipelinerun list --project my-app --author "John Doe"

# Diagnose why a pipeline failed
krci pipelinerun list --project my-app --pr 44 --reason

# Get a specific pipeline run's logs
krci pipelinerun get review-my-app-main-a1b2c3 --logs

# JSON output for scripting
krci project list -o json
```

## Commands

```
krci [--portal-url <url>]
  auth login              Authenticate via browser (OIDC)
  auth status             Show authentication state
  auth logout             Clear stored credentials
  project list|ls         List projects
  project get <name>      Show project details
  deployment list|ls      List deployments
  deployment get <name>   Show deployment details
  pipelinerun list|ls     List and filter pipeline runs
  pipelinerun get <name>  Get a specific pipeline run
  version                 Print version info
```

**Aliases:** `project` → `proj`, `deployment` → `dp`, `pipelinerun` → `run`

### Pipeline Run Filters

All filters combine with AND logic.

| Flag        | Description                                                       |
|-------------|-------------------------------------------------------------------|
| `--project` | Filter by project name                                            |
| `--pr`      | Filter by pull request number                                     |
| `--author`  | Filter by author name                                             |
| `--branch`  | Filter by source branch                                           |
| `--type`    | Filter by type (review, build, deploy, release)                   |
| `--status`  | Filter by status (succeeded, failed, running, timeout, cancelled) |
| `--logs`    | Append logs for the most recent run                               |
| `--reason`  | Show task tree and failure diagnosis                              |

## Inspecting a Specific Pipeline Run

Use `get` to inspect a specific run by name (from `list -o json` or the table output):

```bash
# Basic info
krci run get review-my-app-main-a1b2c3

# Full logs
krci run get review-my-app-main-a1b2c3 --logs

# Failure diagnosis (task tree + failed step + logs)
krci run get review-my-app-main-a1b2c3 --reason
```

## Diagnosing Failed Pipelines

Use `--reason` on either `list` or `get` for a full diagnosis — task tree, failed step, and relevant logs:

```bash
# From the list (targets most recent run matching filters)
krci run list --project my-app --pr 44 --reason

# Or by specific name
krci run get review-my-app-main-a1b2c3 --reason
```

```bash
Pipeline: review-my-app-main-a1b2c3
  Status:   Failed
  Duration: 3m 14s

Tasks:
  ✓ fetch-repository             Succeeded   11s
  ✓ build                        Succeeded   2m 8s
  ✗ sonar                        Failed      52s
  ✓ helm-lint                    Succeeded   8s

Failed: sonar
  Step:    sonar-scanner (exit code 2)
  Message: "step-sonar-scanner" exited with code 2

Logs: sonar
  [sonar-scanner] ERROR: QUALITY GATE STATUS: FAILED
```

For AI agents, use JSON output and filter failed tasks:

```bash
krci run get review-my-app-main-a1b2c3 --reason -o json | jq '.tasks[] | select(.failedStep)'
```

### Global Flag

| Flag           | Env               | Description             |
|----------------|-------------------|-------------------------|
| `--portal-url` | `KRCI_PORTAL_URL` | KubeRocketCI Portal URL |

All other settings (issuer URL, cluster name, namespace) are auto-discovered on login.

### Output Flag

All data commands accept `-o, --output` with values `table` (default) or `json`.

## Configuration

Config file: `~/.config/krci/config.yaml` (auto-populated on login).
Token storage: `~/.config/krci/tokens.enc` (AES-encrypted, key in OS keyring).

## Local Development

### Prerequisites

1. A running KubeRocketCI portal backend (port 3001)
2. The portal's `.env` configured with a valid `OIDC_ISSUER_URL` (e.g. your Keycloak realm)

Start the portal:

```bash
cd krci-portal
cp .env.example .env   # fill in OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET
pnpm dev
```

The backend starts at `http://localhost:3001`. The Vite dev server (port 5173)
only proxies `/api` — the CLI uses `/rest/v1/` endpoints, so it must talk to
port 3001 directly.

### Build the CLI

```bash
make build    # → dist/krci
```

### Login

The portal runs over HTTP, so the CLI cannot auto-discover the OIDC issuer or
cluster metadata. Supply all values upfront via environment variables (copy
`OIDC_ISSUER_URL` from the portal's `.env` or its startup output):

```bash
KRCI_ISSUER_URL=https://idp.example.com/realms/my-realm \
KRCI_CLUSTER_NAME=my-cluster \
KRCI_NAMESPACE=my-namespace \
  ./dist/krci auth login --portal-url http://localhost:3001
```

A browser window opens for authentication. On success:

```
Logged in as user@example.com (User Name)
```

All values are saved to `~/.config/krci/config.yaml`, so subsequent commands
need no flags or environment variables:

```bash
./dist/krci project list
./dist/krci deployment list
./dist/krci pipelinerun list --project my-app --reason
```

## Prerequisites

- Go 1.26+ (for building from source)
- Access to a KubeRocketCI instance with an OIDC provider configured

## Building

```bash
make build
```

## License

[Apache License 2.0](LICENSE)

### Related Articles

- [KubeRocketCI Documentation](https://docs.kuberocketci.io/)
- [Developer Guide](https://docs.kuberocketci.io/docs/next/developer-guide)
- [Install KubeRocketCI](https://docs.kuberocketci.io/docs/next/operator-guide/installation-overview)
