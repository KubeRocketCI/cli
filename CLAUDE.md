# krci CLI

Go CLI for KubeRocketCI (`github.com/KubeRocketCI/cli`).

## Build & Test

```
make build        # → dist/krci
make test         # race detector + coverage
make lint         # golangci-lint v2
make lint-fix     # auto-fix formatting
make ci           # lint → test → build
```

## Architecture

Portal-only: CLI talks exclusively to the portal tRPC API (Bearer idToken). No direct K8s access.

- `internal/portal/` — tRPC client, project/deployment services, config
- `internal/auth/` — OIDC token provider (login, refresh, env-var override)
- `internal/cmdutil/Factory` — lazy-init: IOStreams, Config, TokenProvider, PortalClient
- `internal/config/` — Viper-based config: flags > env vars > config file > defaults
- `pkg/cmd/<group>/<verb>/` — commands export `NewCmd<Verb>(f, runF)`. Follow existing as examples.

### Factory validation gate

The Factory validates all required backend configuration before creating PortalClient:
`PortalURL`, `ClusterName`, `Namespace`. Commands never see invalid state.

### Single flag

`--portal-url`. Everything else (issuer, namespace, cluster) auto-discovered on login.

### Public OIDC issuer

Issuer URL comes from portal `config.get` (unauthenticated) — public metadata per OIDC Discovery.
