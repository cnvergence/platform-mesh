# Resource Sharding Operator — Concept

## Problem Statement

When scaling Kubernetes operators horizontally, each replica typically watches and reconciles **all** instances of a given resource. This leads to contention, redundant work, and limits horizontal scalability. We need a mechanism to deterministically assign resources to specific operator shards so each shard only reconciles its subset.

## Goals

1. Declaratively define sharding for any GVK via a `ResourceSharding` custom resource.
2. Automatically label newly created resources with a shard assignment (round-robin).
3. Periodically rebalance shard assignments to maintain even distribution.
4. Shard operators filter on their assigned label — no code changes required in downstream operators.

## Non-Goals

- Replacing leader election (sharding is orthogonal).
- Handling stateful migration (e.g., draining in-flight work before reassignment).
- Providing the downstream shard controllers themselves — only the assignment mechanism.

---

## Benefits

### Scaling Dimensions

Sharding improves three primary scaling dimensions simultaneously:

#### 1. Memory — Reduced Cache Footprint

In controller-runtime, every watched resource is cached in-memory via the informer. Without sharding, every operator replica holds the **entire** resource set in its cache — even if multiple replicas run for HA or throughput.

With shard-based label selectors, the API server performs **server-side filtering** on the watch. Only resources matching the shard label are streamed to the operator:

| Metric | Without Sharding (3 replicas) | With Sharding (3 shards) |
|--------|-------------------------------|--------------------------|
| Resources in memory per replica | 100,000 | ~33,333 |
| Total cluster memory for operator | 3 × full set | 3 × (1/3 set) = **1× full set** |
| Watch event volume per replica | All events | ~1/3 of events |
| GC pressure per replica | High | Proportionally reduced |

**Key insight:** Label selectors on watches are evaluated **server-side by the API server** (via the etcd watch filter). The operator never receives, decodes, or allocates memory for resources belonging to other shards.

At scale (100k+ resources), this is the difference between operators that OOMKill and operators that fit in a 256 MB memory limit.

#### 2. Throughput — Parallel Reconciliation Across Shards

Without sharding, a single operator (or leader-elected set) must sequentially process the entire work queue. Even with concurrent workers, one process bottlenecks on CPU, network I/O, and rate limits.

With N shards, the total reconciliation work is **distributed across N independent processes**:
- Each shard's work queue is 1/N the size — items are dequeued and reconciled faster.
- Rate limits (e.g., API server QPS) apply per-client, so N shards get N × the effective throughput.
- CPU-intensive reconciliation (e.g., deep diffing, template rendering) parallelizes naturally across pods/nodes.

**Example:** An operator that takes 60 minutes to reconcile 100,000 resources sequentially finishes in ~12 minutes with 5 shards working in parallel (assuming no shared bottleneck).

#### 3. Startup Time — Faster Cache Sync

Before an operator can process work, controller-runtime must complete an initial **list + watch** to populate the informer cache. This is blocking — the operator is not ready until the cache is synced.

| Resources | Unsharded Initial Sync | 5-Shard Initial Sync |
|-----------|----------------------|---------------------|
| 10,000 | ~8s | ~1.6s |
| 100,000 | ~80s | ~16s |
| 1,000,000 | ~800s (13+ min) | ~160s (~2.5 min) |

The sync time scales linearly with object count because:
- The API server paginates LIST responses — fewer objects = fewer pages = fewer round trips.
- Deserialization and cache insertion is O(n) — 1/N objects means 1/N time.
- Less memory allocation during startup means less GC pressure during the critical ready path.

**Impact on availability:** Faster startup means faster rollouts, faster crash recovery, and HPA can scale shard replicas up/down without prolonged not-ready windows.

### Additional Benefits

1. **Reduced API server load**: Fewer watch events per connection means less serialization/encoding work on the API server side.
2. **Smaller failure blast radius**: If one shard's operator crashes, only 1/N resources are temporarily unmanaged instead of all of them.
3. **Independent scaling**: Shards can run on different node pools, scale independently, or even run different operator versions (canary deployments per shard).
4. **Lower network bandwidth**: Watch streams carry only the shard's subset — significant in multi-AZ or edge deployments where cross-zone traffic has cost implications.

### How It Works (Server-Side Filtering)

```
┌──────────────┐         ┌─────────────┐         ┌─────────────────┐
│  API Server  │         │   Watch     │         │  Shard Operator  │
│              │         │   Filter    │         │  (shard-a)       │
│  etcd watch  │────────▶│  label ==   │────────▶│  Cache: only     │
│  (all events)│         │  "shard-a"  │         │  shard-a objects │
│              │         └─────────────┘         └─────────────────┘
│              │
│              │         ┌─────────────┐         ┌─────────────────┐
│              │         │   Watch     │         │  Shard Operator  │
│              │────────▶│   Filter    │────────▶│  (shard-b)       │
│              │         │  label ==   │         │  Cache: only     │
│              │         │  "shard-b"  │         │  shard-b objects │
└──────────────┘         └─────────────┘         └─────────────────┘
```

