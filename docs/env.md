# `krci env` — Environments (Stages)

Inspect KRCI environments — the `Stage` resources surfaced as
"environments" in the Portal — without leaving the terminal. Lists every
stage in the configured namespace or shows full detail for one
(deployment, env) pair, including infrastructure, quality gates, and the
projects deployed there with health, sync, version, image digest, and
ingress URLs. For an environment on the cluster the Portal runs on, lists
the pods of its namespace and the Kubernetes events of that namespace.

**Alias:** `e`

## Subcommands

| Command                                     | Purpose                                                                |
|---------------------------------------------|------------------------------------------------------------------------|
| `env list` (`ls`)                           | List every environment in the namespace, optionally filtered           |
| `env get <deployment> <env>`                | Full detail for one environment, including the projects deployed there |
| `env pods <deployment> <env>`               | The pods of one environment, with the state of every container         |
| `env events <deployment> <env>`             | The Kubernetes events of one environment, newest first                 |

Every subcommand accepts `-o, --output` with `table` (default) or `json`.

`<env>` everywhere refers to `Stage.spec.name` (the short user-facing
identifier `dev`, `stage`, `prod`, `qa`), not the compound K8s resource
name.

## `env list`

```bash
krci env list
```

```
DEPLOYMENT     ENV     CLUSTER       NAMESPACE             TRIGGER   STATUS
my-pipeline    dev     in-cluster    my-pipeline-dev       Auto      created
my-pipeline    stage   in-cluster    my-pipeline-stage     Manual    created
my-pipeline    prod    in-cluster    my-pipeline-prod      Manual    created
other-pipe     dev     in-cluster    other-pipe-dev        Auto      in_progress
```

Sort order: `deployment` ascending, then `Stage.spec.order` ascending.

### Filters

```bash
# Only one deployment
krci env list --deployment my-pipeline

# Only one cluster
krci env list --cluster in-cluster

# Combined
krci env list --deployment my-pipeline --cluster in-cluster
```

| Flag            | Purpose                                                       |
|-----------------|---------------------------------------------------------------|
| `--deployment`  | Filter by parent CDPipeline name (DNS-1123 name)              |
| `--cluster`     | Filter by `Stage.spec.clusterName` (DNS-1123 name)            |
| `-o, --output`  | `table` (default) or `json`                                   |

Empty result is success: `data.stages: []`, exit `0`, with
`No environments found.` to stderr in table mode.

### JSON envelope

```bash
krci env list -o json
```

```json
{
  "schemaVersion": "1",
  "data": {
    "stages": [
      {
        "deployment": "my-pipeline",
        "env": "dev",
        "cluster": "in-cluster",
        "namespace": "my-pipeline-dev",
        "triggerType": "Auto",
        "status": "created",
        "order": 0
      }
    ]
  }
}
```

Scripting — group environments by status:

```bash
krci env list -o json | jq -r '.data.stages[] | "\(.deployment)/\(.env): \(.status)"'
```

## `env get`

```bash
krci env get my-pipeline prod
```

```
Environment:  prod
Deployment:   my-pipeline
Status:       created
Description:  Production environment
Order:        2

Infrastructure:
  Cluster:           in-cluster
  Namespace:         my-pipeline-prod
  Trigger Type:      Manual
  Deploy Pipeline:   deploy-with-helm
  Clean Pipeline:    clean-helm

Quality Gates (2):
  - manual: stage-approval
  - autotests: smoke-tests (branch: main)

Projects (3):
PROJECT  STATUS    SYNC        VERSION   IMAGE_SHA          INGRESS
foo      healthy   synced      1.2.0     sha256:abc12345    foo.prod.example.com
bar      healthy   synced      2.0.1     sha256:def34567    bar.prod.example.com
                                                            bar-admin.prod.example.com
baz      -         -           -         -                  -
```

`<deployment>` and `<env>` are **both positional and both required**. Both
must be DNS-1123 names (lowercase alphanumerics + hyphens, no dots, ≤ 253 chars).
Invalid input fails with exit `1` before contacting the Portal.

### Output blocks

The TTY view is layered top-to-bottom:

