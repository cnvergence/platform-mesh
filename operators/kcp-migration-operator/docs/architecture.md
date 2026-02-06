# Architecture Design

## Overview

The KCP Migration Operator enables synchronization of Kubernetes custom resources from existing clusters to [KCP](https://docs.kcp.io/kcp/main/) workspaces. It uses a dynamic operator spawning pattern where the main operator creates child operators based on user-defined migration configurations.

## KCP Considerations

[KCP](https://docs.kcp.io/kcp/main/) (Kubernetes Control Plane) differs from standard Kubernetes in important ways:

- **No Workload Execution**: KCP doesn't run Pods - it's an API server for managing resources
- **Workspace Model**: Resources live in hierarchical workspaces (e.g., `root:org:team:project`)
- **API-First Design**: Focused on API management via APIExports and APIResourceSchemas
- **Multi-Cluster Sync**: Uses syncers to push resources to physical clusters

### KCP APIs vs Kubernetes CRDs

**Important**: KCP does not use traditional Kubernetes CRDs. Instead, KCP uses:
- **APIResourceSchemas**: Define the structure of resources (similar to CRD schemas)
- **APIExports**: Expose APIs from one workspace to others
- **APIBindings**: Consume APIs from other workspaces

This operator writes resources using an **unstructured client**, which means:
- No client-side schema validation is performed
- The KCP API server validates resources against its APIResourceSchemas
- If the target API doesn't exist in the workspace, the write will fail
- **Prerequisite**: Target APIs must be available in the KCP workspace (via APIBinding) before migration

### APIExport/APIBinding Prerequisites

Before the migration operator can sync resources to a KCP workspace, the target API must be available in that workspace. This requires:

1. **APIResourceSchema**: Defines the resource structure (equivalent to CRD spec)
2. **APIExport**: Exposes the API from a provider workspace (e.g., `root:platform-mesh-system`)
3. **APIBinding**: Binds the API in the target workspace (e.g., `root:orgs:sap`)

**Example Setup for Account Migration**:

```
root:platform-mesh-system/           # Provider workspace
├── APIResourceSchema: accounts      # Defines Account structure
└── APIExport: core-apis             # Exports Account API
    └── permissionClaims: [accounts]

root:orgs/                           # Consumer workspace (org level)
└── APIBinding: core-apis            # Binds to core-apis export
    └── Now has: Account API available

root:orgs:sap/                       # Child workspace (inherits bindings)
└── Account resources can be created here
```

**Migration Operator Behavior**:

When the target API is not available (missing APIBinding):
1. Write operation fails with API not found error
2. Error is classified as **retryable** (not terminal)
3. Resource is requeued with exponential backoff
4. Once APIBinding is created, retry succeeds

**Ordering for New Workspaces**:

When migrating to newly created workspaces:
1. Workspace is created (e.g., by Account operator)
2. APIBindings are created (by workspace type or manually)
3. Migration operator retries and succeeds

This is handled by the retry logic documented in [Workspace Dependency Handling](#workspace-dependency-handling).

### APIResourceSchema Management

The operator can optionally create APIResourceSchemas in KCP to ensure the target API is available. This is useful when:
- Migrating to a fresh KCP installation
- The source CRD schema should be replicated in KCP
- You want the migration to be self-contained

**Schema Sources**:

| Source | Use Case |
|--------|----------|
| `fromSourceCRD` | Automatically derive schema from source cluster's CRD |
| `inline` | Provide explicit schema definition in the KCPMigration |
| `configMapRef` | Reference external schema definition |

**What the Operator Creates**:

```yaml
apiVersion: apis.kcp.io/v1alpha1
kind: APIResourceSchema
metadata:
  name: v1alpha1.accounts.core.platform-mesh.io  # Generated name
spec:
  group: core.platform-mesh.io
  names:
    kind: Account
    plural: accounts
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema: { ... }  # From source CRD or inline
```

**What the Operator Does NOT Create**:

- **APIExports**: Must be configured separately (exports the API from provider workspace)
- **APIBindings**: Must be configured separately (binds API in consumer workspaces)

These are typically configured via workspace types or manual setup, as they represent organizational decisions about API exposure.

**Schema Lifecycle**:

1. Created when KCPMigration is created (if `apiResourceSchema.enabled`)
2. Updated when schema source changes
3. **Retained** when KCPMigration is deleted (to avoid breaking other consumers)

This operator targets KCP as the destination, meaning:
- Primary use case is **custom resources**, not native K8s objects
- Transformations often change API groups to Platform Mesh conventions
- Resources in KCP represent desired state, synced elsewhere for execution

## KCP Client Strategy

### Unstructured Client Approach

The operator uses an **ad-hoc unstructured client** to write resources directly to KCP workspaces:

- **No Virtual Workspaces**: The operator does NOT use KCP's virtual workspace feature
- **Direct Workspace Access**: Resources are written directly to the target workspace path
- **Dynamic Client Construction**: For each sync operation, the operator constructs an unstructured client configured for the specific workspace

**Why Unstructured Client?**
1. **Flexibility**: Can handle any resource kind without pre-compiled types
2. **Dynamic Workspaces**: Target workspace is determined at runtime from source resource
3. **Transformation Support**: Resources are transformed before writing, requiring dynamic typing

**Implementation Pattern**:
```go
// Pseudo-code for workspace client construction
func (s *SyncController) getWorkspaceClient(workspace string) (client.Client, error) {
    // Build REST config for specific workspace
    config := rest.CopyConfig(s.kcpConfig)
    config.Host = fmt.Sprintf("%s/clusters/%s", s.kcpConfig.Host, workspace)

    // Create unstructured client for this workspace
    return client.New(config, client.Options{
        Scheme: runtime.NewScheme(),
    })
}
```

## Design Principles

1. **Single Binary, Multiple Modes**: One binary serves as both the main controller and child operators
2. **Declarative Configuration**: All sync behavior defined through CRDs
3. **Isolation**: Each resource kind gets its own operator instance for fault isolation
4. **Extensibility**: Transformation rules allow flexible resource mapping
5. **Platform Mesh Alignment**: Use platform-mesh golang-commons for consistent operator patterns

## Framework: platform-mesh/golang-commons

This operator uses the **golang-commons lifecycle framework** from platform-mesh for controller implementation:

```go
import (
    "github.com/platform-mesh/golang-commons/pkg/lifecycle"
)
```

**Benefits**:
- Standardized reconciliation loop patterns
- Built-in subroutine orchestration
- Consistent error handling with `dxperrors.OperatorError`
- Status management and condition updates
- Metrics and observability integration

**Lifecycle Pattern**:
```go
// Main controller uses lifecycle manager
type KCPMigrationReconciler struct {
    lifecycle.LifecycleManager
}

// Subroutines for each reconciliation phase
func (r *KCPMigrationReconciler) GetSubroutines() []lifecycle.Subroutine {
    return []lifecycle.Subroutine{
        &ValidateSpecSubroutine{},
        &CreateChildOperatorSubroutine{},
        &UpdateStatusSubroutine{},
    }
}
```

## Components

### 1. Main Controller

**Responsibility**: Watch `KCPMigration` CRDs and manage child operator lifecycle

**Behavior**:
- Reconciles `KCPMigration` resources
- For each migration definition, spawns a child operator (Deployment)
- Manages child operator lifecycle (create, update, delete)
- Reports aggregated status back to the `KCPMigration` resource

```
┌────────────────────────────────────────────────────────────┐
│                    Main Controller                         │
├────────────────────────────────────────────────────────────┤
│  Watches: KCPMigration CRDs                                │
│  Creates: Child Operator Deployments                       │
│  Updates: KCPMigration status with sync statistics         │
└────────────────────────────────────────────────────────────┘
```

### 2. Child Operator (Sync Controller)

**Responsibility**: Watch source resources and sync to KCP

**Sync Mechanism** (watch-based, no polling):
1. **Initial Sync**: List all matching resources and sync to KCP workspace
2. **Watch**: Open Kubernetes watch on the source resource kind
3. **React**: On create/update/delete events, immediately sync changes to KCP

**Behavior**:
- Receives configuration via environment variables or ConfigMap
- Uses Kubernetes informer to watch specified resource kind
- Applies transformations if configured
- Creates/updates/deletes resources in target KCP workspace
- Reports status back to main controller via events/conditions

```
┌────────────────────────────────────────────────────────────┐
│                   Child Operator                           │
├────────────────────────────────────────────────────────────┤
│  Input: Source cluster kubeconfig, target KCP kubeconfig   │
│  Watches: Specific resource kind (via Kubernetes watch)    │
│  Transforms: Optional Go template or CEL transformations   │
│  Syncs: Resources to KCP workspace in real-time            │
└────────────────────────────────────────────────────────────┘
```

### 3. Transformation Engine

**Responsibility**: Transform resources during sync

**Capabilities**:
- API group/version changes
- Kind remapping
- Spec field transformations using CEL expressions
- Metadata modifications (labels, annotations)
- Field filtering (include/exclude specific fields)

## Runtime Modes

The operator binary supports two runtime modes:

### Mode: Controller (Default)

```bash
kcp-migration-operator --mode=controller
```

- Runs the main controller
- Watches `KCPMigration` CRDs
- Manages child operator Deployments

### Mode: Sync

```bash
kcp-migration-operator --mode=sync \
  --source-kubeconfig=/path/to/source \
  --target-kubeconfig=/path/to/kcp \
  --config=/path/to/sync-config.yaml
```

- Runs as a sync operator for a specific resource kind
- Configuration provided via file or environment variables
- Managed by the main controller

## Data Flow

```
┌──────────────────────────────────────────────────────────────────────┐
│                           User Action                                │
│                    Creates KCPMigration CR                           │
└─────────────────────────────────┬────────────────────────────────────┘
                                  │
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        Main Controller                               │
│  1. Validates KCPMigration spec                                      │
│  2. Creates ServiceAccount for child operator                        │
│  3. Creates ConfigMap with sync configuration                        │
│  4. Creates Deployment for child operator                            │
│  5. Updates KCPMigration status                                      │
└─────────────────────────────────┬────────────────────────────────────┘
                                  │
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        Child Operator                                │
│  1. Starts with sync mode                                            │
│  2. Establishes watch on source cluster                              │
│  3. For each source resource:                                        │
│     a. Apply transformation rules                                    │
│     b. Create/Update in KCP workspace                                │
│  4. Reports sync status via metrics/events                           │
└─────────────────────────────────┬────────────────────────────────────┘
                                  │
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         KCP Workspace                                │
│                    Resources synchronized                            │
└──────────────────────────────────────────────────────────────────────┘
```

## CRD Design

### KCPMigration

```yaml
apiVersion: migration.platform-mesh.io/v1alpha1
kind: KCPMigration
metadata:
  name: string
  namespace: string
spec:
  # Source configuration - what to watch
  source:
    apiVersion: string        # Required: e.g., "apps.example.com/v1"
    kind: string              # Required: e.g., "MyApp", "TenantConfig"

  # Transform configuration - how to transform and where to put it
  transform:
    # Derive target workspace from source resource
    targetWorkspace:
      expression: string      # Go template: "root:platform-mesh:{{ .Source.metadata.namespace }}"

    # Template for target resource (inline or ConfigMap reference)
    template:
      inline: object          # Inline Go template for target resource
      configMapRef:           # OR reference to ConfigMap
        name: string
        key: string           # Defaults to "template.yaml"

status:
  # Overall sync status
  phase: string               # "Pending" | "Running" | "Failed" | "Stopped"

  # Child operator reference
  childOperator:
    name: string
    namespace: string
    ready: boolean

  # Sync statistics
  statistics:
    lastSyncTime: timestamp
    resourcesSynced: integer
    resourcesFailed: integer

  # Conditions
  conditions:
    - type: string
      status: string
      reason: string
      message: string
      lastTransitionTime: timestamp
```

### Template Variables

Templates have access to:
- `.Source` - The full source resource (unstructured)
- `.Source.metadata` - Source metadata (name, namespace, labels, annotations)
- `.Source.spec` - Source spec (for custom resources)
- `.Timestamp` - Current timestamp (RFC3339 format)
- `.Migration` - The KCPMigration resource

## Child Operator Deployment

The main controller creates a Deployment for each `KCPMigration` resource. Each child operator:

- Runs the same binary in `--mode=sync`
- Has its own ServiceAccount with RBAC for the specific resource kind
- Receives sync configuration via ConfigMap
- Connects to KCP using workspace credentials

Implementation details such as resource limits, replica count, and exact deployment structure are internal to the operator and not part of the user-facing API.

## Security Considerations

### RBAC

**Main Controller** requires:
- Watch/List/Get on `KCPMigration` CRDs
- Create/Update/Delete on Deployments, ConfigMaps, ServiceAccounts, RoleBindings
- Update status on `KCPMigration` CRDs
- Watch/List/Get on source resource kinds (required to grant permissions to child operators)

**Note**: The main controller must have access to any resource kind it needs to grant to child operators. Kubernetes RBAC only allows creating RoleBindings for permissions you already have.

**Child Operators** require:
- Watch/List/Get on source resource kinds (granted by main controller)
- Create/Update/Delete on target resource kinds in KCP

### Secret Management

The operator requires two kubeconfig secrets for operation:

**Required Secrets**:
1. **`kcp-kubeconfig`**: Admin access credentials for KCP
   - Provides admin access to KCP workspaces
   - Used to write resources directly to target workspaces
   - Mounted at `/etc/kcp/kubeconfig`

2. **`source-kubeconfig`** (or `KUBECONFIG`): Access credentials for the source cluster
   - Read access to watch and list source resources
   - Mounted at `/etc/source/kubeconfig`

**Secret Sharing**:
- Both the main controller and child operators use the **same secrets**
- Child operators inherit secret mounts from the main controller's configuration
- No per-child-operator secret management required

```yaml
# Example secret configuration
apiVersion: v1
kind: Secret
metadata:
  name: kcp-kubeconfig
  namespace: migration-system
type: Opaque
data:
  kubeconfig: <base64-encoded-kcp-kubeconfig>
---
apiVersion: v1
kind: Secret
metadata:
  name: source-kubeconfig
  namespace: migration-system
type: Opaque
data:
  kubeconfig: <base64-encoded-source-kubeconfig>
```

## Scalability

### Resource Isolation

Each `KCPMigration` gets its own:
- Deployment (child operator)
- ServiceAccount
- ConfigMap
- Resource limits

This ensures:
- Failure in one sync does not affect others
- Resource consumption is isolated and limited
- Easy debugging and monitoring per migration

### Performance Considerations

- Use Kubernetes informers for efficient resource watching
- **Watch-based sync**: No polling overhead, react to changes in real-time
- Initial sync uses list operation, then switches to watch for incremental updates
- Implement rate limiting on sync operations to avoid overwhelming KCP
- Support batch operations for bulk initial sync

### Performance Tuning

The operator supports several performance tuning options:

#### MaxWorkers

Controls the number of concurrent worker goroutines processing the reconciliation queue.

| Scenario | Recommended Workers | Notes |
|----------|---------------------|-------|
| Small migration (<100 resources) | 1 (default) | Minimal overhead |
| Medium migration (100-1000 resources) | 3-5 | Good balance of speed and stability |
| Large migration (>1000 resources) | 5-10 | Higher throughput, monitor resource usage |
| Very large migration (>10000 resources) | 10+ | Requires good network, monitor KCP load |

#### Rate Limiting

Rate limiting prevents overwhelming the KCP API server during bulk syncs:

- `resourcesPerSecond`: Maximum sync operations per second (default: 50)
- `burst`: Burst size for handling spikes (default: 100)

**Example configuration:**
```yaml
spec:
  syncOptions:
    maxWorkers: 5
    rateLimit:
      resourcesPerSecond: 100
      burst: 200
```

### Multi-Resource Sync Mode

For local development and testing, the sync operator supports a multi-resource mode via YAML configuration file. This allows syncing multiple resource types with a single operator instance.

#### Configuration Structure

The sync configuration uses reusable struct types for consistent configuration across CLI flags and YAML files:

| Section | Fields | Description |
|---------|--------|-------------|
| `source` | `apiVersion`, `kind`, `namespace` | Source resource to watch |
| `target` | `workspaceExpression`, `namespace` | Target KCP workspace and namespace |
| `transform` | `template`, `templatePath`, `configMapName`, `configMapKey` | Transformation template |
| `performance` | `maxWorkers`, `rateLimitResourcesPerSecond`, `rateLimitBurst` | Performance tuning |

**Configuration file format:**
```yaml
kcpKubeconfigPath: /path/to/kcp/kubeconfig
sourceKubeconfigPath: /path/to/source/kubeconfig  # optional
templatesDir: .templates

resources:
  - name: accounts-to-sap-org
    source:
      apiVersion: fabric.foundation.sap.com/v1alpha1
      kind: Account
      namespace: account-2bzns
    target:
      workspaceExpression: "root:orgs:sap"
    transform:
      templatePath: account-to-project.yaml
    performance:
      maxWorkers: 5

  - name: spaces-to-projects
    source:
      apiVersion: fabric.foundation.sap.com/v1alpha1
      kind: Space
    target:
      workspaceExpression: "root:orgs:sap:{{ .Source.metadata.namespace }}"
    transform:
      template: |
        apiVersion: core.platform-mesh.io/v1alpha1
        kind: Project
        metadata:
          name: "{{ index .Source.metadata \"name\" }}"
    performance:
      maxWorkers: 3
```

**Usage:**
```bash
kcp-migration-operator sync --config=config/sync-config.yaml
```

**Template files:**
- Store templates in a `.templates/` directory (gitignored for local development)
- Templates use Go template syntax with Sprig functions
- Custom functions: `getField`, `getFieldStr` for nested field access

### Workspace Dependency Handling

Some resources create workspaces as a side effect. For example, when an `Account` resource with `type: org` is synced to KCP, it triggers workspace creation (e.g., `root:orgs:sap`). Other resources that target this workspace (like projects or teams) may be synced before the workspace exists.

**Retry Strategy for Missing Workspaces**:

The operator handles missing workspaces as a **transient condition**, not a fatal error:

1. **Detection**: When writing to KCP fails with "workspace not found" or similar errors
2. **Classification**: Error is classified as **retryable** (not terminal)
3. **Backoff**: Resource is requeued with exponential backoff
4. **Status**: Condition is set to `WorkspacePending` with details
5. **Resolution**: Once the workspace exists, the next retry succeeds

**Error Classification**:

| Error Type | Classification | Action |
|------------|----------------|--------|
| Workspace not found | Retryable | Requeue with backoff |
| API not available (missing APIBinding) | Retryable | Requeue with backoff |
| Network timeout | Retryable | Requeue with backoff |
| Invalid transformation | Terminal | Skip resource, emit event |
| Schema validation failure | Terminal | Skip resource, emit event |

**Ordering Considerations**:

For migrations with workspace dependencies:
- Create migrations for workspace-creating resources first (e.g., org Accounts)
- Allow time for workspaces to be created before starting dependent migrations
- Or rely on retry logic to handle eventual consistency

## Observability

### Metrics (Prometheus)

```
# Main controller metrics
kcp_migration_controller_reconcile_total{name, status}
kcp_migration_controller_child_operators_active

# Child operator metrics
kcp_migration_sync_resources_total{kind, status}
kcp_migration_sync_duration_seconds{kind}
kcp_migration_sync_errors_total{kind, error_type}
kcp_migration_sync_last_success_timestamp{kind}
```

### Events

- ResourceSynced: Successfully synced a resource
- ResourceSyncFailed: Failed to sync a resource
- TransformationError: Transformation rule failed
- WorkspacePending: Target workspace does not exist yet (will retry)
- ChildOperatorCreated: Child operator deployment created
- ChildOperatorFailed: Child operator encountered an error
