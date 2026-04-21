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
- `internal/cmdutil/validate.go` — shared validators
- `internal/config/` — Viper-based config: flags > env vars > config file > defaults
- `pkg/cmd/<group>/<verb>/` — commands export `NewCmd<Verb>(f, runF)`. Follow existing as examples.

### Factory validation gate

The Factory validates all required backend configuration before creating PortalClient:
`PortalURL`, `ClusterName`, `Namespace`. Commands never see invalid state.

### Single flag

`--portal-url`. Everything else (issuer, namespace, cluster) auto-discovered on login.

### Public OIDC issuer

Issuer URL comes from portal `config.get` (unauthenticated) — public metadata per OIDC Discovery.

## Portal API client (OpenAPI)

`internal/portal/restapi/api_gen.go` is generated from `internal/portal/openapi/spec.json`
via `make generate`. Both files are vendored artifacts.

**Never hand-edit `spec.json` or `api_gen.go`.** The spec's upstream source of truth
is `krci-portal/packages/trpc/api/openapi.json`, produced from the portal's tRPC
Zod schemas by `pnpm --filter @my-project/trpc run generate:openapi`.

Required workflow when the API surface needs to change:

1. Update the Zod schema / procedure in `krci-portal/packages/trpc/src/...`.
2. Run `pnpm --filter @my-project/trpc run generate:openapi` in the portal repo.
3. Copy `krci-portal/packages/trpc/api/openapi.json` over `internal/portal/openapi/spec.json`.
4. Run `make generate` in the CLI to refresh `api_gen.go`.
5. Update Go call sites and run `make ci`.

Editing `spec.json` directly is a convention-level bug: the next upstream refresh
silently reverts the change and breaks the CLI.