1. **Header** — `Environment / Deployment / Status / Description / Order`. Status
   uses `output.StatusColor` (`created` → green, `failed` → red, `in_progress` → yellow).
   A `Message` line follows `Status` when the Stage reports
   `status.detailed_message`, the operator's reason behind a `failed` status.
2. **Infrastructure** — indented block with the Stage's static placement.
   `Clean Pipeline: —` when no `cleanTemplate` is set.
3. **Quality Gates** — one bullet per gate; `autotests` gates show their
   autotest name and branch (e.g. `autotests: smoke-tests (branch: main)`).
4. **Projects sub-table** — column order: `PROJECT`, `STATUS`, `SYNC`,
   `VERSION`, `IMAGE_SHA`, `INGRESS`. Sorted by project name ascending.
5. **Conditions** — printed only when a project carries an Argo CD condition
   or a sync operation that did not succeed:

   ```
   Conditions (2):
     - foo: ComparisonError: Failed to load live state: failed to get cluster info for "https://k8s.example.com": dial tcp: i/o timeout
     - foo: operation Error: ComparisonError: Failed to load target state
   ```

   This is where the reason behind an `unknown` or `degraded` status lives:
   an unreachable target cluster, a chart that fails to render, a
   `build/<version>` revision that does not exist. Multi-line messages are
   collapsed to one row.

Projects sub-table semantics:

- **STATUS** — ArgoCD health: `healthy` (green), `degraded` / `missing` (red),
  `progressing` (blue, same color as a running pipeline run); `-` when the
  project is registered in `CDPipeline.spec.applications` but no `Application`
  exists yet
- **VERSION** — helm `image.tag` parameter (or `targetRevision` last segment
  when helm is absent)
- **IMAGE_SHA** — short content digest (`sha256:` + first 8 hex chars =
  15 visible chars). Full digest under `imageDigest` in `-o json`
- **INGRESS** — hostnames from `status.summary.externalURLs`. Multiple URLs
  stack across visual rows (the project name appears only on the first row);
  hostnames are truncated at 50 chars but the OSC 8 hyperlink target keeps
  the full URL — `cmd-click` opens it

### Not-found errors

```bash
$ krci env get nope dev
Error: deployment "nope" not found

$ krci env get my-pipeline wat
Error: environment "wat" not found in deployment "my-pipeline"
```

Both exit `1`. Under `-o json` the message is also written to stdout as
`{"schemaVersion":"1","error":{"message":"…"}}`.

### JSON envelope

```bash
krci env get my-pipeline prod -o json
```

```json
{
  "schemaVersion": "1",
  "data": {
    "deployment": "my-pipeline",
    "env": "prod",
    "status": "created",
    "detailedMessage": null,
    "description": "Production environment",
    "order": 2,
    "infrastructure": {
      "cluster": "in-cluster",
      "namespace": "my-pipeline-prod",
      "triggerType": "Manual",
      "deployPipeline": "deploy-with-helm",
      "cleanPipeline": "clean-helm"
    },
    "qualityGates": [
      { "type": "manual", "stepName": "stage-approval", "autotestName": null, "branchName": null },
      { "type": "autotests", "stepName": "smoke", "autotestName": "smoke-tests", "branchName": "main" }
    ],
    "projects": [
      {
        "name": "foo",
        "status": "healthy",
        "sync": "synced",
        "version": "1.2.0",
        "imageTag": "1.2.0",
        "imageDigest": "sha256:abc12345...",
        "ingressUrls": ["https://foo.prod.example.com"],
        "argocdUrl": "/applications/my-pipeline-my-pipeline-prod-foo",
        "deployedAt": "2026-04-25T08:00:00Z",
        "valuesOverride": false,
        "conditions": [],
        "operation": {
          "phase": "Succeeded",
          "message": "successfully synced (all tasks run)",
          "startedAt": "2026-04-25T07:59:40Z",
          "finishedAt": "2026-04-25T08:00:00Z"
        }
      },
      {
        "name": "bar",
        "status": "unknown",
        "sync": "unknown",
        "version": "2.0.1",
        "imageTag": "2.0.1",
        "imageDigest": null,
        "ingressUrls": [],
        "argocdUrl": "/applications/my-pipeline-my-pipeline-prod-bar",
        "deployedAt": null,
        "valuesOverride": false,
        "conditions": [
          {
            "type": "ComparisonError",
            "message": "Failed to load live state: failed to get cluster info for \"https://k8s.example.com\": dial tcp: i/o timeout",
            "lastTransitionTime": "2026-04-25T08:03:12Z"
          }
        ],
        "operation": null
      },
      {
        "name": "baz",
        "status": null,
        "sync": null,
        "version": null,
        "imageTag": null,
        "imageDigest": null,
        "ingressUrls": [],
        "argocdUrl": null,
        "deployedAt": null,
        "valuesOverride": null,
        "conditions": [],
        "operation": null
      }
    ]
  }
}
```

