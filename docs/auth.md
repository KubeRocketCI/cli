# `krci auth` — authentication

OIDC Authorization Code + PKCE flow against the Portal's configured provider.
Tokens are stored encrypted on disk (AES-256-GCM); the key lives in the OS
keyring, or in `~/.config/krci/.keyfile` (see [Config & token storage](#config--token-storage)).

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

### Portal URL

Every command that calls the portal, `auth login` included, checks the portal
URL before the first request:

- Scheme `https`. `http` is accepted only when the host is `localhost` or a
  loopback IP (`127.0.0.0/8`, `[::1]`).
- A host is required; user info (`https://user@host`), a query and a fragment
  are refused. A path is allowed.
- A portal response that redirects to another host or port, or from `https`
  to `http`, is not followed; the request fails and the redirect target is not
  contacted. An omitted port is the scheme's default (`443` for `https`, `80`
  for `http`), so `https://host:443` and `https://host` are the same port.
  Redirects to the same host and port are followed; the chain stops after 10
  requests.

| Condition | Exit | Message (stderr) |
|---|---|---|
| URL cannot be parsed | `1` | `invalid portal URL: cannot be parsed` |
| No host | `1` | `invalid portal URL: a host is required` |
| URL carries a query or fragment | `1` | `invalid portal URL: query and fragment are not allowed` |
| URL carries user info | `1` | `invalid portal URL: user info is not allowed: <url>` (password shown as `xxxxx`) |
| Scheme is not `https`, or `http` host is not loopback | `1` | `invalid portal URL: must use https, or http on localhost or a loopback IP: <url>` |
| Redirect to another host or port (an omitted port is the scheme's default), or from `https` to `http` | `1` | `<context>: Get "<location>": redirect refused from <url> to <target>` |

`auth login` checks the URL before issuer discovery and before the browser
opens, so a refused URL is never saved to `config.yaml`.

## `auth status`

```bash
krci auth status
```

```
User:         user@example.com
Name:         User Name
Status:       Authenticated
Expires:      22 Apr 26 11:22 EEST (14m51s)
Groups:       admin, developers, viewers
```

### Exit codes

| State                                   | Exit | Message (stderr)                             |
|-----------------------------------------|------|----------------------------------------------|
| Valid session                           | `0`  | —                                            |
| No stored token bound to the portal URL | `1`  | `not authenticated: run 'krci auth login'`   |
| Session expired and refresh not possible| `1`  | `session expired: run 'krci auth login'`     |
| `KRCI_TOKEN` is a JWT past its `exp`    | `1`  | `KRCI_TOKEN has expired: supply a fresh token` |
| `KRCI_TOKEN` or unreadable claims, portal rejects the token | `1` | `not authenticated: the portal rejected the token` |
| `KRCI_TOKEN` or unreadable claims, portal unreachable or the answer is not the portal config | `1` | `verifying the token with the portal: <cause>` |
| `KRCI_TOKEN` or unreadable claims, portal URL, cluster name, or namespace not set | `1` | `<setting> not configured`, then how to set it |
| `KRCI_TOKEN` or unreadable claims, portal URL invalid       | `1` | `invalid portal URL: <reason>`; see [Common messages](json-schemas.md#error-envelope) |
| Config file holds a value of the wrong type (a YAML syntax error is only a warning) | `1` | `loading config: <cause>` |

The stored session comes from `krci auth login` and is checked locally. Its
ID token is the bearer; within 30 s of the token's `exp` the CLI refreshes it.
The CLI needs a new ID token from each refresh. An IdP may omit it (OIDC Core
§12.2); the session then ends at the stored ID token's `exp`: commands fail
with `token refresh failed: refresh response has no id_token…` and
`auth status` reports `session expired`. A failed refresh is final for the
running command; the next command tries again.
`KRCI_TOKEN`, and a stored token whose claims cannot be read, are checked with
one call to the portal through the configured portal URL, so the check needs
the same configuration as every portal command: portal URL, cluster name, and
namespace. An `http://` portal URL on `localhost` or a loopback IP works as it
does for the other commands; see [Portal URL](#portal-url).

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
`expiresAt` is the ID token's `exp`, or the access-token expiry when the ID
token has none; RFC3339 in UTC, `null` when neither is set. `user` and `name`
are omitted when the stored token's claims cannot be read.

Once flags and arguments are accepted, a failure exits `1` and writes the
error envelope to stdout, so a script can branch on either signal:

```json
{
  "schemaVersion": "1",
  "error": { "message": "not authenticated: run 'krci auth login'" }
}
```

## `auth logout`

```bash
krci auth logout
```

Deletes `~/.config/krci/tokens.enc`. The encryption key stays in the keyring or
`.keyfile`. The portal URL in `config.yaml` is preserved so subsequent
`auth login` runs don't need `--portal-url` again.

## Config & token storage

| Path                          | Purpose                                     |
|-------------------------------|---------------------------------------------|
| `~/.config/krci/config.yaml`  | Portal URL, issuer, cluster, namespace      |
| `~/.config/krci/tokens.enc`   | AES-encrypted access/refresh/id tokens      |
| OS keyring (`krci`)           | Symmetric key used to decrypt `tokens.enc`  |
| `~/.config/krci/.keyfile`     | The key, when the keyring is empty or unavailable, or with `KRCI_KEYRING_BACKEND=file` |

Reading a token never creates a key. The CLI looks up the key in the keyring,
then in `.keyfile`; with `KRCI_KEYRING_BACKEND=file`, only in `.keyfile`.

| Key lookup result | `tokens.enc` | Output |
|---|---|---|
| Key found, file decrypts | kept | — |
| Key found, file does not decrypt | deleted | `Warning: removing undecryptable token file (<path>): <cause>`, then `not authenticated: run 'krci auth login'` |
| No key in any store checked | kept; the next login replaces it | `Warning: no encryption key for the token file (<path>); the next login replaces it.` (once per command), then `not authenticated: run 'krci auth login'` |
| Keyring fails, no `.keyfile` | kept | an error ending in `unable to access OS keyring: <cause>` |
| Key unreadable or malformed | kept | an error ending in the key error, such as `key file <path>: decoding key: <cause>` |

On login with a working but empty keyring, an unreadable or malformed
`.keyfile` is ignored with a warning and a new key is stored in the keyring.
When the keyring also rejects the new key, login fails with an error naming
`.keyfile`; delete it and log in again.

`tokens.enc` records the portal URL the token was bound to at login or refresh.
A stored token is used only when that URL equals the configured portal URL;
otherwise commands fail with `not authenticated: run 'krci auth login'`. Tokens
stored by earlier CLI versions carry no binding and require one `krci auth login`
after upgrading. `KRCI_TOKEN` is not bound to a portal.

## Non-interactive environments

For CI, or when discovery isn't possible, supply everything up front:

```bash
KRCI_ISSUER_URL=https://idp.example.com/realms/my-realm \
KRCI_CLUSTER_NAME=my-cluster \
KRCI_NAMESPACE=my-namespace \
  krci auth login --portal-url http://localhost:3001
```

See [`docs/development.md`](development.md) for the full local-portal setup.
