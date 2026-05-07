# Usage Guide

This document covers deployment, `ResourceSharding` CR configuration, downstream operator integration, and operational procedures.

## Prerequisites

- Kubernetes 1.29+
- `kubectl` with cluster-admin access
- Helm 3 (for chart-based deployment)
- The operator image published at `ghcr.io/platform-mesh/resource-sharding-operator`

## Deploying the Operator

### Helm

```bash
helm upgrade --install resource-sharding-operator \
  oci://ghcr.io/platform-mesh/helm-charts/resource-sharding-operator \
  --namespace resource-sharding-system \
  --create-namespace
```

See `chart/values.yaml` for the full list of configurable values. Key options:

| Value | Default | Description |
|-------|---------|-------------|
| `replicaCount` | `1` | Number of operator replicas |
| `leaderElection.enabled` | `true` | Enable leader election |
| `metrics.bindAddress` | `:9090` | Prometheus metrics endpoint |
| `health.bindAddress` | `:8090` | Health / readiness probe endpoint |
| `kcp.apiExportEndpointSliceName` | `sharding.platform-mesh.io` | KCP APIExport name (only used when KCP mode is enabled) |

### Building Locally

```bash
task build          # compile binary to bin/manager
task docker-build   # build container image
task docker:kind    # build and load into a local Kind cluster
```

## Configuring Sharding

Create a `ResourceSharding` CR for each resource type you want to shard. The CR is cluster-scoped.

### Minimal Example

```yaml
apiVersion: sharding.platform-mesh.io/v1alpha1
kind: ResourceSharding
metadata:
  name: accounts-sharding
spec:
  target:
    group: platform-mesh.io
    version: v1alpha1
    resource: accounts    # plural resource name
  shards:
    - name: shard-a
    - name: shard-b
    - name: shard-c
```

This creates three shards with all defaults:
- Label key: `sharding.platform-mesh.io/shard`
- Rebalance interval: 5 minutes, 20% threshold
- Webhook: disabled (controller-only mode)

### Full Example

```yaml
apiVersion: sharding.platform-mesh.io/v1alpha1
kind: ResourceSharding
metadata:
  name: accounts-sharding
spec:
  target:
    group: platform-mesh.io
    version: v1alpha1
    resource: accounts
  shardLabelKey: sharding.platform-mesh.io/shard   # optional, this is the default
  shards:
    - name: shard-a
    - name: shard-b
    - name: shard-c
  rebalance:
    interval: 5m        # how often the rebalancer runs
    threshold: 20       # % imbalance before rebalancing kicks in (1-100)
    movesPerCycle: 10   # % of imbalanced resources to move per run (1-100)
    minMovesPerCycle: 10 # floor: always move at least this many resources
    rateLimit: 10       # max PATCH operations per second during rebalance
  webhook:
    enabled: true       # enable fast-path assignment at admission time
```

### Spec Reference

#### `spec.target` (required)

| Field | Description |
|-------|-------------|
| `group` | API group of the target resource (e.g. `platform-mesh.io`) |
| `version` | API version (e.g. `v1alpha1`) |
| `resource` | Plural resource name (e.g. `accounts`) |

#### `spec.shards` (required, minimum 1)

List of shard names. Names are arbitrary strings — they become label values. Shard assignment is round-robin; adding or removing shards triggers rebalancing.

#### `spec.shardLabelKey` (optional)

Label key applied to target resources. Default: `sharding.platform-mesh.io/shard`. Override if the target resource already uses this key for another purpose.

#### `spec.rebalance`

| Field | Default | Description |
|-------|---------|-------------|
| `interval` | `5m` | Rebalance check interval |
| `threshold` | `20` | Imbalance percentage that triggers moves (1–100) |
| `movesPerCycle` | `10` | Percentage of imbalanced resources to move per cycle (1–100) |
| `minMovesPerCycle` | `10` | Minimum moves per cycle (floor, prevents stalling) |
| `rateLimit` | `10` | Max PATCH operations per second |

#### `spec.webhook`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable the mutating admission webhook for this target |

### Status Fields

```yaml
status:
  totalShards: 3
  distribution:
    - shard: shard-a
      count: 1024
    - shard: shard-b
      count: 1031
    - shard: shard-c
      count: 1019
  lastRebalanceTime: "2026-05-01T10:00:00Z"
  observedGeneration: 1
  conditions:
    - type: Ready
      status: "True"
      reason: ControllersRunning
      message: "Watching platform-mesh.io/v1alpha1, Resource=accounts, 3 shards configured"
```

### Checking Status

```bash
# View distribution
kubectl get resourcesharding accounts-sharding -o jsonpath='{.status.distribution}' | jq .

# Check conditions
kubectl get resourcesharding accounts-sharding -o jsonpath='{.status.conditions}' | jq .
```

## Granting RBAC for Target Resources

The operator uses an aggregated ClusterRole. Create a per-resource ClusterRole with the aggregation label:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: resource-sharding-operator-accounts
  labels:
    sharding.platform-mesh.io/aggregate-to-operator: "true"
rules:
  - apiGroups: ["platform-mesh.io"]
    resources: ["accounts"]
    verbs: ["get", "list", "watch", "patch"]