Field absence rules:

- `description`, `cleanPipeline` → `null` when the Stage spec omits them.
- `detailedMessage` → the Stage's `status.detailed_message`, `null` when the
  operator reported none.
- Per-project dynamic fields (`status`, `sync`, `version`, `imageTag`,
  `imageDigest`, `argocdUrl`, `deployedAt`, `valuesOverride`) → `null` for
  registered-but-not-deployed projects. `ingressUrls` is always an array,
  `[]` when none.
- `conditions[]` is always an array, `[]` when the Application has none;
  each entry carries `type`, `message`, and `lastTransitionTime` (`null`
  when Argo CD omits it). `operation` → `null` for
  registered-but-not-deployed projects and for Applications never synced;
  otherwise `phase`, `message`, `startedAt`, `finishedAt`, each of the last
  three `null` when absent.
- `qualityGates[]` is always an array (empty when none).
- In `-o json`, `imageDigest` is the FULL `sha256:...`. The table view
  shortens it to 15 visible characters under `IMAGE_SHA`.

### Scripting examples

```bash
# Projects that aren't healthy in this env
krci env get my-pipeline prod -o json |
  jq -r '.data.projects[] | select(.status != "healthy" and .status != null) | "\(.name): \(.status)"'

# All ingress URLs for one project in this env
krci env get my-pipeline prod -o json |
  jq -r '.data.projects[] | select(.name=="foo") | .ingressUrls[]'

# Why is a project unknown or degraded? Argo CD conditions and the last operation
krci env get my-pipeline prod -o json |
  jq -r '.data.projects[] | .name as $n | (.conditions[] | "\($n): \(.type): \(.message)"),
         (select(.operation != null and .operation.phase != "Succeeded") | "\($n): operation \(.operation.phase): \(.operation.message)")'

# Quality-gate names + branches
krci env get my-pipeline prod -o json |
  jq -r '.data.qualityGates[] | "\(.type): \(.stepName) (\(.branchName // "no-branch"))"'
```

## `env pods`

```bash
krci env pods my-pipeline dev
```

```
POD                    PROJECT   STATUS             READY   RESTARTS     CREATED
bar-5d8f7c6b9d-abcde   bar       Running            1/1     0            2d ago
foo-6c9f7d9b8-x2x9k    foo       CrashLoopBackOff   0/1     7 (2m ago)   31m ago
seed-29857080-jm9bh    -         Completed          0/1     0            3h ago

Details (1):
  - foo-6c9f7d9b8-x2x9k/foo: waiting (CrashLoopBackOff): back-off 5m0s restarting failed container=foo; last termination: Error, exit 1
      image: registry.example.com/ns/foo:1.2.0@sha256:abc12345
```

Lists every pod in the namespace of the environment (`infrastructure.namespace`
of `env get`), sorted by name. This is where the reason behind a `degraded`
or `progressing` project of `env get` shows: a crash loop, an image that
cannot be pulled, a pod that cannot be scheduled.

Columns:

- **PROJECT** — the project of the deployment whose name the pod carries in
  its `app.kubernetes.io/instance` label, the label the Portal reads to find
  the pods of an application; `-` for any other pod of the namespace
