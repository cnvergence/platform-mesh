# Architecture

This document describes the internal components of the resource-sharding-operator, how they interact, and why key design decisions were made. For the full concept including scaling motivation, prior-art comparison, and design trade-offs, see [concept.md](concept.md).

## Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                    resource-sharding-operator                        │
│                                                                      │
│  ┌─────────────────────────┐  ┌───────────────────────────────────┐  │
│  │  ResourceSharding        │  │  Dynamic Controller (per CR)      │  │
│  │  Reconciler              │  │                                   │  │
│  │                          │  │  - Watches target GVK             │  │
│  │  - Validates target GVK  │  │    (PartialObjectMetadata only)   │  │
│  │  - Checks RBAC via SSAR  │  │  - Negative label selector:       │  │
│  │  - Starts/stops dynamic  │  │    "!sharding.../shard"           │  │
│  │    controllers           │  │  - Assigns shard label (RR)       │  │
│  │  - Manages webhook cfg   │  │  - Auto-evicts from cache after   │  │
│  │  - Runs periodic rebalance│ │    label is applied               │  │
│  │  - Updates CR status     │  └───────────────────────────────────┘  │
│  └─────────────────────────┘                                         │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐   │
│  │  Mutating Webhook (optional fast path)                        │   │
│  │                                                               │   │
│  │  - Intercepts CREATE for configured GVKs                      │   │
│  │  - Assigns shard label at admission time (immediate)          │   │
│  │  - failurePolicy: Ignore — never blocks resource creation     │   │
│  │  - Shares ShardAssigner state with the dynamic controller     │   │
│  └───────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────┘
```

## Components

### ResourceSharding Reconciler

`internal/controller/resourcesharding_reconciler.go`

The reconciler is triggered by every change to a `ResourceSharding` CR and also fires periodically at `spec.rebalance.interval` to run the rebalancer.

**Reconciliation steps (in order):**

1. **Finalizer** — adds `sharding.platform-mesh.io/cleanup` on first reconcile; removes it during deletion after stopping the dynamic controller and deleting the webhook configuration.
2. **Target validation** — calls the Discovery API to confirm the target GVR exists in the cluster. Sets `TargetNotFound` condition and requeues if not found.
3. **Uniqueness check** — lists all `ResourceSharding` objects and rejects the CR if another one already targets the same GVR. Sets `Conflict` condition.
4. **RBAC pre-check** — issues a `SelfSubjectAccessReview` for `get`, `list`, `watch`, and `patch` on the target GVR. Sets `PermissionsMissing` condition if any verb is denied. This prevents silent watch failures.
5. **Dynamic controller** — starts the dynamic controller for this GVR if one is not already running (idempotent via the registry).
6. **Webhook configuration** — creates or updates the `MutatingWebhookConfiguration` when `spec.webhook.enabled: true`; deletes it when disabled or the CR is deleted.
7. **Rebalance** — runs the `Rebalancer` to count resources per shard and redistribute if drift exceeds `spec.rebalance.threshold`.
8. **Status update** — writes distribution counts, `totalShards`, `lastRebalanceTime`, and the `Ready` condition.

**Error handling:** Target not found and permissions-missing states requeue at `spec.rebalance.interval` rather than returning an error, to avoid exponential backoff on persistent configuration problems.

### Dynamic Controller Registry

`internal/controller/registry.go`

A thread-safe map from `ResourceSharding.UID` to a `RunningController`. Shared between the reconciler and the webhook handler. Stores the cancel function, GVR, label key, and `ShardAssigner` for each running controller.

The webhook handler looks up the registry by GVR to find the right `ShardAssigner` when intercepting a CREATE request.

### Dynamic Controller

`internal/controller/dynamic_controller.go`

Started per `ResourceSharding` CR. Implemented as a dedicated informer cache + work queue rather than a controller-runtime controller, to avoid limitations with dynamic GVK registration at runtime.

**Key design:**

- Creates a `cache.New` instance with a **negative label selector** (`!<labelKey>`) scoped to `PartialObjectMetadata` for the target GVK. Only resources that do not yet have the shard label are ever streamed to this process.
- Registers an `AddEventHandler` on the informer before starting the cache, so the initial LIST results are enqueued immediately.
- Starts the cache in a goroutine; waits for the initial cache sync before processing.
- Work queue processes items by: fetching the object, checking whether the label already exists (defense-in-depth), calling `ShardAssigner.Next()`, and patching the label.
- On patch success: the API server detects the label no longer matches the `!key` selector and sends a synthetic DELETE event on the watch stream. The informer removes the object from cache automatically — no extra logic needed.
- On patch failure: the item is re-queued with rate limiting.
- Context cancellation (on CR deletion) shuts the queue down via a goroutine watching `ctrlCtx.Done()`.

### Shard Assigner

`internal/controller/shard_assigner.go`

Round-robin counter backed by an `atomic.Uint64`. The shard list itself is guarded by a mutex and updated when `spec.shards` changes.

### Rebalancer

`internal/controller/rebalancer.go`

Runs as part of every reconcile cycle. Uses `PartialObjectMetadataList` with `client.Limit(1)` plus `RemainingItemCount` to count resources per shard — O(1) API calls per shard regardless of resource count, with no full resource objects loaded into memory.

**Steps:**

1. **Count** — one LIST per shard with `labelKey=shardName`, limit 1, read `RemainingItemCount`.
2. **Orphan cleanup** — LIST resources where the label key exists but the value is not in `spec.shards`. Strips the label so the resource re-enters the dynamic controller's watch and is reassigned.
3. **Rebalance** — computes ideal count (`total / len(shards)`), identifies overloaded shards (exceeding `ideal * (1 + threshold/100)`), and moves resources from over- to under-loaded shards. Move count per cycle: `max(minMovesPerCycle, toMove * movesPerCycle%)`. Patches are rate-limited to `spec.rebalance.rateLimit` patches/second.
4. **Metrics** — updates `resource_sharding_distribution` and `resource_sharding_imbalance_ratio` gauges.

### Mutating Webhook Handler

`internal/controller/webhook_handler.go`

A single global handler is registered at `/mutate-shard-assign` on the manager's webhook server when `spec.webhook.enabled: true`. It intercepts CREATE operations.

1. Looks up the running controller for the incoming resource's group+resource in the registry.
2. If the resource already has the shard label, passes through unchanged.
3. Otherwise calls `ShardAssigner.Next()` from the shared assigner (same instance used by the dynamic controller) and returns a JSON patch adding the label.

The webhook uses `failurePolicy: Ignore`. If the webhook is unreachable, the API server admits the resource without the label. The dynamic controller picks it up within seconds.

### Webhook Configuration Manager

`internal/controller/webhook_config.go`

Creates and updates `MutatingWebhookConfiguration` objects. Each configuration is named `resource-sharding-<cr-name>` and owned by the `ResourceSharding` CR via an owner reference.

> **Note:** Today the generated `MutatingWebhookConfiguration` points at a per-CR path of the form `/mutate-shard-assign-<cr-name>`, but only the global `/mutate-shard-assign` endpoint is registered on the webhook server. This mismatch is tracked as a known issue and will be resolved by either registering per-CR paths or updating the configuration to use the shared endpoint.

### Prometheus Metrics

`internal/controller/metrics.go`

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `resource_sharding_distribution` | Gauge | `resourcesharding`, `shard` | Current resource count per shard |
| `resource_sharding_imbalance_ratio` | Gauge | `resourcesharding` | Max deviation from ideal (0.0 = balanced) |
| `resource_sharding_assignments_total` | Counter | `resourcesharding`, `shard` | Total shard assignments made |
| `resource_sharding_rebalance_moves_total` | Counter | `resourcesharding` | Total label patches applied during rebalance |

## Assignment Flow

```
Resource CREATE
      │
      ▼
