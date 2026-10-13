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
- `internal/cmdutil/Factory` — lazy-init: IOStreams, Config, TokenProvider, RestClient, Services (`portal.Services` over RestClient)
- `internal/cmdutil/validate.go` — shared validators
- `internal/config/` — Viper-based config: flags > env vars > config file > defaults
- `internal/termseq/` — the terminal sequence policy: which SGR parameters and OSC 8 targets pass. Both the streams and the output primitives call it.
- `internal/iostreams/` — process streams. `FromWriters` builds stdout and stderr as the terminal policy (`terminal.go`: what may reach a terminal) followed by one colour profile; `New` is the raw form for tests. Write styled text with `fmt`; `lipgloss.Fprint*` is banned.
- `internal/output/` — terminal primitives (`Cell`, tables, detail lines, headings, styles); each takes an `io.Writer` and the TTY state. No `-o` knowledge. A `Cell`'s styled form is built only here; cluster text enters through `Text`, `Hyperlink` and the color constructors.
- `pkg/cmd/internal/render/` — the CLI output contract: `-o` names, validation and dispatch, the `{schemaVersion, data}` and error envelopes, the auth-error prefix. Every `-o` decision goes through it; the JSON and YAML encoders stay in `output`.
- `pkg/cmd/<group>/<verb>/` — commands export `NewCmd<Verb>(f, runF)`. Follow existing as examples.

depguard and forbidigo (rules in `.golangci.yml`) enforce the module boundaries; each rule's `desc` or `msg` states its boundary. Commands get portal services, scoped to the configured target, from `Factory.Services`; they neither import `restapi` or `internal/config` nor read `Factory.RestClient` or `Factory.Config`. Exceptions: `root.go` imports config only to register its flags; `login.go` reads `Factory.Config` and calls the portal config endpoints. Tests and `cmdtest` are exempt from these rules.

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
