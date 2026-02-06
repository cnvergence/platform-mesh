# API Specification

## Custom Resource Definitions

### KCPMigration

The `KCPMigration` CRD defines a synchronization job from a source cluster to a KCP workspace.

---

## Full Specification (Minimal API)

```yaml
apiVersion: migration.platform-mesh.io/v1alpha1
kind: KCPMigration
metadata:
  name: string                    # Unique name for this migration
  namespace: string               # Namespace where migration runs
spec:
  source: SourceSpec              # Required: What to watch
  transform: TransformSpec        # Required: How to transform and where to put it
  syncOptions: SyncOptions        # Optional: Rate limiting and sync behavior
  apiResourceSchema: APIResourceSchemaSpec  # Optional: Create APIResourceSchema in KCP
status:
  phase: string                   # Current phase
  childOperator: ChildOperatorStatus
  statistics: SyncStatistics
  conditions: []Condition
```

---

## Spec Fields

### SourceSpec

Defines the source resources to watch. The operator watches all instances of the specified resource kind across all namespaces (or a specific namespace if configured).

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `apiVersion` | string | Yes | API version of the source resource (e.g., `apps.example.com/v1`) |
| `kind` | string | Yes | Kind of the source resource (e.g., `MyApp`, `TenantConfig`) |
| `namespace` | string | No | Namespace to filter source resources. If empty, watches all namespaces |
| `labelSelectors` | []string | No | List of label selectors to filter source resources. Resources must match ALL selectors (AND logic). Supports standard Kubernetes label selector syntax |

#### Label Selector Syntax

Label selectors support the standard Kubernetes selector syntax:

| Operator | Example | Description |
|----------|---------|-------------|
| `=`, `==` | `app=myapp` | Equality |
| `!=` | `env!=dev` | Inequality |
| `in` | `env in (prod,staging)` | Value in set |
| `notin` | `env notin (dev,test)` | Value not in set |
| (key only) | `app` | Key exists |
| `!` | `!temporary` | Key does not exist |

Multiple selectors are combined with AND logic - a resource must match all selectors to be synced.

#### Example: Basic Source

```yaml
spec:
  source:
    apiVersion: apps.example.com/v1
    kind: MyApp
```

#### Example: With Namespace and Label Selectors

```yaml
spec:
  source:
    apiVersion: apps.example.com/v1
    kind: MyApp
    namespace: production
    labelSelectors:
      - "app=myapp"
      - "env in (prod,staging)"
      - "!temporary"
```

This watches `MyApp` resources in the `production` namespace that have:
- Label `app` with value `myapp`
- Label `env` with value `prod` OR `staging`
- No label named `temporary`

---

### TransformSpec

Defines how source resources are transformed and where they are placed in KCP. This is the core of the migration configuration.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `targetWorkspace` | WorkspaceExpression | Yes | How to determine the target KCP workspace |
| `targetNamespace` | string | No | Namespace in target workspace (Go template supported). If not specified, uses source namespace or cluster-scoped |
| `template` | TemplateSpec | No | Template for the target resource. If not specified, resource is synced as-is |

**Note**: If `template` is not specified, the source resource is copied to KCP without transformation (pass-through mode). The `targetNamespace` field can still be used to control placement.

#### WorkspaceExpression

Defines how to derive the target KCP workspace from the source resource.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `expression` | string | Yes | Go template expression to derive workspace path |

The expression has access to:
- `.Source` - The full source resource
- `.Source.metadata.namespace` - Source namespace (commonly used to derive workspace)
- `.Source.metadata.name` - Source resource name
- `.Source.metadata.labels` - Source labels

#### TemplateSpec

Defines the target resource structure.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `inline` | object | Conditional | Inline template for target resource |
| `configMapRef` | ConfigMapReference | Conditional | Reference to ConfigMap containing template |

One of `inline` or `configMapRef` must be specified.

#### ConfigMapReference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the ConfigMap |
| `key` | string | No | Key in ConfigMap. Defaults to `template.yaml` |