The filtering happens at the API server's watch multiplexer — it never touches the operator process. This is fundamentally different from client-side filtering (where all objects are received and then discarded), which provides no memory benefit.

---

## Custom Resource: `ResourceSharding`

```yaml
apiVersion: sharding.platform-mesh.io/v1alpha1
kind: ResourceSharding
metadata:
  name: myresource-sharding
spec:
  # The resource to shard
  target:
    group: example.io
    version: v1
    resource: myresources   # plural name

  # Label key applied to target resources
  shardLabelKey: sharding.platform-mesh.io/shard  # optional, has default

  # Shard definitions
  shards:
    - name: shard-a
    - name: shard-b
    - name: shard-c

  # Rebalance configuration
  rebalance:
    interval: 5m            # how often to check distribution
    threshold: 20           # percentage imbalance before triggering rebalance
    movesPerCycle: 10%      # percentage of imbalanced resources to move per cycle
    minMovesPerCycle: 10    # floor — move at least this many per cycle
    rateLimit: 10           # max patches per second during rebalance

  # Optional webhook for fast-path assignment
  webhook:
    enabled: true           # default: false (controller-only mode)


status:
  # Current distribution snapshot
  distribution:
    - shard: shard-a
      count: 34
    - shard: shard-b
      count: 31
    - shard: shard-c
      count: 35
  lastRebalanceTime: "2026-04-30T10:00:00Z"
  conditions:
    - type: Ready
      status: "True"
      reason: ControllersRunning
      message: "Watching myresources.example.io"
```

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│              Resource Sharding Operator                                   │
│                                                                          │
│  ┌─────────────────────┐  ┌─────────────────────────────────────────┐   │
│  │  ResourceSharding       │  │  Dynamic Controller (fallback)          │   │
│  │  Reconciler          │  │  (per ResourceSharding)                    │   │
│  │                      │  │                                         │   │
│  │  - Validates spec    │  │  - Watches target GVK via               │   │
│  │  - Starts/stops      │  │    PartialObjectMetadata                │   │
│  │    dynamic controllers│ │  - Negative label selector:             │   │
│  │  - Manages webhook   │  │    "!sharding.../shard"                 │   │
│  │    configuration     │  │  - Catches anything the webhook missed  │   │
│  │  - Periodic rebalance│  │  - Assigns shard label (round-robin)    │   │
│  │    (via LIST calls)  │  │  - Near-zero cache in steady state      │   │
│  │  - Updates status    │  │    (webhook handles the fast path)      │   │
│  └─────────────────────┘  └─────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  Mutating Webhook (optional, independently scalable)              │   │
│  │                                                                    │   │
│  │  - Intercepts CREATE for target GVKs                              │   │
│  │  - Assigns shard label at admission time (immediate)              │   │
│  │  - failurePolicy: Ignore (never blocks resource creation)         │   │
│  │  - Can be scaled independently from the operator                  │   │
│  │  - If unavailable: resource created without label → controller    │   │
│  │    picks it up as fallback                                        │   │
│  └──────────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────────┘

        ┌─────────────────────────────────────────────────┐
        │            Assignment Flow                        │
        │                                                  │
        │  Resource CREATE                                 │
        │       │                                          │
        │       ▼                                          │
        │  Webhook available? ───Yes──▶ Label at admission │
        │       │                        (fast path)       │
        │       No                                         │
        │       │                                          │
        │       ▼                                          │
        │  Created without label                           │
        │       │                                          │
        │       ▼                                          │
        │  Controller watch fires ──▶ Label via patch      │
        │       (fallback path, seconds delay)             │
        └─────────────────────────────────────────────────┘