- **STATUS**, **READY** — the values `kubectl get pods` prints for the pod.
  On a TTY the status is green for a pod that runs with every container
  ready or has completed, red for a failed pod, yellow otherwise
- **RESTARTS** — the restart count `kubectl get pods` prints, followed by
  the time of the newest restart it counts
- **CREATED** — the pod's creation time, relative

### Details

The block below the table is printed only when a pod has something to add:

- one line per container that is not running ready, did not exit with `0`,
  or has been restarted: its state, the reason, a non-zero exit code, the
  message, and how its previous run ended; an init container is marked
  `(init)`, a sidecar `(sidecar)`. A second line names the image the
  container asks for and the short digest the kubelet pulled
- one line for a pod that carries its own `status.reason` or
  `status.message` (`Evicted`, `NodeLost`), with that reason and message
- for a pod that reports no container yet, one line per condition that
  holds it back, such as `PodScheduled False (Unschedulable)` with the
  scheduler's message

Multi-line messages are collapsed to one row.

### Limits

- The Portal reads the namespace with your session: you need the right to
  list pods there, otherwise the command fails with
  `listing pods in namespace "<namespace>": permission denied`.
- The Portal reads only the cluster it runs on, the one a Stage names
  `in-cluster`. An environment on another cluster is refused with exit `1`:

  ```bash
  $ krci env pods my-pipeline prod
  Error: environment "prod" of deployment "my-pipeline" runs on cluster "prod-cluster"; the Portal reads pods and events only on its own cluster
  ```

- Container logs are not part of the output. The `message` of a terminated
  container is its termination message, which most images leave empty.
- A missing deployment or environment is reported like `env get` does.
- An empty namespace is success: `data.pods: []`, exit `0`, with
  `No pods found in namespace <namespace>.` to stderr in table mode.

### JSON envelope

```bash
krci env pods my-pipeline dev -o json
```

```json
{
  "schemaVersion": "1",
  "data": {
    "deployment": "my-pipeline",
    "env": "dev",
    "cluster": "in-cluster",
    "namespace": "my-pipeline-dev",
    "pods": [
      {
        "name": "foo-6c9f7d9b8-x2x9k",
        "project": "foo",
        "status": "CrashLoopBackOff",
        "phase": "Running",
        "readyContainers": 0,
        "totalContainers": 1,
        "restarts": 7,
        "lastRestartAt": "2026-04-25T08:10:02Z",
        "createdAt": "2026-04-25T07:40:11Z",
        "node": "node-1",
        "owner": { "kind": "ReplicaSet", "name": "foo-6c9f7d9b8" },
        "reason": null,
        "message": null,
        "conditions": [
          {
            "type": "Ready",
            "status": "False",
            "reason": "ContainersNotReady",
            "message": "containers with unready status: [foo]",
            "lastTransitionTime": "2026-04-25T07:41:00Z"
          }
        ],
        "containers": [
          {
            "name": "foo",
            "init": false,
            "sidecar": false,
            "image": "registry.example.com/ns/foo:1.2.0",
            "imageID": "registry.example.com/ns/foo@sha256:abc12345...",
            "imageDigest": "sha256:abc12345...",
            "ready": false,
            "restarts": 7,
            "state": "waiting",
            "reason": "CrashLoopBackOff",
            "message": "back-off 5m0s restarting failed container=foo",
            "exitCode": null,
            "startedAt": null,
            "finishedAt": null,
            "lastTermination": {
              "reason": "Error",
              "message": null,
              "exitCode": 1,
              "startedAt": "2026-04-25T08:09:58Z",
              "finishedAt": "2026-04-25T08:10:02Z"
            }
          }
        ]
      }
    ]
  }
}
```

Field rules:

- `pods[]` is always an array, sorted by `name`.
- `status`, `readyContainers`, `totalContainers` and `restarts` are the
  `STATUS`, `READY` and `RESTARTS` values of `kubectl get pods`: a sidecar
  (an init container that restarts always) counts as a container. `phase`
  is the Kubernetes pod phase. `lastRestartAt` → the time of the newest
  restart `restarts` counts, `null` without one.
