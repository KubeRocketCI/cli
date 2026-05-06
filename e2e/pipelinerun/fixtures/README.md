# pipelinerun start — e2e fixtures

Self-contained Tekton manifests used by the `krci pipelinerun start` e2e rows
in `../test-cases.md`. Files are the source of truth — apply with
`kubectl apply -f` and the cluster matches what's checked in (no drift).

All pipelines run a single inline `taskSpec` that calls `busybox` and echoes
its inputs. None of them reach external systems, push artifacts, or write to
git. They are safe to start repeatedly. All resource names use a synthetic
`krci-cli-e2e-*` prefix and the manifests carry no organisation-specific
identifiers — drop them into any namespace on any KubeRocketCI cluster.

## Bundle

| File | Resource | Purpose | Test rows |
|---|---|---|---|
| `pipeline-noop.yaml` | Pipeline `krci-cli-e2e-noop` | Four params, all with defaults; `should-fail=true` makes the run exit non-zero. | `PR-S-1`, `PR-S-2`, `PR-S-3`, `PR-S-LABEL`, `PR-S-DRY-YAML`, `PR-S-DRY-JSON`, `PR-S-RACE`, `PR-S-GENNAME`, `PR-S-COL-EQ` |
| `pipeline-required.yaml` | Pipeline `krci-cli-e2e-required` | Declares a no-default param. Documents the portal's empty-string synthesis: omitting `--param message=...` produces `spec.params[0].value: ""`, **not** an admission rejection. The fixture's spec.description still references the old (incorrect) intent — left untouched to avoid drift; see `PR-S-PARAM-SYNTHESIZED` for the actual contract. | `PR-S-PARAM-SYNTHESIZED` |
| `pipeline-broken-tt.yaml` | Pipeline `krci-cli-e2e-broken-tt` | Carries an `app.edp.epam.com/triggertemplate` label pointing at a TT that does not exist. | `PR-S-TT-MISSING` |
| `pipeline-bare.yaml` | Pipeline `krci-cli-e2e-bare` | No KRCI labels at all. Proves the CLI can start any valid Tekton Pipeline regardless of the KubeRocketCI labelling convention. | `PR-S-BARE` |
| `pipeline-with-tt.yaml` | Pipeline `krci-cli-e2e-with-tt` | Paired with `trigger-template.yaml`. Exercises the TriggerTemplate branch of `createPipelineRunDraftFromPipeline` — placeholder resolution and label sanitization. | `PR-S-TT-OK`, `PR-S-TT-DRY` |
| `trigger-template.yaml` | TriggerTemplate `krci-cli-e2e-tt` | Resourcetemplate uses `$(tt.params.X)` placeholders that resolve against `pipeline-with-tt.yaml`'s param defaults. | (paired with above) |

## Apply

Substitute `<namespace>` with whatever namespace you target (e.g. the one your
portal session points at):

```sh
kubectl apply -n <namespace> -f source/cli/e2e/pipelinerun/fixtures/
```

Verify:

```sh
kubectl -n <namespace> get pipeline.tekton.dev,triggertemplate.triggers.tekton.dev \
  -l app.edp.epam.com/pipelinetype=tests
```

## Smoke test (after apply)

```sh
# Dry-run — proves CLI <-> portal plumbing without creating a PipelineRun.
./dist/krci pipelinerun start krci-cli-e2e-noop --dry-run -o json | jq .

# Real run with default params.
./dist/krci pipelinerun start krci-cli-e2e-noop -o json

# Real run overriding a param.
./dist/krci pipelinerun start krci-cli-e2e-noop \
  --param git-revision=develop --param count=3 -o json

# Deterministic failure path.
./dist/krci pipelinerun start krci-cli-e2e-noop --param should-fail=true -o json

# No-default-param path: succeeds with synthesised empty value (exit 0).
# Demonstrates that "missing required param" is not a reachable failure mode.
./dist/krci pipelinerun start krci-cli-e2e-required -o json

# TriggerTemplate-not-found path: synthesised "TT does not exist" message, exit 1.
./dist/krci pipelinerun start krci-cli-e2e-broken-tt

# Bare Tekton Pipeline (no KRCI labels) — must still start.
./dist/krci pipelinerun start krci-cli-e2e-bare -o json

# TriggerTemplate happy path — dry-run reveals resolved $(tt.params.X) values.
./dist/krci pipelinerun start krci-cli-e2e-with-tt --dry-run -o json | jq .

# TriggerTemplate happy path — real run.
./dist/krci pipelinerun start krci-cli-e2e-with-tt -o json
```

## Cleanup

The PipelineRun objects created by tests are subject to whatever GC the
cluster has configured (Tekton Results retention, operator pruning, etc.).
To remove the fixture resources themselves:

```sh
kubectl delete -n <namespace> -f source/cli/e2e/pipelinerun/fixtures/
```

To purge accumulated runs (label selector picks up runs from any of the
fixture pipelines because every fixture sets `pipelinetype: tests` on either
the Pipeline or — via TT-resolved labels — the resulting PipelineRun):

```sh
kubectl -n <namespace> delete pipelinerun.tekton.dev \
  -l app.edp.epam.com/pipelinetype=tests
```

## Why these manifests look the way they do

- **Labelled `pipelinetype: tests`.** Distinguishes these fixtures from
  KubeRocketCI's real pipelines (build/review/deploy/clean/security/release).
  Filter via `kubectl get pipeline.tekton.dev
  -l app.edp.epam.com/pipelinetype=tests` or the same selector in the portal
  UI. `tests` is already a valid value in the portal's `pipelineTypeEnum`.
- **`app.edp.epam.com/*` label keys are upstream KubeRocketCI definitions**
  and cannot be renamed — the portal reads exactly those keys to drive
  TriggerTemplate lookup, codebase filters, and pipelinetype filters. The
  values used in this bundle (`tests`, `krci-cli-e2e`, `""`) are synthetic
  and carry no organisation-specific data.
- **`triggertemplate` label is empty** on the `noop` and `required` fixtures.
  The portal's Pipeline Zod schema requires both labels to be present, but
  `getTriggerTemplateLabel` treats an empty string as "absent" and skips the
  TT lookup — so `start` works without us having to also create a TT.
  `pipeline-broken-tt.yaml` is the deliberate exception: its label points at
  a TT that does not exist, which is exactly the failure mode it tests.
- **`pipeline-with-tt.yaml` + `trigger-template.yaml` are a pair.** The
  Pipeline carries `triggertemplate: krci-cli-e2e-tt`; the matching TT's
  `resourcetemplates[0]` uses `$(tt.params.X)` placeholders. The portal's
  draft builder resolves those placeholders against the Pipeline's param
  defaults (not the TT's), so the resulting PipelineRun reflects values
  declared on the Pipeline side. This is the only fixture that exercises
  `createPipelineRunDraftFromPipeline`'s TT branch end-to-end.
- **Inline `taskSpec`, not `taskRef`.** Decouples the fixtures from
  cluster-installed Tasks/ClusterTasks — apply works on any cluster with
  Tekton Pipelines installed.
- **Params passed via env, not interpolated into shell.** Tekton substitutes
  `$(params.X)` as text before the shell runs. Routing through `env:` means
  param values land in shell variables, where `"$VAR"` does not re-expand
  command substitutions — so a hostile `--param message='$(reboot)'` is
  echoed literally rather than executed.
- **Pinned image tag (`busybox:1.36`).** Reproducible across runs.
