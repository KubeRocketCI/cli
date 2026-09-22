# `krci deployment` — CDPipelines and Stages

Inspect `CDPipeline` resources (called "deployments" in the Portal) and their
environments.

**Alias:** `dp`

## Subcommands

| Command                     | Purpose                                                 |
|-----------------------------|---------------------------------------------------------|
| `deployment list` (`ls`)    | List all deployments with their apps and stages         |
| `deployment get <name>`     | Show a deployment's environments and promotion gates    |

Both accept `-o, --output` with `table` (default) or `json`.

## `deployment list`

```bash
krci deployment list
```

```
NAME            APPLICATIONS                                                         ENVS       STATUS
my-pipeline     payments-api, payments-ui, payments-db +1 more                       dev        created
orders          orders-api, orders-ui, orders-queue +4 more                          dev, qa    created
shared-infra    cert-manager, ingress-nginx, external-dns                            dev        created
```

Columns:
- **APPLICATIONS** — truncated after three items with a `+N more` suffix
- **ENVS** — comma-separated list of stages (e.g. `dev, qa, prod`)
- **STATUS** — the CDPipeline status as reported by the operator

## `deployment get`

```bash
krci deployment get my-pipeline
```

```
Name:         my-pipeline
Namespace:    edp-delivery
Applications: payments-api, payments-ui, payments-db, payments-worker
Description:  Payments delivery pipeline
Status:       created
Available:    true

Environments:
ORDER   ENV    DEPLOY MODE   PROMOTE GATES   NAMESPACE                  STATUS
0       dev    Manual        1 manual        edp-delivery-my-pipe-dev   created
1       qa     Auto          2 auto          edp-delivery-my-pipe-qa    created
2       prod   Manual        1 manual        edp-delivery-my-pipe-prd   created
```

Environment columns:
- **ORDER** — promotion order within the pipeline
- **DEPLOY MODE** — `Manual` or `Auto` trigger
- **PROMOTE GATES** — number and type of quality gates that must pass
- **NAMESPACE** — target Kubernetes namespace

When the operator records `status.detailed_message` on the CDPipeline, a
`Message:` line follows `Available:`; stages with a message are listed under
a `Messages:` block below the table, `<env>: <message>` per line. Both are
absent for healthy resources.

```
Status:       failed
Available:    false
Message:      failed to create namespace my-pipeline-dev: quota exceeded

Environments:
ORDER   ENV    DEPLOY MODE   PROMOTE GATES   NAMESPACE                  STATUS
0       dev    Manual        1 manual        my-pipeline-dev            failed

Messages:
  dev: failed to create namespace my-pipeline-dev: quota exceeded
```

## JSON output

```bash
krci deployment get my-pipeline -o json
```

```json
{
  "name": "my-pipeline",
  "namespace": "edp-delivery",
  "applications": ["payments-api", "payments-ui", "payments-db"],
  "description": "Payments delivery pipeline",
  "status": "created",
  "available": true,
  "stages": [
    {
      "name": "dev",
      "order": 0,
      "triggerType": "Manual",
      "qualityGates": [
        { "name": "dev", "type": "manual" }
      ],
      "namespace": "edp-delivery-my-pipe-dev",
      "clusterName": "eks-sandbox",
      "description": "Dev stage",
      "status": "created",
      "available": true
    }
  ]
}
```

`detailedMessage` appears on the deployment and on a stage only when the
resource carries `status.detailed_message` (`"status": "failed",
"detailedMessage": "failed to create namespace ...: quota exceeded"`);
`deployment list -o json` carries it on the same terms.

Agent workflow — list namespaces for a pipeline:

```bash
krci deployment get my-pipeline -o json | jq -r '.stages[].namespace'
```