```

If the operator does not have the required permissions, the `PermissionsMissing` status condition is set and the dynamic controller is not started until the RBAC issue is resolved.

## Enabling the Webhook (Optional)

The webhook provides immediate shard assignment at resource creation time. Without it, the dynamic controller assigns labels within seconds — the webhook is an optimisation, not a requirement.

When `spec.webhook.enabled: true`, the operator creates a `MutatingWebhookConfiguration` named `resource-sharding-<cr-name>`. The webhook uses `failurePolicy: Ignore`, so if the webhook pod is unavailable, resource creation continues and the controller assigns the label as fallback.

The webhook service and TLS certificate must be available for the API server to call it. Ensure the operator's webhook service is running and the certificate is valid. In-cluster certificate management (e.g. cert-manager) is recommended.

## Integrating Downstream Operators

This is the key step that delivers memory and throughput benefits. After the sharding operator labels resources, configure each downstream operator replica to watch only its assigned shard.

### Step 1 — Set the shard label selector on the manager cache

```go
import (
    "os"
    "sigs.k8s.io/controller-runtime/pkg/cache"
    "k8s.io/apimachinery/pkg/labels"
)

mgr, err := ctrl.NewManager(cfg, ctrl.Options{
    Cache: cache.Options{
        DefaultLabelSelector: labels.SelectorFromSet(labels.Set{
            "sharding.platform-mesh.io/shard": os.Getenv("SHARD_NAME"),
        }),
    },
})
```

Each replica reads its assigned shard name from the `SHARD_NAME` environment variable.

### Step 2 — Deploy one replica per shard

```yaml
# shard-a deployment
env:
  - name: SHARD_NAME
    value: shard-a
```

Repeat for `shard-b`, `shard-c`, etc. Each deployment reconciles only its labelled subset.

### Step 3 (recommended) — Propagate the shard label to owned resources

For full memory isolation, copy the shard label to all resources created by the operator:

```go
func (r *AccountReconciler) createNamespace(account *v1alpha1.Account) *corev1.Namespace {
    return &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: account.Name,
            Labels: map[string]string{
                "sharding.platform-mesh.io/shard": account.Labels["sharding.platform-mesh.io/shard"],
            },
        },
    }
}
```

Without label propagation, the `DefaultLabelSelector` on the cache will cause the operator to not receive events for owned resources (Namespaces, Pods, etc.) unless they also carry the shard label. Use `ByObject` selectors if you only want to filter the primary resource and cache all secondary resources:

```go
cache.Options{
    ByObject: map[client.Object]cache.ByObject{
        &v1alpha1.Account{}: {
            Label: shardSelector,
        },
        // secondary resources — no selector, fully cached
    },
}
```

## Rollout Procedure

The sharding operator is additive. Existing operators continue working unchanged during rollout.

1. Deploy the sharding operator and grant it RBAC for the target resource.
2. Create the `ResourceSharding` CR. The operator begins assigning labels.
3. Wait for initial labelling to complete. Monitor via `status.distribution` — all resources should appear in the distribution counts.
4. Deploy the sharded downstream operator replicas (one per shard, each with `SHARD_NAME` set).
5. Scale down / remove the unsharded downstream operator deployment.

At no point are resources left unmanaged. The original operator continues running until step 4 is complete.

## Adding a Shard

1. Add the new shard name to `spec.shards`.
2. The rebalancer detects imbalance and moves resources to the new shard over subsequent cycles.
3. Deploy the new shard replica with its `SHARD_NAME` environment variable once it appears in `status.distribution`.

## Removing a Shard

1. Scale down the shard replica you are removing.
2. Remove the shard name from `spec.shards`.
3. The rebalancer strips the now-invalid label from the affected resources. They re-enter the dynamic controller watch and are reassigned to the remaining shards.

> [!NOTE]
> Resources that belonged to the removed shard are temporarily unmanaged between the label strip and reassignment. This window is bounded by the rebalance rate limit and the dynamic controller queue processing time (typically seconds to minutes depending on volume).

## Monitoring

Prometheus metrics are exposed on `metrics.bindAddress` (default `:9090`).

| Metric | Type | What to alert on |
|--------|------|-----------------|
| `resource_sharding_distribution{shard="..."}` | Gauge | Persistent zero — shard receiving no resources |
| `resource_sharding_imbalance_ratio` | Gauge | Sustained value > 0.3 (30% imbalance) |
| `resource_sharding_assignments_total` | Counter | Unexpectedly high rate (potential label churn) |
| `resource_sharding_rebalance_moves_total` | Counter | Continuous high rate (check threshold config) |

## Troubleshooting

### `TargetNotFound` condition

The resource type specified in `spec.target` does not exist in the cluster. Verify the GVR is correct with:

```bash
kubectl api-resources --api-group=<group>
```

### `PermissionsMissing` condition

The operator's service account lacks `get`, `list`, `watch`, or `patch` on the target resource. Create the aggregated ClusterRole (see [Granting RBAC](#granting-rbac-for-target-resources)).

### `Conflict` condition

Another `ResourceSharding` already targets the same GVR. Only one `ResourceSharding` per GVR is allowed. List existing configurations:

```bash
kubectl get resourcesharding -o custom-columns='NAME:.metadata.name,TARGET:.spec.target.resource'
```

### Resources not being labelled

1. Check the `Ready` condition on the `ResourceSharding` CR.
2. Check operator logs for `shard-assign` entries.
3. Verify the rebalance interval and threshold — resources may already be labelled and balanced.

### Webhook not intercepting requests

1. Confirm `spec.webhook.enabled: true` and the `MutatingWebhookConfiguration` exists:
   ```bash
   kubectl get mutatingwebhookconfigurations resource-sharding-<cr-name>
   ```
2. Check that the webhook service and TLS certificate are valid.
3. `failurePolicy: Ignore` means the webhook being unavailable is not blocking — check the dynamic controller fallback is working by looking at `status.distribution` after a rebalance.
