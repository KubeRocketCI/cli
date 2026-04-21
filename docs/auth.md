# `krci auth` — authentication

OIDC Authorization Code + PKCE flow against the Portal's configured provider.
Tokens are stored encrypted on disk (AES-256-GCM); the key lives in the OS
keyring.

## Subcommands

| Command       | Purpose                              |
|---------------|--------------------------------------|
| `auth login`  | Browser-based OIDC authentication    |
| `auth status` | Show the currently signed-in user    |
| `auth logout` | Clear stored credentials             |

## `auth login`

Opens a browser to the portal's OIDC provider, captures the callback, and
persists configuration under `~/.config/krci/config.yaml`.

```bash
# Portal URL via flag
krci auth login --portal-url https://portal.example.com

# …or via environment
export KRCI_PORTAL_URL=https://portal.example.com
krci auth login
```

On success:

```
Logged in as user@example.com (User Name)
```

The issuer URL, cluster name, and namespace are auto-discovered from the
Portal's public `config.get` endpoint — no extra flags required.

## `auth status`

```bash
krci auth status
```

```
User:         user@example.com
Name:         User Name
Status:       Authenticated
Expires:      22 Apr 26 11:22 EEST (22h24m3s)
Groups:       admin, developers, viewers
```

Exits non-zero when the token is missing or expired — handy for wrapping in a
shell guard:

```bash
krci auth status >/dev/null 2>&1 || krci auth login
```

## `auth logout`

```bash
krci auth logout
```

Deletes `~/.config/krci/tokens.enc` and removes the key from the keyring. The
portal URL in `config.yaml` is preserved so subsequent `auth login` runs don't
need `--portal-url` again.

## Config & token storage

| Path                          | Purpose                                     |
|-------------------------------|---------------------------------------------|
| `~/.config/krci/config.yaml`  | Portal URL, issuer, cluster, namespace      |
| `~/.config/krci/tokens.enc`   | AES-encrypted access/refresh/id tokens      |
| OS keyring (`krci`)           | Symmetric key used to decrypt `tokens.enc`  |

## Non-interactive environments

For CI or local dev against an HTTP portal where discovery isn't possible,
supply everything up front:

```bash
KRCI_ISSUER_URL=https://idp.example.com/realms/my-realm \
KRCI_CLUSTER_NAME=my-cluster \
KRCI_NAMESPACE=my-namespace \
  krci auth login --portal-url http://localhost:3001
```

See [`docs/development.md`](development.md) for the full local-portal setup.
