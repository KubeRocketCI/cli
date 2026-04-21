# Local development

How to run the CLI against a locally running KubeRocketCI Portal.

## Prerequisites

1. A KubeRocketCI portal backend on port 3001
2. The portal's `.env` configured with a valid `OIDC_ISSUER_URL`
   (e.g. a Keycloak realm)
3. Go 1.26+

## Start the portal

```bash
cd krci-portal
cp .env.example .env   # fill in OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET
pnpm dev
```

The backend listens on `http://localhost:3001`. The Vite dev server
(port 5173) only proxies `/api`, but the CLI talks to `/rest/v1/` endpoints —
point it directly at port 3001.

## Build the CLI

```bash
make build        # → dist/krci
make test         # race detector + coverage
make lint         # golangci-lint v2
make lint-fix     # auto-fix formatting
make ci           # lint → test → build
```

## Login against the local portal

The local portal runs over HTTP, so the CLI can't auto-discover the OIDC
issuer or cluster metadata. Supply everything upfront via environment
variables:

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

All values are persisted to `~/.config/krci/config.yaml`, so subsequent
commands need no flags or env vars:

```bash
./dist/krci project list
./dist/krci deployment list
./dist/krci pipelinerun list --project keycloak-operator --reason
```

## Architecture notes

- **Portal-only** — the CLI talks exclusively to the portal tRPC API (Bearer
  `idToken`). No direct Kubernetes access.
- **Factory gate** — `internal/cmdutil/Factory` validates `PortalURL`,
  `ClusterName`, and `Namespace` before creating `PortalClient`; commands
  never see invalid state.
- **Config precedence** — flags > env vars > `config.yaml` > defaults
  (Viper-based, `internal/config/`).
- **Public OIDC issuer** — the issuer URL comes from the portal's
  unauthenticated `config.get` endpoint (public metadata per OIDC Discovery).

## Command layout

Each command lives in `pkg/cmd/<group>/<verb>/` and exports
`NewCmd<Verb>(f, runF)`. Follow the existing commands as examples.
