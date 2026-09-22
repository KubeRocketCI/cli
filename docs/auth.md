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

### Exit codes

| State                                   | Exit | Message (stderr)                             |
|-----------------------------------------|------|----------------------------------------------|
| Valid session                           | `0`  | —                                            |
| No stored session                       | `1`  | `not authenticated: run 'krci auth login'`   |
| Session expired and refresh not possible| `1`  | `session expired: run 'krci auth login'`     |
| `KRCI_TOKEN` is a JWT past its `exp`    | `1`  | `KRCI_TOKEN has expired: supply a fresh token` |
| `KRCI_TOKEN` or unreadable claims, portal rejects the token | `1` | `not authenticated: the portal rejected the token` |
| `KRCI_TOKEN` or unreadable claims, portal unreachable       | `1` | `verifying the token with the portal: <cause>` |

The stored session comes from `krci auth login` and is checked locally.
`KRCI_TOKEN`, and a stored token whose claims cannot be read, are checked with
one call to the portal through the configured portal URL, so the check needs
the same configuration as every portal command: portal URL, cluster name, and
namespace. An `http://` portal URL for local development works as it does for
the other commands.

With `KRCI_TOKEN` set, `User`, `Groups`, and `Expires` come from its claims,
not from the stored session. When the portal accepts a token whose claims
cannot be read, such as an opaque OIDC access token, the command exits `0` and
prints `Status:  Authenticated (unable to read user info)`; in JSON,
`data.user` is absent and `data.expiresAt` is `null`.

The non-zero exit makes the command a shell guard:

```bash
krci auth status >/dev/null 2>&1 || krci auth login
```

### JSON output

```bash
krci auth status -o json
```

```json
{
  "schemaVersion": "1",
  "data": {
    "authenticated": true,
    "user": "user@example.com",
    "name": "User Name",
    "groups": ["admin", "developers", "viewers"],
    "expiresAt": "2026-04-22T08:22:00Z"
  }
}
```

`groups` is always an array (`[]` when the token carries no groups);
`expiresAt` is RFC3339 in UTC, `null` when the token has no expiry; `user`
and `name` are omitted when the stored token's claims cannot be read.

Without a valid session the command still exits `1` and writes the error
envelope to stdout, so a script can branch on either signal:

```json
{
  "schemaVersion": "1",
  "error": { "message": "not authenticated: run 'krci auth login'" }
}
```

```bash
# Scripting — proceed only with a session that lasts another hour
krci auth status -o json |
  jq -e '.data.expiresAt | fromdateiso8601 > (now + 3600)' >/dev/null || krci auth login
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