```

### Component 1: ResourceSharding Reconciler

Triggered by changes to `ResourceSharding` CRs and periodically (rebalance interval).

**Responsibilities:**

1. **Validate** the target GVK exists in the cluster (via discovery).
2. **Validate uniqueness** — reject if another `ResourceSharding` already targets the same GVK. Set a `Conflict` status condition and do not start a controller. Enforced at reconcile time (check for existing ResourceShardings with the same target) and optionally via a validating webhook for fast feedback.
3. **Start a dynamic controller** for the target GVK if one isn't already running.
4. **Manage webhook configuration** — if webhook is enabled, create/update `MutatingWebhookConfiguration` for the target GVK.
5. **Periodic rebalance** — every `spec.rebalance.interval`:
   - List all target resources (partial metadata only).
   - Count resources per shard label value.
   - If imbalance exceeds `spec.rebalance.threshold`, reassign labels to equalize.
6. **Update status** with current distribution counts.
7. **Stop the dynamic controller** and remove webhook config if the `ResourceSharding` is deleted (finalizer).

### Component 2: Mutating Webhook (optional, fast path)

An optional admission webhook that assigns the shard label at resource creation time.

**Behavior:**

- Intercepts `CREATE` operations on the target GVK.
- If the resource already has the shard label → no-op (pass through).
- If unlabeled → assign the next shard value (round-robin) and inject the label into the admission response.
- `failurePolicy: Ignore` — if the webhook is unreachable, the API server allows the CREATE without modification. The resource is created unlabeled.
- Can be deployed and scaled independently from the operator (separate Deployment).
- Shares the round-robin counter state with the operator (or maintains its own — convergence happens via rebalance anyway).

**Why `Ignore` and not `Fail`:**
- The controller is always running as a safety net. Blocking resource creation for a sharding concern is never acceptable.
- Webhook downtime = brief "unlabeled window" resolved by the controller within seconds.
- This is explicitly not on the critical path — it's an optimization, not a gate.

### Component 3: Dynamic Controller (fallback + catch-all)

A controller dynamically registered at runtime that watches the target GVK. Uses `controller.NewUnmanaged()` (controller-runtime v0.15+) — started in a goroutine with a cancellable context, managed by the ResourceSharding reconciler (following the KCP api-syncagent pattern).

**Behavior:**

- Uses `PartialObjectMetadata` watches (metadata-only, low overhead).
- **Negative label selector** on the watch: `!sharding.platform-mesh.io/shard` — the API server only streams resources that do NOT have the shard label. This is server-side filtering; unlabeled resources are the only objects in cache.
- On reconcile: **re-check the label before patching** (defense-in-depth), then add the shard label with the next shard value (round-robin).
- Once the label is applied, the API server sends a synthetic DELETE event on the filtered watch — the resource **auto-evicts from cache** with no additional logic needed.
- **When webhook is healthy:** cache is empty (webhook labels everything at admission). The controller is idle but watching — zero cost.
- **When webhook is unavailable:** cache fills with unlabeled resources and the controller takes over assignment. Self-healing, no manual intervention.

**No race between webhook and controller:** The webhook is part of the admission chain — the object is not persisted to etcd until admission completes. If the webhook labels it, the first watch event already has the label, and the `!key` selector filters it server-side. The controller never sees objects that the webhook handled. The re-check before patching is defense-in-depth for edge cases (e.g., manual label application racing with queue processing, or stale queue items after restart).

**Why negative selector over a predicate:**

| Approach | Memory | Server-side | Auto-eviction |
|----------|--------|-------------|---------------|
| Client-side predicate | All resources in cache, filtered at enqueue | No | No — stays in cache |
| Negative label selector (`!key`) | Only unlabeled resources in cache | Yes | Yes — removed on label patch |

The predicate approach still receives and caches all objects (the informer holds them regardless of whether they're enqueued). The negative selector ensures unmatched objects are never sent to the operator process at all.

---

## Detailed Design

### Negative Label Selector Watch

The dynamic controller registers its cache/informer with a field or label selector that excludes already-assigned resources:

```go
import "k8s.io/apimachinery/pkg/labels"

// Server-side: only resources WITHOUT the shard label
selector, _ := labels.Parse("!sharding.platform-mesh.io/shard")

