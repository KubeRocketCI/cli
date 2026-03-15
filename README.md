# KubeRocketCI CLI

| :heavy_exclamation_mark: Please refer to the [KubeRocketCI documentation](https://docs.kuberocketci.io/) to get the notion of the main concepts and guidelines. |
| --- |

Command-line interface for the [KubeRocketCI](https://kuberocketci.io) platform — enables developers and AI agents to manage projects, deployments, pipelines, and environments from the terminal.

## Overview

`krci` is a CLI client that interacts with KubeRocketCI platform resources via Kubernetes API. It provides secure OIDC-based authentication with Keycloak and styled terminal output for human users, with JSON output for automation and AI agent workflows.

## Features

- **Authentication** — OIDC Authorization Code + PKCE flow with Keycloak, encrypted token storage (AES-256-GCM), OS keyring integration
- **Projects** — List and inspect Codebase resources
- **Deployments** — List and inspect CDPipeline and Stage resources *(planned)*
- **Output** — Styled tables for terminals, plain text for pipes, JSON for automation

## Quick Start

```bash
# Authenticate with your KubeRocketCI instance
krci auth login --issuer-url https://your-keycloak/realms/your-realm

# Check auth status
krci auth status

# List projects
krci project list

# Get project details
krci project get my-app

# JSON output for scripting
krci project list -o json
```

## Prerequisites

- Go 1.26+ (for building from source)
- Access to a KubeRocketCI instance with Keycloak configured

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