#### Template Variables

Templates use Go template syntax with access to:
- `.Source` - The full source resource (unstructured)
- `.Source.metadata` - Source metadata (name, namespace, labels, annotations)
- `.Source.spec` - Source spec (for custom resources)
- `.Timestamp` - Current timestamp (RFC3339 format)
- `.Migration` - The KCPMigration resource

#### Template Functions

| Function | Description | Example |
|----------|-------------|---------|
| `toJson` | Convert to JSON string | `{{ .Source.spec \| toJson }}` |
| `toYaml` | Convert to YAML string | `{{ .Source.spec \| toYaml }}` |
| `default` | Default value if empty | `{{ .Source.spec.replicas \| default 1 }}` |
| `required` | Fail if value missing | `{{ required "name required" .Source.metadata.name }}` |
| `lower` | Lowercase string | `{{ .Source.metadata.name \| lower }}` |
| `upper` | Uppercase string | `{{ .Source.metadata.name \| upper }}` |
| `replace` | String replacement | `{{ .Source.metadata.name \| replace "-" "_" }}` |

---

### SyncOptions

Optional configuration for sync behavior.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `rateLimit` | RateLimitConfig | No | See below | Rate limiting for sync operations |
| `maxWorkers` | integer | No | 1 | Maximum concurrent reconciliation workers |

#### RateLimitConfig

Controls how fast resources are synced, especially during initial bulk sync.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `resourcesPerSecond` | integer | No | 50 | Maximum resources to sync per second |
| `burst` | integer | No | 100 | Maximum burst size for rate limiter |

#### MaxWorkers

Controls the number of concurrent worker goroutines that process the reconciliation queue. Higher values increase throughput for large migrations but also increase resource consumption.

**Recommendations:**
- Default value of 1 is suitable for most migrations
- Increase to 3-5 for migrations with thousands of resources
- Increase to 10+ only for very large migrations with good network connectivity

#### Example

```yaml
spec:
  syncOptions:
    maxWorkers: 5
    rateLimit:
      resourcesPerSecond: 100
      burst: 200
```

---

### APIResourceSchemaSpec

Optional configuration for creating APIResourceSchemas in KCP. This ensures the target API is available before syncing resources.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `enabled` | boolean | No | If true, operator creates/updates APIResourceSchema. Default: false |
| `targetWorkspace` | string | Yes (if enabled) | Workspace where APIResourceSchema should be created (e.g., `root:platform-mesh-system`) |
| `schema` | SchemaSource | Yes (if enabled) | Source for the APIResourceSchema definition |

#### SchemaSource

Defines where to get the APIResourceSchema definition.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fromSourceCRD` | boolean | No | If true, derive schema from source cluster's CRD. Default: false |
| `inline` | object | No | Inline APIResourceSchema spec |
| `configMapRef` | ConfigMapReference | No | Reference to ConfigMap containing schema |

One of `fromSourceCRD`, `inline`, or `configMapRef` must be specified when `enabled` is true.

#### Example: Derive Schema from Source CRD

The operator reads the CRD from the source cluster and creates a corresponding APIResourceSchema in KCP:

```yaml
spec:
  source:
    apiVersion: fabric.foundation.sap.com/v1alpha1
    kind: Account

  apiResourceSchema:
    enabled: true
    targetWorkspace: "root:platform-mesh-system"
    schema:
      fromSourceCRD: true  # Derives schema from source cluster's Account CRD
```

#### Example: Inline Schema Definition

```yaml
spec:
  source:
    apiVersion: apps.example.com/v1
    kind: MyApp

  apiResourceSchema:
    enabled: true
    targetWorkspace: "root:platform-mesh-system"
    schema:
      inline:
        names:
          plural: myapps
          singular: myapp
          kind: MyApp
        scope: Namespaced
        versions:
          - name: v1
            served: true
            storage: true
            schema:
              openAPIV3Schema:
                type: object
                properties:
                  spec:
                    type: object
                    properties:
                      replicas:
                        type: integer