// Used when constructing the informer for the target GVK
cache.Options{
    ByObject: map[client.Object]cache.ByObject{
        &metav1.PartialObjectMetadata{
            TypeMeta: metav1.TypeMeta{
                APIVersion: gv.String(),
                Kind:       kind,
            },
        }: {
            Label: selector,
        },
    },
}
```

**Lifecycle of a resource in this watch:**
1. Resource created (no shard label) → API server streams it → enters cache → enqueued.
2. Reconciler patches the shard label onto the resource.
3. API server detects label no longer matches `!key` → sends DELETE event on this watch stream.
4. Informer removes resource from cache. Done.

No predicate needed — the watch itself is the filter.

### Shard Assignment (Round-Robin)

The operator maintains an in-memory counter per `ResourceSharding` to track which shard to assign next. On restart, the counter resets and the next assignment uses the shard with the fewest current resources (graceful recovery).

```go
func (r *DynamicReconciler) nextShard() string {
    r.mu.Lock()
    defer r.mu.Unlock()
    shard := r.shards[r.counter % len(r.shards)]
    r.counter++
    return shard.Name
}
```

### Rebalancing Strategy

Rebalancing runs on a timer (e.g., every 5 minutes) and uses **single-page LIST calls with `limit=1`** to get counts via `RemainingItemCount`. This is O(1) memory per shard regardless of resource count — one API call per shard, no pagination needed for counting.

Exact accuracy is not required. `RemainingItemCount` is an estimate from etcd and may be slightly stale — that's fine. The goal is to detect meaningful skew and correct it, not maintain a perfectly real-time count.

```go
func (r *ResourceShardingReconciler) rebalance(ctx context.Context, rs *v1alpha1.ResourceSharding) error {
    // Single-page LIST per shard with limit=1 to get RemainingItemCount
    // Avoids loading all items into memory — O(1) per shard regardless of resource count
    counts := make(map[string]int, len(rs.Spec.Shards))
    for _, shard := range rs.Spec.Shards {
        selector, _ := labels.Parse(rs.Spec.ShardLabelKey + "=" + shard.Name)
        list := &metav1.PartialObjectMetadataList{}
        list.SetGroupVersionKind(targetGVK)
        if err := r.Client.List(ctx, list,
            client.MatchingLabelsSelector{Selector: selector},
            client.Limit(1),
        ); err != nil {
            return err
        }
        count := len(list.Items)
        if list.RemainingItemCount != nil {
            count += int(*list.RemainingItemCount)
        }
        counts[shard.Name] = count
    }
    // Assess and rebalance...
}
```

**Steps:**
1. For each shard in `spec.shards`, LIST target resources with `labelKey=shardName` (metadata only, `limit=1` + `RemainingItemCount`).
2. **Detect orphaned assignments:** LIST target resources where the shard label key exists but the value does NOT match any defined shard (e.g., from backup restores, removed shards, manual edits). Strip the label from these resources — they re-enter the negative-selector watch and get reassigned via the normal path.
3. Compute ideal count: `total / len(shards)`.
4. If any shard exceeds `ideal * (1 + threshold/100)`:
   - Determine resources to move from over-represented shards to under-represented ones.
   - Pick resources to move deterministically (e.g., alphabetical by name).
   - **Compute moves for this cycle:** `max(minMovesPerCycle, toMove * movesPerCycle%)`
   - **Rate-limit patches** to `rateLimit` per second to avoid spiking the API server.
5. Patch moved resources with the new shard label (rebalance) or strip invalid labels (orphan cleanup).

**Gradual convergence:** Each cycle moves a percentage of the outstanding imbalance, with a floor to ensure progress. This scales naturally — large imbalances move more resources per cycle (in absolute terms), small imbalances still converge without stalling.

**Example — shard removal with 10,000 resources (10%, min 10):**
```
Cycle 1:  10,000 to move → 10% = 1,000 moves
Cycle 2:   9,000 to move → 10% =   900 moves
Cycle 3:   8,100 to move → 10% =   810 moves
...
Cycle 20:    ~135 to move → 10% =    13 moves
...
Cycle 40:     ~8 to move  → min  =    10 moves (floor kicks in, done)
```

At `interval: 5m`, the first 90% converges in ~2 hours. The tail converges shortly after. This is proportional — a shard removal with 100 resources finishes in a few cycles.

**Example — small imbalance of 30 resources:**
```
Cycle 1:  30 to move → 10% = 3 → floor = 10 moves
Cycle 2:  20 to move → 10% = 2 → floor = 10 moves
Cycle 3:  10 to move → 10% = 1 → floor = 10 moves (done)
```

Small imbalances resolve in 1-3 cycles regardless of percentage.

**Concern:** Reassigning a label causes the downstream operator to re-reconcile the resource. This is acceptable — downstream operators should be idempotent. The percentage-based moves and rate limit ensure this doesn't overwhelm them.

### PartialObjectMetadata Usage

Watching via `PartialObjectMetadata` avoids loading full resource specs into memory:

```go
controller.For(&metav1.PartialObjectMetadata{
    TypeMeta: metav1.TypeMeta{
        APIVersion: gvk.GroupVersion().String(),
        Kind:       gvk.Kind,
    },
}, builder.OnlyMetadata)
```

### Dynamic Controller Lifecycle

Uses the **KCP api-syncagent pattern**: unmanaged controllers started in goroutines with cancellable contexts.

```go
// ResourceSharding reconciler maintains a registry of running controllers
type Reconciler struct {
    cancels map[types.UID]context.CancelFunc
    // ...
}

// Start a dynamic controller for a new ResourceSharding
func (r *Reconciler) startController(ctx context.Context, rs *v1alpha1.ResourceSharding) error {
    ctrlCtx, cancel := context.WithCancelCause(ctx)

    ctrl, err := controller.NewUnmanaged("shard-"+rs.Name, r.Manager, controller.Options{
        Reconciler: &DynamicReconciler{shards: rs.Spec.Shards, ...},
    })
    if err != nil {
        cancel(err)
        return err
    }

    // Watch target GVK with negative label selector (metadata-only)
    ctrl.Watch(source.Kind(r.Manager.GetCache(), &metav1.PartialObjectMetadata{...},
        handler.EnqueueRequestForObject{},
    ))

    go ctrl.Start(ctrlCtx)
    r.cancels[rs.UID] = cancel
    return nil
}

// Stop when ResourceSharding is deleted
func (r *Reconciler) stopController(rs *v1alpha1.ResourceSharding) {
    if cancel, ok := r.cancels[rs.UID]; ok {
        cancel(errors.New("ResourceSharding deleted"))
        delete(r.cancels, rs.UID)
    }
}
```

| Event | Action |
|-------|--------|
| `ResourceSharding` created | Discover GVK, start unmanaged controller, add finalizer |
| `ResourceSharding` updated | Cancel old controller, start new one with updated shard list |
| `ResourceSharding` deleted | Cancel controller, remove webhook config, remove finalizer |

---

## RBAC Requirements

The operator uses **aggregated ClusterRoles** for target GVK permissions. The operator ships with a base ClusterRole that aggregates roles labeled with `sharding.platform-mesh.io/aggregate-to-operator: "true"`. Users create per-GVK ClusterRoles with that label to grant access.

**Base ClusterRole (shipped with operator):**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: resource-sharding-operator
aggregationRule:
  clusterRoleSelectors:
    - matchLabels:
        sharding.platform-mesh.io/aggregate-to-operator: "true"
rules: [] # aggregated automatically
```

