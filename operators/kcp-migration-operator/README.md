# kcp Migration Operator

A Kubernetes operator for Platform Mesh that synchronizes custom resources from existing Kubernetes clusters to [kcp](https://docs.kcp.io/kcp/main/) (Kubernetes-like Control Planes) workspaces.

## What is kcp?

[kcp](https://docs.kcp.io/kcp/main/) is a Kubernetes-like control plane focused on managing APIs and resources across multiple clusters. Unlike standard Kubernetes clusters that run workloads, kcp provides:

- **Workspaces**: Isolated, hierarchical tenants for organizing resources
- **API Management**: Ability to define and serve custom APIs without running pods
- **Multi-cluster Coordination**: Sync resources to physical clusters via syncer

**Important**: kcp's API server behaves differently from standard Kubernetes. While it speaks the Kubernetes API, it primarily manages **custom resources** and API definitions rather than native workloads like Pods or Deployments.

## Problem Statement

Organizations with existing Kubernetes clusters often have **custom resources** (CRDs) that are not managed by kcp. When adopting Platform Mesh, these resources need to be synchronized to kcp workspaces while potentially transforming them to match Platform Mesh API conventions.

## Primary Use Case

The primary use case is synchronizing **custom resources** from source clusters to kcp:

```
Source Cluster (Standard K8s)          kcp Workspace
┌────────────────────────────┐        ┌────────────────────────────┐
│  Custom Resources (CRDs)   │   ──►  │  Platform Mesh Resources   │
│  - MyApp CRs               │        │  - ManagedApp CRs          │
│  - Config CRs              │        │  - ManagedConfig CRs       │
│  - Service Mesh CRs        │        │  - Workload CRs            │
└────────────────────────────┘        └────────────────────────────┘
```

While native Kubernetes resources (ConfigMaps, Secrets) can also be synced, the focus is on custom resources since kcp workspaces are designed for API-level resource management.

## Goals

- **Resource Discovery**: Watch specified custom resource kinds in source clusters
- **Synchronization**: Sync resources from source clusters to kcp workspaces
- **Transformation**: Transform resources during sync (different API groups, spec modifications)
- **Filtering**: Filter source resources by namespace and label selectors
- **Scalability**: Dynamically spawn operator instances based on workload definitions
- **Declarative Configuration**: Users define sync behavior through a custom resource

## Non-Goals

- Bidirectional synchronization (kcp to source cluster)
- Real-time conflict resolution between source and target
- Migration of cluster-scoped resources (initial version)
- Running workloads in kcp (kcp doesn't run Pods)

## Architecture Overview

The operator uses a **dynamic operator spawning** pattern:

1. **Main Operator**: Watches `KCPMigration` custom resources
2. **Child Operators**: Spawned dynamically per migration definition, each watching specific resource kinds
3. **Same Binary**: Both main and child operators use the same binary with different runtime modes
4. **Shared Secrets**: Child operators use the same kubeconfig secrets as the main operator

### Cluster Access

The operator requires two kubeconfig secrets:
- **`kcp-kubeconfig`**: Admin access to kcp for writing resources to workspaces
- **`source-kubeconfig`**: Read access to the source cluster for watching resources

Resources are written directly to kcp workspaces using an unstructured client (no virtual workspaces).

### Framework

Built on **platform-mesh/golang-commons** lifecycle framework for consistent operator patterns.

```
┌─────────────────────────────────────────────────────────────┐
│                Source Cluster (Standard K8s)                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │  MyApp CRs  │  │ Config CRs  │  │  Other CRs  │         │
│  │             │  │             │  │             │         │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘         │
└─────────┼────────────────┼────────────────┼─────────────────┘
          │                │                │
          ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────┐
│              kcp Migration Operator (Main)                  │
│  ┌────────────────────────────────────────────────────┐    │
│  │           Watches KCPMigration CRDs                │    │
│  └────────────────────────┬───────────────────────────┘    │
│                           │                                 │
│            ┌──────────────┼──────────────┐                 │
│            ▼              ▼              ▼                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │   Child     │  │   Child     │  │   Child     │        │
│  │  Operator   │  │  Operator   │  │  Operator   │        │
│  │  (MyApp)    │  │  (Config)   │  │  (Other)    │        │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
└─────────┼────────────────┼────────────────┼─────────────────┘
          │                │                │
          ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────┐
│                    kcp Workspace                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │ ManagedApp  │  │ManagedConfig│  │ Transformed │         │
│  │     CRs     │  │     CRs     │  │     CRs     │         │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
└─────────────────────────────────────────────────────────────┘
```

## Custom Resource Example

Sync a custom resource from source cluster to kcp with transformation:

```yaml
apiVersion: migration.platform-mesh.io/v1alpha1
kind: KCPMigration
metadata:
  name: myapp-migration
  namespace: migration-system
spec:
  # Source: watch MyApp resources matching the filters
  source:
    apiVersion: apps.example.com/v1
    kind: MyApp
    # Optional: filter by namespace
    namespace: production
    # Optional: filter by labels (AND logic - all must match)
    labelSelectors:
      - "env=production"
      - "tier in (frontend,backend)"

  # Transform: define target workspace and resource template
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
        spec:
          replicas: "{{ .Source.spec.replicas | default 1 }}"
          image: "{{ .Source.spec.image }}"
          config: "{{ .Source.spec.config | toJson }}"
```

**Result**: `MyApp` in namespace `team-a` → kcp workspace `root:platform-mesh:team-a`

**Note**: When a source resource is deleted, the target in kcp is also deleted. To stop syncing while preserving kcp resources, delete the `KCPMigration` CR itself.

## Documentation

- [Architecture Design](docs/architecture.md) - Detailed architecture and design decisions
- [API Specification](docs/api-specification.md) - CRD specification and field definitions

## Comparison with kcp API Sync Agent

The [kcp API Sync Agent](https://github.com/kcp-dev/api-syncagent) is a related project with a different purpose:

| Aspect | kcp Migration Operator | kcp API Sync Agent |
|--------|------------------------|-------------------|
| **Purpose** | Temporary migration of resources to kcp | Long-term continuous synchronization |
| **Direction** | Source cluster → kcp (one-way) | Bidirectional (kcp ↔ cluster) |
| **Use Case** | Migrate existing resources before decommissioning source cluster | Permanent runtime sync where kcp is source of truth |
| **Lifecycle** | Removed after migration complete | Runs indefinitely |
| **Source of Truth** | Source cluster (during migration) | kcp (always) |
| **Transformation** | Full resource transformation support | Primarily syncs as-is |

**When to use kcp Migration Operator:**
- You have existing Kubernetes clusters with custom resources
- You want to migrate these resources to kcp/Platform Mesh
- After migration, you will build operators that reconcile against kcp directly
- The source cluster's APIs will become obsolete

**When to use kcp API Sync Agent:**
- kcp is your permanent control plane
- You need ongoing bidirectional sync between kcp and execution clusters
- Operators on physical clusters process resources synced from kcp

## References

- [kcp Documentation](https://docs.kcp.io/kcp/main/) - Official kcp documentation
- [kcp GitHub](https://github.com/kcp-dev/kcp) - kcp source code

## Status

**Phase: Planning** - This repository contains design documentation only. No implementation code yet.

## Contributing

This project is in the design phase. Please review the architecture documentation and provide feedback through issues or pull requests.

## License

Apache License 2.0
