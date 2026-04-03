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

# JSON output for scripting
krci project list -o json
```

## Commands

```
krci [--portal-url <url>]
  auth login          Authenticate via browser (OIDC)
  auth status         Show authentication state
  auth logout         Clear stored credentials
  project list|ls     List projects
  project get <name>  Show project details
  deployment list|ls  List deployments
  deployment get <name>  Show deployment details
  version             Print version info
```

**Aliases:** `project` → `proj`, `deployment` → `dp`

### Global Flag

| Flag | Env | Description |
|---|---|---|
| `--portal-url` | `KRCI_PORTAL_URL` | KubeRocketCI Portal URL |

All other settings (issuer URL, cluster name, namespace) are auto-discovered on login.

### Output Flag

All data commands accept `-o, --output` with values `table` (default) or `json`.

## Configuration

Config file: `~/.config/krci/config.yaml` (auto-populated on login).
Token storage: `~/.config/krci/tokens.enc` (AES-encrypted, key in OS keyring).

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