**Per-GVK ClusterRole (created by user per target):**
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

**RBAC pre-check:** Before starting a dynamic controller, the reconciler performs a `SelfSubjectAccessReview` for the target GVK (watch + patch). If permissions are missing, it sets a `PermissionsMissing` status condition and does not start the controller. This prevents silent failures where the informer can't connect.

**Operator's own CRD permissions (always required):**
```yaml
- apiGroups: ["sharding.platform-mesh.io"]
  resources: ["resourceshardingings"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["sharding.platform-mesh.io"]
  resources: ["resourceshardingings/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["sharding.platform-mesh.io"]
  resources: ["resourceshardingings/finalizers"]
  verbs: ["update"]
- apiGroups: ["authorization.k8s.io"]
  resources: ["selfsubjectaccessreviews"]
  verbs: ["create"]
```

---

## Prior Art and Comparison

### Existing Implementations

#### 1. timebertt/kubernetes-controller-sharding (SAP Gardener)

**Repo:** https://github.com/timebertt/kubernetes-controller-sharding  
**Origin:** Tim Ebert's master thesis at SAP (Gardener team), presented at KubeCon EU 2025.

- Central **Sharder** component deployed via a `ControllerRing` CRD.
- Uses **consistent hash ring** to assign objects to shards (not round-robin).
- Assignment via **mutating webhook** — the API server calls the sharder during admission for objects lacking the shard label.
- Shard membership announced via individual **Lease** objects (replaces leader election).
- Rebalancing uses a **drain label protocol**: sharder adds drain label → old shard acknowledges by removing both labels → webhook reassigns.
- Demonstrated horizontal scalability up to ~9,000 objects / 300 changes per second in load tests.

#### 2. KubeVela Controller Sharding

**Docs:** https://kubevela.io/docs/platform-engineers/system-operation/controller-sharding/

- **Master controller** runs webhooks and watches apps labeled `scheduled-shard-id: master`.
- **Slave controllers** filter by `controller.core.oam.dev/scheduled-shard-id=<shard-id>`.
- Mutating webhook on master assigns shard to applications without the label.
- Shard discovery is dynamic — master watches pods with `shard-id` labels; only Ready pods are schedulable.
- No auto-rescheduling if a shard fails — requires manual relabeling.

#### 3. Flux CD Sharding

**Docs:** https://fluxcd.io/flux/installation/configuration/sharding/

- Each controller type can be deployed as multiple instances.
- Each shard uses `--watch-label-selector=sharding.fluxcd.io/key=shard1`.
- Default controller uses negated selector `--watch-label-selector='!sharding.fluxcd.io/key'` for unlabeled resources.
- **No automatic assignment** — labels applied manually or via scripts/Kustomize overlays.
- No rebalancing mechanism.

#### 4. AppsCode Operator Shard Manager

**Artifact Hub:** https://artifacthub.io/packages/helm/appscode/operator-shard-manager

- `ShardConfiguration` CRD specifying controllers and target resources.
- **Consistent hashing with bounded loads** to distribute resources across StatefulSet pods.
- Labels resources with `shard.operator.k8s.appscode.com/<controller-name>: "<pod-index>"`.
- Auto-discovers ready pods and assigns resources accordingly.
- Production-proven with KubeDB (database operator).

#### 5. Argo CD Application Controller Sharding

**Docs:** https://argo-cd.readthedocs.io/en/latest/operator-manual/dynamic-cluster-distribution/

- Application Controller runs as a **StatefulSet**; each pod shards by ordinal.
- Shards are assigned **clusters** (not individual applications) — different granularity.
- Two algorithms: `legacy` (hash-based on cluster server URL) or `round-robin`.
- Dynamic scaling: auto-rebalances based on cluster count.

#### 6. kube-state-metrics Sharding

- MD5 hash of object UID mod total-shards.
- Every instance still fetches **all objects** from API server — no server-side filtering.
- Only splits metric emission, not watch/cache overhead.

---

### Comparison