```

#### Schema Lifecycle

When `apiResourceSchema.enabled` is true:

1. **On KCPMigration creation**: Operator creates APIResourceSchema in target workspace
2. **On KCPMigration update**: Operator updates APIResourceSchema if schema source changed
3. **On KCPMigration deletion**: APIResourceSchema is **retained** (not deleted) to avoid breaking other consumers

**Note**: APIExports and APIBindings are NOT managed by this operator. They should be configured separately as part of your KCP workspace setup (e.g., via workspace types or manual configuration).

---

## Error Handling

### Template Errors

When a Go template fails to execute for a specific resource (e.g., missing field), the operator:

1. **Skips the resource** - does not write to KCP
2. **Logs the error** - with resource name and error details
3. **Emits an event** - `TransformationError` event on the KCPMigration
4. **Continues processing** - other resources are not affected
5. **Updates statistics** - increments `resourcesFailed` counter

This ensures that one malformed resource does not block the entire migration.

### Sync Errors

When writing to KCP fails:

1. **Retries with backoff** - transient errors are retried
2. **Logs the error** - with workspace and resource details
3. **Emits an event** - `ResourceSyncFailed` event
4. **Updates statistics** - tracks failed resources

---

## Deletion Behavior

When a source resource is deleted, the corresponding target resource in KCP is also deleted. This ensures the KCP workspace accurately reflects the source cluster state.

### Manual Deletion in KCP

If a synced resource is manually deleted in KCP (not via source deletion):

1. The operator detects the missing resource on next reconciliation
2. The resource is **re-synced** from source
3. KCP state is restored to match source

This ensures the **source cluster remains the source of truth** during migration.

### Decommissioning Workflow

To stop syncing without deleting resources in KCP, delete the `KCPMigration` CR itself:

1. Delete the `KCPMigration` resource
2. The child operator stops and is removed
3. All synced resources in KCP remain untouched
4. Source resources can then be cleaned up independently

This is the recommended approach for migration scenarios where the source cluster will be decommissioned.

---

## Status Fields

### Phase

| Value | Description |
|-------|-------------|
| `Pending` | Migration created, waiting for child operator |
| `Running` | Child operator is running and syncing |
| `Failed` | Child operator failed to start or is in error state |
| `Stopped` | Migration manually stopped |

### ChildOperatorStatus

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Name of the child operator Deployment |
| `namespace` | string | Namespace of the child operator |
| `ready` | boolean | Whether the child operator is ready |

### SyncStatistics

| Field | Type | Description |
|-------|------|-------------|
| `lastSyncTime` | timestamp | Last successful sync time |
| `resourcesSynced` | integer | Total resources successfully synced |
| `resourcesFailed` | integer | Resources that failed to sync |

### Conditions

Standard Kubernetes conditions:

| Type | Description |
|------|-------------|
| `Ready` | Overall readiness of the migration |
| `ChildOperatorReady` | Child operator deployment is ready |
| `Syncing` | Active sync in progress |
| `APIResourceSchemaReady` | APIResourceSchema created successfully (if enabled) |

---

## Complete Examples

### Example 1: Namespace-to-Workspace Migration

Each source namespace becomes a KCP workspace. Resources are transformed to Platform Mesh types.

```yaml
apiVersion: migration.platform-mesh.io/v1alpha1
kind: KCPMigration
metadata:
  name: myapp-migration
  namespace: migration-system
spec:
  source:
    apiVersion: apps.example.com/v1
    kind: MyApp

  transform:
    # Derive workspace from source namespace
    targetWorkspace:
      expression: "root:platform-mesh:{{ .Source.metadata.namespace }}"

    # Transform to Platform Mesh resource
    template:
      inline:
        apiVersion: apps.platform-mesh.io/v1alpha1
        kind: ManagedApp
        metadata:
          name: "{{ .Source.metadata.name }}"
          labels:
            platform-mesh.io/migrated: "true"
            platform-mesh.io/source-namespace: "{{ .Source.metadata.namespace }}"
        spec:
          replicas: "{{ .Source.spec.replicas | default 1 }}"
          image: "{{ .Source.spec.image }}"
          config: "{{ .Source.spec.config | toJson }}"