- `project` → `null` for a pod whose `app.kubernetes.io/instance` label is
  not the name of a project of the deployment.
- `createdAt` → the pod's creation time. `node` → `null` until the pod is
  scheduled. `owner` → the controller of the pod (a `ReplicaSet` for a
  Deployment, a `StatefulSet`, a `DaemonSet`, a `Job`), or its first owner
  when none is marked as the controller; `null` for a pod without owners.
- `reason`, `message` → the pod's own `status.reason` and `status.message`
  (`Evicted`, `NodeLost`), `null` when the pod sets none.
- `conditions[]` is always an array and carries only the conditions that
  hold the pod back: a condition that is not `True`, or a `DisruptionTarget`
  that is. `[]` for a healthy pod. `reason`, `message` and
  `lastTransitionTime` are `null` when Kubernetes omits them.
- `containers[]` is always an array: the containers the kubelet reported,
  init containers first (`init: true`), `[]` for a pod not yet scheduled.
  `sidecar: true` marks an init container with `restartPolicy: Always`,
  which runs beside the containers; `false` for every other container.
- `containers[].image` → the image the pod asks for. `imageID` → the
  `imageID` the container runtime reports, `null` until the image is
  present. `imageDigest` → the registry digest in it, the full `sha256:...`
  that `env get` reports as `imageDigest` and `project versions` as
  `digest`; `null` when `imageID` names none. A bare `sha256:...` `imageID`
  is the image's local ID on the node (an image loaded onto the node, not
  pulled), not a registry digest. The Details line shows the short registry
  digest, else the short image ID.
- `containers[].state` is one of `waiting`, `running`, `terminated`.
  `reason` and `message` belong to a `waiting` or `terminated` state,
  `exitCode` and `finishedAt` to a `terminated` one, `startedAt` to a
  `running` or `terminated` one; each is `null` otherwise.
- `containers[].lastTermination` → how the previous run of a restarted
  container ended, `null` for a container that has not been restarted.
- The pod spec is not part of the output.

### Scripting examples

```bash
# Pods that are not fully ready, with the status kubectl would show
krci env pods my-pipeline dev -o json |
  jq -r '.data.pods[] | select(.readyContainers < .totalContainers) | "\(.name): \(.status)"'

# Why is each container of one project not running?
krci env pods my-pipeline dev -o json |
  jq -r '.data.pods[] | select(.project=="foo") | .name as $p | .containers[] |
         select(.state != "running") | "\($p)/\(.name): \(.state) \(.reason // "") \(.message // "")"'

# How did the previous run of every restarted container end?
krci env pods my-pipeline dev -o json |
  jq -r '.data.pods[] | .name as $p | .containers[] | select(.lastTermination != null) |
         "\($p)/\(.name): \(.lastTermination.reason // "-") exit \(.lastTermination.exitCode)"'

# The conditions that hold each pod back, such as why it is not scheduled
krci env pods my-pipeline dev -o json |
  jq -r '.data.pods[] | .name as $p | .conditions[] | "\($p): \(.type)=\(.status) \(.reason // ""): \(.message // "")"'
```

## `env events`

```bash
krci env events my-pipeline dev
```

```
FIRST_SEEN   LAST_SEEN   TYPE      REASON         OBJECT                      COUNT   MESSAGE
30m ago      2m ago      Warning   BackOff        Pod/foo-6c9f7d9b8-x2x9k     42      Back-off restarting failed container foo in pod foo-6c9f7d9b8-x2x9k
31m ago      4m ago      Normal    Pulled         Pod/foo-6c9f7d9b8-x2x9k     8       Container image "registry.example.com/ns/foo:1.2.0" already present on machine
31m ago      31m ago     Normal    Scheduled      Pod/foo-6c9f7d9b8-x2x9k     1       Successfully assigned my-pipeline-dev/foo-6c9f7d9b8-x2x9k to node-1
52m ago      40m ago     Warning   FailedCreate   ReplicaSet/bar-7b9c8d7f6c   3       pods "bar-7b9c8d7f6c-" is forbidden: exceeded quota
```