| Aspect | **This Design** | timebertt | KubeVela | Flux | AppsCode |
|--------|----------------|-----------|----------|------|----------|
| Assignment mechanism | Webhook (fast path) + Controller (fallback) | Mutating webhook only | Mutating webhook only | Manual | Controller |
| Assignment algorithm | Round-robin | Consistent hashing | Selection from ready pods | N/A (manual) | Consistent hashing (bounded loads) |
| Server-side filtering (sharding operator) | Yes — only unlabeled resources in cache | N/A (webhook, no watch needed) | N/A (webhook) | N/A | Yes |
| Server-side filtering (downstream) | Yes | Yes | Yes | Yes | Yes |
| Auto-eviction from cache on assignment | Yes (label triggers DELETE event on negated watch) | N/A | N/A | N/A | N/A |
| Rebalancing | Periodic LIST + threshold | Drain label protocol | None | None | Automatic |
| Webhook failure policy | `Ignore` (never blocks) | `Fail` (blocks creation) | `Fail` | N/A | N/A |
| Fallback when webhook is down | Controller takes over (self-healing) | Creation blocked or unsharded | Creation blocked | N/A | N/A |
| Coordinator memory footprint | Near-zero (only unlabeled backlog) | Constant (watches Leases only) | Watches all apps + pods | None | Watches target resources |
| Reassignment on shard add/remove | Rebalance moves excess resources | ~K/N keys remap (consistent hashing) | Manual | Manual | ~K/N keys remap |
| Downstream code changes | ~3 lines (label selector on cache) | ~50 lines | Moderate | None (CLI flag) | Minimal |
| Assignment latency (happy path) | Immediate (webhook) | Immediate (webhook) | Immediate (webhook) | N/A | Reconcile loop |
| Assignment latency (degraded) | Seconds (controller fallback) | Blocked or unassigned | Blocked | N/A | N/A |
| Webhook required for operation | No (optional optimization) | Yes (critical) | Yes (critical) | No | No |

### Key Differentiators of This Design

1. **Hybrid approach: webhook + controller fallback.** Combines immediate assignment (webhook, fast path) with guaranteed eventual assignment (controller, fallback). The webhook is an optimization — the system works without it.

2. **Non-blocking webhook (`Ignore` policy).** Unlike timebertt and KubeVela where the webhook is on the critical path (`Fail` policy), this design never blocks resource creation. If the webhook is down, resources are created normally and the controller assigns them within seconds.

3. **Independent scaling.** The webhook can be scaled horizontally (separate Deployment) to handle high-throughput CREATE scenarios without scaling the operator itself. The operator stays lightweight — it only processes the fallback backlog.

4. **Near-zero coordinator memory.** The negative label selector means the operator's cache only holds the transient backlog of unassigned resources. When the webhook is healthy, this is empty. When degraded, it's proportional to the brief backlog — never the full resource set.

5. **Self-healing.** If both webhook and operator restart, unlabeled resources accumulate and are processed on recovery. No resources are lost or stuck. No manual intervention needed.

6. **Graceful degradation, not failure.** The system has three operating modes, all functional:

| Mode | Webhook | Controller | Behavior |
|------|---------|------------|----------|
| Full (default) | Healthy | Running | Immediate assignment, empty controller cache |
| Degraded | Down | Running | Controller assigns (seconds delay) |
| Minimal | Not deployed | Running | Controller-only mode, always works |

### Tradeoffs vs. Pure Webhook Approaches (timebertt, KubeVela)

| Concern | Pure Webhook | This Design (Hybrid) |
|---------|--------------|---------------------|
| Assignment latency | Immediate | Immediate (webhook) / seconds (fallback) |
| "Unlabeled" window | Never | Never (webhook healthy) / brief (webhook down) |
| Webhook downtime impact | Creation blocked or unsharded | No impact — controller covers |
| Operational complexity | Webhook + certs + `Fail` policy | Webhook optional; `Ignore` policy; controller always present |
| Scaling options | Scale the webhook | Scale webhook OR controller independently |
| Cert management | Required, critical | Required if webhook enabled, but not critical (failure = fallback) |

### Tradeoffs vs. Consistent Hashing

| Concern | Consistent Hashing | Round-Robin + Rebalance (this design) |
|---------|-------------------|---------------------------------------|
| Source of truth | Algorithm (derived from identity + ring) | Label value (concrete, written once) |
| Statefulness | Stateless (deterministic) | Stateful counter (recoverable from distribution) |
| Shard addition churn | ~1/N resources remap | No immediate effect; rebalance redistributes |
| Shard removal churn | ~1/N resources remap | Same — rebalance moves removed shard's resources |
| Compatibility with rebalancing | Conflicts — hash fights rebalance | No conflict — assignments are freely movable |
| Self-healing behavior | Always returns to hash position | Applies current strategy (no "home position") |
| Strategy evolution | Changing algorithm requires migration | Strategy is internal; label is the contract |
| Future flexibility | Locked to hash ring | Can swap to load-aware, priority-based, etc. |

**Bottom line:** Consistent hashing optimizes for minimal churn during topology changes. But in this design, topology changes (shard add/remove) are infrequent operational events, not continuous. Round-robin with periodic rebalancing provides the same outcome (even distribution) with more flexibility and simpler interactions with the rest of the system.

### Lessons Learned from Prior Art

| Pitfall | Description | Mitigation in this design |
|---------|-------------|---------------------------|
| Thundering herd on rebalance | Mass reassignment causes burst reconciliations on receiving shards | Rate-limit label patches during rebalance |
| Stale counter on restart | Round-robin counter resets, causing temporary imbalance | On restart, resume from least-loaded shard |
| Label conflicts | Downstream operators accidentally removing shard label | Self-healing: resource re-enters watch and gets reassigned |
| API server load during rebalance | Bulk LIST + PATCH spikes | Paginate LISTs, rate-limit patches, metadata-only |
| Webhook unavailability | Blocks resource creation | `Ignore` policy + controller fallback — never blocks |
| Cross-shard references | Resources referencing objects in another shard | Design sharding boundaries to keep related resources together |