```

**Result**:
- Source `MyApp` in namespace `team-a` → KCP workspace `root:platform-mesh:team-a`
- Source `MyApp` in namespace `team-b` → KCP workspace `root:platform-mesh:team-b`

### Example 2: Label-based Workspace Routing

Route resources to workspaces based on a label value.

```yaml
apiVersion: migration.platform-mesh.io/v1alpha1
kind: KCPMigration
metadata:
  name: config-migration
  namespace: migration-system
spec:
  source:
    apiVersion: config.example.com/v1
    kind: TenantConfig

  transform:
    # Derive workspace from organization label
    targetWorkspace:
      expression: "root:{{ .Source.metadata.labels.organization }}:configs"

    template:
      inline:
        apiVersion: config.platform-mesh.io/v1alpha1
        kind: ManagedConfig
        metadata:
          name: "{{ .Source.metadata.name }}"
        spec:
          data: "{{ .Source.spec.data | toJson }}"
```

**Result**:
- Source with label `organization: acme` → KCP workspace `root:acme:configs`
- Source with label `organization: globex` → KCP workspace `root:globex:configs`

### Example 3: Filtering with Label Selectors

Only sync resources that match specific label criteria.

```yaml
apiVersion: migration.platform-mesh.io/v1alpha1
kind: KCPMigration
metadata:
  name: prod-services-migration
  namespace: migration-system
spec:
  source:
    apiVersion: services.example.com/v1
    kind: MicroService
    labelSelectors:
      - "env=production"
      - "tier in (frontend,backend)"
      - "!deprecated"

  transform:
    targetWorkspace:
      expression: "root:platform-mesh:production"

    template:
      inline:
        apiVersion: workload.platform-mesh.io/v1alpha1
        kind: ManagedWorkload
        metadata:
          name: "{{ .Source.metadata.name }}"
        spec:
          image: "{{ .Source.spec.image }}"
```

**Result**:
- Only `MicroService` resources with `env=production`, `tier` being `frontend` or `backend`, and without a `deprecated` label are synced
- Resources not matching all selectors are ignored

### Example 4: Static Workspace with ConfigMap Template

All resources go to a fixed workspace, using an external template.

```yaml
apiVersion: migration.platform-mesh.io/v1alpha1
kind: KCPMigration
metadata:
  name: services-migration
  namespace: migration-system
spec:
  source:
    apiVersion: services.example.com/v1
    kind: MicroService

  transform:
    # Fixed workspace for all resources
    targetWorkspace:
      expression: "root:platform-mesh:production:services"

    template:
      configMapRef:
        name: microservice-template
        key: template.yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: microservice-template
  namespace: migration-system
data:
  template.yaml: |
    apiVersion: workload.platform-mesh.io/v1alpha1
    kind: ManagedWorkload
    metadata:
      name: "{{ .Source.metadata.name }}"
      labels:
        app.kubernetes.io/name: "{{ .Source.metadata.name }}"
        platform-mesh.io/source-namespace: "{{ .Source.metadata.namespace }}"
    spec:
      runtime: kubernetes
      replicas: "{{ .Source.spec.replicas | default 1 }}"
      image: "{{ .Source.spec.image }}"
      ports: "{{ .Source.spec.ports | toJson }}"
```

---

## Validation Rules

### Required Fields
- `spec.source.apiVersion`
- `spec.source.kind`
- `spec.transform.targetWorkspace.expression`
- `spec.transform.template` (either `inline` or `configMapRef`)

### Immutable Fields
Once created, the following fields cannot be changed:
- `spec.source.apiVersion`
- `spec.source.kind`

To change these, delete and recreate the migration.