Lists the Kubernetes events in the namespace of the environment, newest
first by the time last seen: what the scheduler, the kubelet and the
controllers reported about its pods and workloads. Events about workloads
are listed with those about pods: a ReplicaSet that a quota stops from
creating its pod reports it here, while `env pods` shows no pod at all.

### Filters

```bash
# Only what went wrong
krci env events my-pipeline dev --warnings

# The events of one pod (take the name from `env pods`)
krci env events my-pipeline dev --pod foo-6c9f7d9b8-x2x9k

# Combined
krci env events my-pipeline dev --pod foo-6c9f7d9b8-x2x9k --warnings
```

| Flag            | Purpose                                                         |
|-----------------|-----------------------------------------------------------------|
| `--pod`         | Only the events about the pod of this exact name                |
| `--warnings`    | Only the events of type `Warning`                               |
| `-o, --output`  | `table` (default) or `json`                                     |

`--pod` selects by the object of the event, so it also finds the events of a
pod that is already gone.

On a TTY `Warning` is yellow and a long `MESSAGE` is truncated with `...`;
piped output keeps the full text on one line.

### Limits

- Kubernetes keeps an event for a limited time, one hour by default. An
  environment that broke earlier may have no event left; `env pods` still
  shows the state of its containers.
- The right to list events in the namespace, the cluster the Portal runs
  on, and a missing deployment or environment are handled as for `env pods`.
- No event is success: `data.events: []`, exit `0`, with
  `No events found in namespace <namespace>.` to stderr in table mode.

### JSON envelope

```bash
krci env events my-pipeline dev --warnings -o json
```

```json
{
  "schemaVersion": "1",
  "data": {
    "deployment": "my-pipeline",
    "env": "dev",
    "cluster": "in-cluster",
    "namespace": "my-pipeline-dev",
    "events": [
      {
        "type": "Warning",
        "reason": "BackOff",
        "message": "Back-off restarting failed container foo in pod foo-6c9f7d9b8-x2x9k",
        "involvedObject": { "kind": "Pod", "name": "foo-6c9f7d9b8-x2x9k" },
        "count": 42,
        "firstSeen": "2026-04-25T07:41:00Z",
        "lastSeen": "2026-04-25T08:10:02Z",
        "source": "kubelet"
      }
    ]
  }
}
```

Field rules:

- `events[]` is always an array, newest first by `lastSeen`.
- `count`, `firstSeen` and `lastSeen` are the values `kubectl get events`
  derives: an event recorded as a series reports the series count and its
  last observed time, an event without a count happened once.
- `source` → the component that reported the event (`kubelet`,
  `default-scheduler`, a controller), `null` when the event names none.
- `involvedObject` carries the `kind` and `name` of the object the event is
  about.

### Scripting examples

```bash
# One line per warning
krci env events my-pipeline dev --warnings -o json |
  jq -r '.data.events[] | "\(.involvedObject.kind)/\(.involvedObject.name): \(.reason): \(.message)"'

# Which objects report the most warnings?
krci env events my-pipeline dev --warnings -o json |
  jq -r '.data.events | group_by(.involvedObject.kind + "/" + .involvedObject.name)[] |
         "\(map(.count) | add)\t\(.[0].involvedObject.kind)/\(.[0].involvedObject.name)"' |
  sort -rn
```

## Status colors (TTY only)

| Value           | Color   | Same as             |
|-----------------|---------|---------------------|
| `healthy`       | green   | —                   |
| `degraded`      | red     | —                   |
| `missing`       | red     | —                   |
| `progressing`   | blue    | running pipelinerun |
| anything else (`suspended`, `unknown`, …) | unstyled | — |

## Typical workflows

```bash
# Where do I have stages, and what state are they in?
krci env list

# Drill into one environment
krci env get my-pipeline prod

# Quick "what's deployed in dev?" loop, JSON-friendly
krci env list --deployment my-pipeline -o json |
  jq -r '.data.stages[] | "\(.env): \(.status)"'

# Pre-deploy sanity check — every project healthy in prod?
krci env get my-pipeline prod -o json |
  jq -e 'all(.data.projects[]; .status == "healthy" or .status == null)' >/dev/null
```