---

## Design Decisions

1. **Cluster-scoped CRD.** `ResourceSharding` is cluster-scoped. It supports both cluster-scoped and namespace-scoped target GVKs with a single configuration. No need for per-namespace duplication.

2. **Shard removal → strip label, reassign via normal path.** When a shard is removed from `spec.shards`, the rebalance loop strips the now-invalid shard label from affected resources. They re-enter the negative-selector watch and get assigned to a remaining shard via the normal round-robin path. No special redistribution logic needed.

3. **No label protection mechanism.** If a controller or user removes the shard label, the resource immediately re-enters the negative-selector watch (matches `!key`) and gets reassigned. The system is self-healing — no SSA field managers, webhooks, or convention enforcement required.

4. **No drain mode in v1alpha1.** Removing a shard strips labels and triggers reassignment. Downstream operators are expected to be idempotent, so re-reconciliation on the new shard is safe. A drain protocol can be added later if needed.

5. **Round-robin assignment (label-as-truth model).** The concrete label value is the assignment — the strategy that produced it is an internal implementation detail.

   **Why not consistent hashing:**
   - Hashing makes the algorithm the source of truth (assignment is derived at runtime). This conflicts with rebalancing — if you move a resource away from its hash position, self-healing would put it back.
   - Hashing is static: changing the strategy requires migrating all assignments.
   - With a concrete label, the strategy can evolve freely (round-robin today, load-aware tomorrow, priority-based later) without affecting downstream operators or requiring migration.

   **Why round-robin works here:**
   - Naturally even distribution for new resources.
   - No conflict with rebalancing — assignments are arbitrary and freely movable.
   - Self-healing (label removal) just re-enters the current strategy — no "home position" to fight.
   - On restart, counter recovers from current distribution (assign to least-loaded shard).
   - Strategy is per-`ResourceSharding` — different resources can use different strategies in future versions.

6. **Prometheus metrics.** Expose per-shard resource counts as gauges from the rebalance loop. Enables alerting on imbalance without requiring external tooling.
   ```
   resource_shard_distribution{resourcesharding="myresource-sharding", shard="shard-a"} 34
   resource_shard_imbalance_percent{resourcesharding="myresource-sharding"} 8.2
   ```

---

## Downstream Operator Integration

Downstream operators **must propagate the shard label to all owned/child resources** they create. Without this, each shard operator would cache ALL sub-resources cluster-wide — defeating the purpose of sharding.

**Example:** 1M primary resources create 3M sub-resources (Pods, Services, ConfigMaps).
- **Without propagation:** Each shard operator caches 10k primaries + 3M secondaries = sharding fails.
- **With propagation:** Each shard operator caches 10k primaries + 30k secondaries = full memory isolation.

**Integration requirement:** When creating owned resources, copy the shard label from the parent:

```go
func (r *MyReconciler) createOwnedPod(parent *v1alpha1.MyResource) *corev1.Pod {
    pod := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Labels: map[string]string{
                "sharding.platform-mesh.io/shard": parent.Labels["sharding.platform-mesh.io/shard"],
                // ... other labels
            },
        },
        // ...
    }
    controllerutil.SetControllerReference(parent, pod, r.Scheme)
    return pod
}
```

With the shard label on all resources (primary + owned), use `DefaultLabelSelector` for full cache isolation:

```go
mgr, err := ctrl.NewManager(cfg, ctrl.Options{
    Cache: cache.Options{
        DefaultLabelSelector: labels.SelectorFromSet(labels.Set{
            "sharding.platform-mesh.io/shard": os.Getenv("SHARD_NAME"),
        }),
    },
})
```

This ensures:
- Primary resources are filtered server-side — only the shard's subset is cached.
- Owned resource watches work — children have the label, so they're in cache. `Owns()` handler maps child events to parent via ownerReference.
- Memory scales as `(primaries + secondaries) / N` per shard — full isolation.