Webhook available? ──Yes──▶ Assign label at admission (immediate)
      │
      No / Ignore
      │
      ▼
Resource created without shard label
      │
      ▼
Informer ADD event fires on dynamic controller watch
      │
      ▼
Work queue dequeues item
      │
      ▼
Re-fetch object, check label (defense-in-depth)
      │
      ▼
ShardAssigner.Next() → PATCH label onto object
      │
      ▼
API server sends DELETE event (label no longer matches !key selector)
      │
      ▼
Object auto-evicted from controller cache
```

## Status Conditions

| Condition type | When set | Meaning |
|----------------|----------|---------|
| `Ready=True` | Normal operation | Controllers running, rebalance completed |
| `Ready=False` | Any blocking error | See reason field |
| `TargetNotFound` | GVR not in cluster | Re-checked every `rebalance.interval` |
| `PermissionsMissing` | SSAR denied | Operator lacks `get/list/watch/patch` on target |
| `Conflict` | Duplicate target | Another `ResourceSharding` targets the same GVR |

## KCP / Multicluster Mode

When `kcp.enabled: true` in the Helm values, the operator initialises a `multicluster-runtime` manager backed by the KCP `apiexport` provider. The `ResourceSharding` CRD is published via an `APIExport`; the provider discovers consumer workspaces that bind it.

In single-cluster mode (the default), the provider is not used and the operator behaves identically to a standard controller-runtime operator.

## RBAC Model

The operator uses an **aggregated ClusterRole** for target GVK permissions. Users create per-GVK ClusterRoles labelled `sharding.platform-mesh.io/aggregate-to-operator: "true"`, which are automatically included in the operator's ClusterRole:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: resource-sharding-operator-myresources
  labels:
    sharding.platform-mesh.io/aggregate-to-operator: "true"
rules:
  - apiGroups: ["example.io"]
    resources: ["myresources"]
    verbs: ["get", "list", "watch", "patch"]
```

Before starting a dynamic controller, the reconciler verifies permissions via `SelfSubjectAccessReview`. Missing permissions surface as a `PermissionsMissing` status condition rather than a runtime error.
