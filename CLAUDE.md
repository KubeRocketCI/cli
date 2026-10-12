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
- `internal/cmdutil/Factory` — lazy-init: IOStreams, Config, TokenProvider, RestClient
- `internal/cmdutil/validate.go` — shared validators
- `internal/config/` — Viper-based config: flags > env vars > config file > defaults
- `internal/output/` — terminal primitives (tables, detail lines, headings, styles); each takes an `io.Writer` and the TTY state. No `-o` knowledge.
- `pkg/cmd/internal/render/` — the CLI output contract: `-o` names, validation and dispatch, the `{schemaVersion, data}` and error envelopes, the auth-error prefix. Every `-o` decision goes through it; the JSON and YAML encoders stay in `output`.
- `pkg/cmd/<group>/<verb>/` — commands export `NewCmd<Verb>(f, runF)`. Follow existing as examples.

depguard enforces the split: `output` imports neither `iostreams` nor `cmdutil`, `cmdutil` does not import `output`, `render` does not import `cmdutil`.

### Factory validation gate

The Factory validates all required backend configuration before creating RestClient:
`PortalURL`, `ClusterName`, `Namespace`. Commands never see invalid state.

### Single flag

`--portal-url`. Everything else (issuer, namespace, cluster) auto-discovered on login.

### Public OIDC issuer

Issuer URL comes from portal `config.get` (unauthenticated) — public metadata per OIDC Discovery.

## Portal API client (OpenAPI)

`internal/portal/openapi/spec.json` and `internal/portal/restapi/api_gen.go` are
vendored artifacts. **Never hand-edit them.** Regenerate upstream in the portal repo
(`pnpm --filter @my-project/trpc run generate:openapi`), copy the resulting
`openapi.json` over `spec.json`, then run `make generate`.
