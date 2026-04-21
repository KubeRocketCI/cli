# `krci project` — Codebase resources

Inspect the `Codebase` resources registered in the Portal — applications,
libraries, autotests, and infrastructure repos.

**Alias:** `proj`

## Subcommands

| Command                  | Purpose                 |
|--------------------------|-------------------------|
| `project list` (`ls`)    | List all projects       |
| `project get <name>`     | Show a single project   |

Both accept `-o, --output` with `table` (default) or `json`.

## `project list`

```bash
krci project list
```

```
NAME                             TYPE          LANGUAGE     BUILD TOOL        STATUS
keycloak-operator                application   other        go                created
payments-api                     application   java         maven             created
orders-ui                        application   javascript   npm               created
shared-libs                      library       other        kaniko            created
smoke-tests                      autotest      other        gradle            created
```

Scripting — pick all failed-status projects:

```bash
krci project list -o json | jq -r '.[] | select(.status=="failed") | .name'
```

## `project get`

```bash
krci project get keycloak-operator
```

```
Name:         keycloak-operator
Namespace:    edp-delivery
Type:         application
Language:     other
Build Tool:   go
Framework:    keycloak-operator
Git Server:   github
Git URL:      https://github.com/epam/edp-keycloak-operator
Status:       created
Available:    true
```

JSON envelope (full output):

```json
{
  "name": "keycloak-operator",
  "namespace": "edp-delivery",
  "type": "application",
  "language": "other",
  "buildTool": "go",
  "framework": "keycloak-operator",
  "gitServer": "github",
  "gitUrl": "https://github.com/epam/edp-keycloak-operator",
  "status": "created",
  "available": true
}
```

Pull a single field from an agent workflow:

```bash
krci project get keycloak-operator -o json | jq -r '.gitUrl'
```