**If label propagation is not feasible** (e.g., third-party operator you can't modify), fall back to `ByObject` selectors on the primary only. Secondaries will be fully cached — less optimal but functional:

```go
cache.Options{
    ByObject: map[client.Object]cache.ByObject{
        &v1alpha1.MyResource{}: {
            Label: shardSelector,  // only filter the primary
        },
        // Owned resources — no selector, fully cached per shard
    },
}
```

### Rollout Procedure

The sharding operator is additive — it labels resources without affecting existing operator behavior. Safe rollout:

1. **Deploy sharding operator** and create the `ResourceSharding` CR.
2. **Wait for initial assignment to complete.** Monitor via `status.distribution` (sum of counts equals total resources) or the `AssignmentComplete` condition. Downstream operators continue running unchanged — they still watch all resources.
3. **Reconfigure downstream operators** with the shard label selector and deploy as N shard replicas.

At no point are resources unmanaged. The sharding operator is purely additive until downstream operators opt in to filtering.

---

## Multi-Cluster and KCP Support

### Design Principle: Config Cluster vs. Resource Cluster

The operator is designed to support a separation between where configuration lives and where target resources live:

- **Config cluster:** Where `ResourceSharding` CRDs are created and managed. In a KCP deployment, this is a KCP workspace. In a single-cluster deployment, this is the same cluster as the resources.
- **Resource cluster(s):** Where the actual target resources (the ones being sharded) live. These are the physical workload clusters.

```
┌─────────────────────┐         ┌──────────────────────────────┐
│   Config Cluster     │         │   Resource Cluster(s)         │
│   (KCP workspace     │         │   (Physical clusters)         │
│    or same cluster)  │         │                               │
│                      │         │  - Target resources live here │
│  - ResourceSharding CRs │────────▶│  - Shard labels applied here  │
│  - Operator watches  │         │  - Dynamic controllers watch  │
│    these for config  │         │    here with negative selector │
└─────────────────────┘         │  - Downstream operators run    │
                                │    here with label selector    │
                                └──────────────────────────────┘
```

### Foundation: multicluster-runtime

The operator uses `sigs.k8s.io/multicluster-runtime` as its manager foundation (standard across all platform-mesh operators). This provides:

1. **`mcmanager.New()`** — a manager that can discover and connect to multiple clusters.
2. **Cluster provider** — pluggable mechanism for cluster discovery:
   - `github.com/kcp-dev/multicluster-provider/apiexport` for KCP (discovers virtual workspaces via APIExport).
   - Static provider for single-cluster or explicit multi-cluster setups.
3. **Multicluster controllers** — controllers that receive cluster context and can operate across cluster boundaries.
4. **Local manager access** — `mgr.GetLocalManager()` for resources that live on the config cluster (webhooks, leader election).

### Manager Setup

```go
import (
    "github.com/kcp-dev/multicluster-provider/apiexport"
    mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

// Config cluster connection (where ResourceSharding CRDs live)
configCfg := ctrl.GetConfigOrDie()

// Cluster provider — discovers resource clusters
// In KCP mode: discovers virtual workspaces via APIExport
// In single-cluster mode: the config cluster IS the resource cluster
provider, err := apiexport.New(configCfg, apiExportEndpointSliceName, apiexport.Options{
    Scheme: scheme,
})

// Multicluster manager
mgr, err := mcmanager.New(configCfg, provider, mcmanager.Options{
    Scheme:         scheme,
    LeaderElection: true,
    // ...
})
```

### How It Maps to the Sharding Operator

| Component | Cluster | Why |
|-----------|---------|-----|
| ResourceSharding reconciler | Config cluster (local manager) | Watches config CRDs |
| Dynamic controller (assignment) | Resource cluster(s) | Watches + patches target resources |
| Rebalance LIST calls | Resource cluster(s) | Counts target resources per shard |
| Mutating webhook | Resource cluster(s) | Intercepts CREATE on target GVKs |
| Leader election | Config cluster (local) | Prevents duplicate reconciliation |
| SSAR pre-check | Resource cluster(s) | Verifies permissions on target |

### Single-Cluster Mode (Default)

In single-cluster mode, the config cluster and resource cluster are the same. The multicluster-runtime provider simply provides the local cluster as the only "discovered" cluster. No KCP dependencies are needed at runtime — the operator works identically to a standard controller-runtime operator.

### KCP Mode

In KCP mode:
- The `ResourceSharding` CRD is published via an `APIExport` in a KCP workspace.
- The multicluster provider discovers consumer workspaces that bind the APIExport.
- The dynamic controller starts watches on each consumer workspace's resources.
- Each workspace can have different target resources, independently sharded.

This enables a **platform team** to define sharding policies centrally (in the provider workspace) that are applied across all consumer workspaces.

### Impact on Dynamic Controllers

The dynamic controller (per-ResourceSharding) needs to watch target resources on the resource cluster, not the config cluster. With multicluster-runtime, the controller receives a `multicluster.Cluster` representing the resource cluster:

```go
// The dynamic controller gets a cluster-scoped client for the resource cluster
resourceClient := cluster.GetClient()
resourceCache := cluster.GetCache()

// Negative label selector watch on the resource cluster's cache
ctrl.Watch(source.Kind(resourceCache, obj, handler))

// Patches go to the resource cluster
resourceClient.Patch(ctx, obj, patch)
```

In single-cluster mode, this is equivalent to using the local manager's client and cache. The abstraction allows the same code to work in both modes.

---

| Component | Trigger | Action |
|-----------|---------|--------|
| ResourceSharding Reconciler | CR change | Start/stop dynamic controller |
| ResourceSharding Reconciler | Timer (interval) | Rebalance distribution |
| Dynamic Controller | New unlabeled resource | Assign shard label (round-robin) |
| Downstream Operator | Labeled resource | Reconcile only its shard |
