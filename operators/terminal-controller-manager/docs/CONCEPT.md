# Terminal Controller Manager - Concept Document

## Context

Platform-mesh needs a browser-based terminal to connect to KCP (Kubernetes Control Plane) for running kubectl commands. Users should be able to open a terminal in the UI and interact with their KCP workspaces using kubectl and KCP plugins.

This implementation is inspired by [Gardener's terminal-controller-manager](https://github.com/gardener/terminal-controller-manager) but tailored specifically for KCP and platform-mesh architecture.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  Browser (portal / generic-resource-ui)                             │
│  ├─ xterm.js terminal emulator                                      │
│  ├─ Holds OIDC token (from Keycloak)                                │
│  └─ Connects to ttyd via Gateway API HTTPRoute                      │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ WebSocket (wss://portal.example.com/terminals/{session-id})
                           │ Token sent as first stdin message
                           ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Runtime Cluster                                                    │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  Gateway (k8sapi-gateway)                                     │  │
│  │  └─ HTTPRoute per terminal → routes to terminal Service       │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                              │                                       │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  Terminal Controller Manager                                   │  │
│  │  ├─ Watches Terminal CRDs (via KCP APIExport)                  │  │
│  │  ├─ Creates terminal pods, services, HTTPRoutes                │  │
│  │  └─ Manages terminal lifetime (auto-cleanup after 2h)          │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                              │                                       │
│                              ▼                                       │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  Terminal Pod (ephemeral)                                      │  │
│  │  ├─ ttyd WebSocket server on port 8080                         │  │
│  │  ├─ kubectl + KCP plugins + k9s pre-installed                  │  │
│  │  ├─ Reads token from stdin, validates user, creates kubeconfig │  │
│  │  └─ Drops to interactive shell                                 │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                           │
          ┌────────────────┴────────────────┐
          │ Watch Terminal CRs              │ kubectl commands
          ▼                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│  KCP API Server                                                     │
│  └─ User's workspace                                                │
│     ├─ Terminal CRs created here (user-facing API)                  │
│     └─ AccountInfo provides workspace URL and CA cert               │
└─────────────────────────────────────────────────────────────────────┘
```

### Multi-Cluster Design

The operator follows a **split-cluster architecture**:

| Cluster | Resources | Purpose |
|---------|-----------|---------|
| KCP (virtual workspaces) | Terminal CRs, AccountInfo | User-facing API, watched via APIExport |
| Runtime cluster | Terminal pods, services, HTTPRoutes, controller | Actual pod execution, Gateway routing |

This means:
- Users create `Terminal` resources in their KCP workspace
- Controller watches across all KCP workspaces via multicluster-runtime + apiexport provider
- Pods, Services, and HTTPRoutes are created on the runtime cluster where the controller runs
- The controller needs two clients: one for KCP (watching CRs) and one for runtime (managing resources)

## Key Design Decisions

### 1. Minimal CRD Spec (Controller-Driven Configuration)

**Problem:** Complex CRD specs require users to know implementation details and can lead to misconfiguration.

**Solution:** The Terminal CRD has an empty spec. All configuration (image, namespace, timeouts) is driven by controller arguments for consistent platform-wide behavior. The target workspace is automatically derived from where the Terminal CR is created.

**Benefits:**
- Users just create an empty Terminal CR to get a terminal
- Platform administrators control all terminal settings centrally
- No per-terminal configuration drift

### 2. ttyd + Gateway API (No Custom WebSocket Proxy)

**Problem:** Building a custom WebSocket proxy is complex and requires handling many edge cases.

**Solution:** Use [ttyd](https://github.com/tsl0922/ttyd) in each terminal pod to provide WebSocket-based terminal access, exposed via Gateway API HTTPRoutes.

**Benefits:**
- Battle-tested WebSocket terminal server
- Standard Gateway API for routing (works with any implementation)
- No custom WebSocket code in the controller
- Path-based routing with session IDs for security

### 3. Frontend Token Injection

**Problem:** Storing OIDC tokens in Kubernetes secrets creates security risks and complicates token lifecycle management.

**Solution:** The frontend passes the OIDC token as the first stdin message after WebSocket connection. The token is never stored in Kubernetes resources.

**Benefits:**
- Token never persisted in etcd/secrets
- Token lifecycle managed by frontend (refresh handled there)
- Simpler Terminal CRD (no credentials spec)
- Better security posture

### 4. Session ID for URL Security

**Problem:** Terminal URLs must not be guessable to prevent unauthorized access.

**Solution:** Each terminal gets a UUID-based session ID stored in status, used in the HTTPRoute path: `/terminals/{session-id}`.

**Benefits:**
- URLs are non-guessable
- Combined with user validation in setup script for defense in depth
- Session ID generated once and persists for terminal lifetime

## How It Works

1. **User requests terminal** - Frontend creates a `Terminal` custom resource in their KCP workspace
2. **Controller reconciles** - Reads AccountInfo to get workspace URL and CA cert, creates pod, service, and HTTPRoute
3. **Session ID generated** - UUID stored in status.sessionId for non-guessable URL path
4. **Resources created** - Pod with ttyd, ClusterIP Service, HTTPRoute pointing to the gateway
5. **Pod becomes ready** - Status updated with phase=Ready, podName, workspacePath
6. **Frontend connects** - WebSocket to `wss://portal.example.com/terminals/{session-id}`
7. **Token handshake** - Frontend sends OIDC token as first message (typed into ttyd stdin)
8. **User validated** - setup.sh validates token's `sub` claim matches `createdBy` in status
9. **Kubeconfig created** - Token and CA cert written to tmpfs, kubeconfig configured
10. **Shell ready** - User gets interactive shell with kubectl configured for their workspace
11. **Lifetime expires** - After 2h (configurable), controller deletes Terminal CR
12. **Cleanup** - Finalizers ensure pod, service, and HTTPRoute are deleted

## Core Components

### 1. Terminal CRD

```yaml
apiVersion: terminal.platform-mesh.io/v1alpha1
kind: Terminal
metadata:
  name: my-terminal
# No spec needed - configuration driven by controller
spec: {}
status:
  # Current state: Pending, Creating, Ready, Failed, Terminating
  phase: Ready

  # UUID for non-guessable URL path
  sessionId: "550e8400-e29b-41d4-a716-446655440000"

  # User identity from KCP (sub claim from OIDC token)
  createdBy: "user@example.com"

  # Name of the created pod on runtime cluster
  podName: "terminal-my-terminal"

  # Resolved workspace path from AccountInfo
  workspacePath: "root:myorg:myteam:dev"

  # Conditions for detailed status
  conditions:
    - type: Ready
      status: "True"
      reason: "PodRunning"
      message: "Terminal pod is running and ready"
```

Note: The spec is intentionally empty. All configuration is controlled by the platform operator via controller flags.

### 2. Terminal Controller Manager

**Responsibilities:**

| Responsibility | Description |
|----------------|-------------|
| Watch Terminal CRDs | Reconcile desired state to actual state via APIExport |
| Create Terminal Pods | With ttyd, kubectl, KCP plugins, k9s (no credentials) |
| Create Services | ClusterIP service exposing pod port 8080 |
| Create HTTPRoutes | Gateway API routes with session-ID-based paths |
| Handle Lifecycle | TTL-based cleanup (default 2h) |
| Status Updates | Report phase, podName, sessionId, workspacePath |

**Reconciliation Flow:**

```
Terminal Created
       │
       ▼
┌──────────────────┐
│ Get AccountInfo  │ → Not Found → Requeue
└────────┬─────────┘
         │ Found
         ▼
┌──────────────────┐
│ Generate Session │ (UUID for URL path)
│ Capture Creator  │ (from KCP annotations)
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Create Pod       │ (ttyd + setup.sh, env vars for workspace)
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Create Service   │ (ClusterIP targeting pod)
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Create HTTPRoute │ (path: /terminals/{session-id})
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Wait for Ready   │ → Update status.phase
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Check Lifetime   │ → Expired → Delete Terminal CR
└──────────────────┘
```

**Subroutines:**

| Subroutine | Purpose |
|------------|---------|
| LifetimeSubroutine | Checks terminal age, triggers deletion when expired |
| PodSubroutine | Creates/manages terminal pod with ttyd |
| ServiceSubroutine | Creates ClusterIP service for pod |
| HTTPRouteSubroutine | Creates Gateway API HTTPRoute |

### 3. Terminal Pod Image

**Base:** Alpine 3.20

**Installed Tools:**
- kubectl (latest stable)
- KCP kubectl plugins (`kubectl ws`, `kubectl kcp`)
- k9s (terminal UI for Kubernetes)
- ttyd (WebSocket terminal server)
- bash, curl, jq, vim

**Dockerfile:**

```dockerfile
FROM alpine:3.20

# Install required tools including ttyd for WebSocket terminal
RUN apk add --no-cache \
    bash curl jq vim ca-certificates ttyd

# Install kubectl
RUN KUBECTL_VERSION=$(curl -Ls https://dl.k8s.io/release/stable.txt) && \
    curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" && \
    chmod +x kubectl && mv kubectl /usr/local/bin/

# Install kcp kubectl plugins
RUN KCP_VERSION=$(curl -s https://api.github.com/repos/kcp-dev/kcp/releases/latest | jq -r '.tag_name') && \
    curl -LO "https://github.com/kcp-dev/kcp/releases/download/${KCP_VERSION}/kubectl-kcp-plugin_${KCP_VERSION#v}_linux_amd64.tar.gz" && \
    curl -LO "https://github.com/kcp-dev/kcp/releases/download/${KCP_VERSION}/kubectl-ws-plugin_${KCP_VERSION#v}_linux_amd64.tar.gz" && \
    tar -xzf "kubectl-kcp-plugin_${KCP_VERSION#v}_linux_amd64.tar.gz" -C /tmp && \
    tar -xzf "kubectl-ws-plugin_${KCP_VERSION#v}_linux_amd64.tar.gz" -C /tmp && \
    mv /tmp/bin/kubectl-kcp /tmp/bin/kubectl-ws /usr/local/bin/

# Install k9s
RUN K9S_VERSION=$(curl -s https://api.github.com/repos/derailed/k9s/releases/latest | jq -r '.tag_name') && \
    curl -LO "https://github.com/derailed/k9s/releases/download/${K9S_VERSION}/k9s_Linux_amd64.tar.gz" && \
    tar -xzf k9s_Linux_amd64.tar.gz -C /tmp && mv /tmp/k9s /usr/local/bin/

# Non-root user
RUN adduser -D -u 1000 terminal
USER terminal
WORKDIR /home/terminal

COPY --chown=terminal:terminal setup.sh entrypoint.sh /home/terminal/
RUN chmod +x /home/terminal/setup.sh /home/terminal/entrypoint.sh

ENV KUBECONFIG=/tmp/kubeconfig
EXPOSE 8080

ENTRYPOINT ["/home/terminal/entrypoint.sh"]
```

**Entrypoint Script (entrypoint.sh):**

```bash
#!/bin/bash
set -e

# Validate required environment variables
if [ -z "${KCP_WORKSPACE_URL}" ]; then
    echo "ERROR: KCP_WORKSPACE_URL environment variable is not set" >&2
    exit 1
fi

# Start ttyd on port 8080
# - Runs setup.sh which reads token from first WebSocket message
# - setup.sh creates kubeconfig and starts interactive bash
exec ttyd \
    --port 8080 \
    --writable \
    --max-clients 1 \
    /home/terminal/setup.sh
```

**Setup Script (setup.sh):**

```bash
#!/bin/bash
set -e

TOKEN_FILE="/tmp/token"

echo "[setup] Terminal session starting..."
echo "[setup] Waiting for authentication token from client..."

# Disable echo to prevent token from being displayed
stty -echo 2>/dev/null || true

# Read token from stdin (sent by frontend as first message)
if ! read -r -t 30 TOKEN; then
    stty echo 2>/dev/null || true
    echo "[setup] ERROR: Timeout waiting for authentication token" >&2
    exit 1
fi

stty echo 2>/dev/null || true

# Validate user identity if EXPECTED_USER_ID is set
if [ -n "${EXPECTED_USER_ID}" ]; then
    echo "[setup] Validating user identity..."

    # Decode JWT payload and extract sub claim
    JWT_PAYLOAD_B64=$(echo "${TOKEN}" | cut -d'.' -f2 | tr '_-' '/+')
    JWT_PAYLOAD=$(echo "${JWT_PAYLOAD_B64}" | base64 -d 2>/dev/null || true)
    TOKEN_USER=$(echo "${JWT_PAYLOAD}" | jq -r '.sub // empty' 2>/dev/null || true)

    if [ "${TOKEN_USER}" != "${EXPECTED_USER_ID}" ]; then
        echo "[setup] ERROR: Access denied - user mismatch" >&2
        exit 1
    fi
    echo "[setup] User verified: ${TOKEN_USER}"
fi

# Write token to file and create kubeconfig
echo -n "${TOKEN}" > "${TOKEN_FILE}"
chmod 600 "${TOKEN_FILE}"

export KUBECONFIG=/tmp/kubeconfig
cat > "${KUBECONFIG}" << EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: ${KCP_WORKSPACE_URL}
    certificate-authority-data: ${KCP_CA_DATA}
  name: kcp
contexts:
- context:
    cluster: kcp
    user: user
  name: default
current-context: default
users:
- name: user
  user:
    token: ${TOKEN}
EOF
chmod 600 "${KUBECONFIG}"

# Clear token from environment
unset TOKEN

echo "[setup] Connected to KCP workspace: ${KCP_WORKSPACE_URL}"
echo ""
echo "Welcome to the Platform Mesh Terminal!"
echo ""
echo "Available tools:"
echo "  - kubectl (alias: k)  - Kubernetes CLI"
echo "  - k9s                 - Terminal UI"
echo ""

# Start interactive shell
exec /bin/bash
```

### 4. Frontend Integration

**Technology:**
- xterm.js for terminal rendering
- WebSocket connection via Gateway API HTTPRoute
- Integration with existing Angular app (portal / generic-resource-ui)

**Connection Flow:**

```
1. User clicks "Open Terminal" in UI
2. Frontend creates Terminal CR via Kubernetes API (empty spec)
3. Frontend polls/watches for Ready status
4. Frontend reads status.sessionId from Terminal CR
5. Frontend opens WebSocket to:
   wss://portal.example.com/terminals/{session-id}
6. Frontend sends OIDC token as first message (typed as input)
7. ttyd proxies to setup.sh, which validates and creates kubeconfig
8. xterm.js renders terminal, streams I/O over WebSocket
9. On close: Frontend deletes Terminal CR, triggering cleanup
```

**Frontend Pseudo-code:**

```typescript
async function openTerminal() {
  // 1. Create Terminal CR (empty spec - workspace derived from context)
  const terminal = await createTerminalCR({});

  // 2. Wait for Ready
  await waitForTerminalReady(terminal.name);

  // 3. Get session ID from status
  const sessionId = terminal.status.sessionId;

  // 4. Connect WebSocket via Gateway
  const ws = new WebSocket(`wss://portal.example.com/terminals/${sessionId}`);

  // 5. Send token as first message
  ws.onopen = () => {
    const token = getOIDCToken(); // From Keycloak
    ws.send(token + '\n'); // Token read via stdin in setup.sh
  };

  // 6. Connect to xterm.js
  const term = new Terminal();
  term.onData(data => ws.send(data));
  ws.onmessage = event => term.write(event.data);
}
```

## Security Considerations

### Authentication & Authorization

1. **User Authentication**: OIDC tokens from Keycloak (existing platform-mesh auth)
2. **Token Injection**: Frontend sends token over encrypted WebSocket, never stored
3. **User Validation**: setup.sh validates JWT `sub` claim matches `createdBy` in status
4. **Session Security**: UUID-based session IDs in URL paths prevent guessing
5. **Workspace Isolation**: Each terminal targets the workspace where it was created

### Token Security

| Aspect | Approach |
|--------|----------|
| Storage | Never stored in Kubernetes resources |
| Transport | Encrypted via TLS (wss://) |
| In Pod | Stored in tmpfs (memory only) |
| Lifetime | Managed by frontend, session ends on token expiry |
| Refresh | Token refresh function available: `__update_token__` |
| TLS Verification | Uses CA certificate from AccountInfo |

### Controller RBAC

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: terminal-controller-manager
rules:
  # Terminal CRD management (on KCP)
  - apiGroups: ["terminal.platform-mesh.io"]
    resources: ["terminals"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["terminal.platform-mesh.io"]
    resources: ["terminals/status"]
    verbs: ["get", "update", "patch"]
  # AccountInfo read (on KCP)
  - apiGroups: ["account.platform-mesh.io"]
    resources: ["accountinfos"]
    verbs: ["get", "list", "watch"]
  # Pod management (on runtime cluster)
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch", "create", "delete"]
  # Service management (on runtime cluster)
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  # HTTPRoute management (on runtime cluster)
  - apiGroups: ["gateway.networking.k8s.io"]
    resources: ["httproutes"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### Security Hardening

| Area | Measures |
|------|----------|
| Pod Security | Non-root user (UID 1000), read-only root filesystem, no privilege escalation, drop all capabilities, seccomp profile |
| Volumes | tmpfs for /tmp and /home/terminal (no persistent storage) |
| Token | tmpfs storage, cleared from environment after kubeconfig creation |
| Sessions | 2h lifetime (configurable), auto-cleanup on expiry |
| URL Security | UUID-based session IDs prevent URL guessing |
| User Validation | JWT `sub` claim must match terminal creator |

## Implementation Phases

### Phase 1: Core Controller (MVP) ✅
- [x] Terminal CRD definition (empty spec, status-driven)
- [x] Basic controller reconciliation (create pod, service, HTTPRoute)
- [x] Terminal pod image with ttyd, kubectl, KCP plugins, k9s
- [x] Setup script for token injection and user validation
- [x] Lifetime-based cleanup (2h default)
- [x] Session ID generation for URL security

### Phase 2: Helm Chart & Platform Integration
- [ ] Create helm chart in `helm-charts/terminal-controller-manager/`
  - Deployment with multicluster-runtime configuration
  - ServiceAccount with RBAC for pod/service/HTTPRoute management
  - ConfigMap for operator configuration
- [ ] Add to platform-mesh-operator for deployment
  - Add APIExport for `terminal.platform-mesh.io` API group
  - Add APIResourceSchema generated from CRDs (via apigen)
  - Add subroutine to deploy terminal-controller-manager HelmRelease
  - Feature toggle in PlatformMesh CR spec

### Phase 3: Test UI ✅
- [x] Minimal standalone HTML/JS test page
- [x] Manual token input for testing
- [x] xterm.js terminal rendering
- [x] Connect to local Kind cluster

### Phase 4: Frontend Integration
- [ ] xterm.js component in generic-resource-ui
- [ ] Terminal creation via Kubernetes API
- [ ] WebSocket connection via Gateway
- [ ] Session state management (NgRx)

### Phase 5: Production Hardening
- [ ] Network policies restricting egress
- [ ] Metrics and monitoring (Prometheus)
- [ ] Audit logging integration
- [ ] Rate limiting for terminal creation

## Directory Structure

```
terminal-controller-manager/
├── main.go                           # Thin wrapper → cmd.Execute()
├── cmd/
│   ├── root.go                       # Cobra CLI setup, scheme registration
│   └── operator.go                   # Manager initialization, controller setup
├── api/
│   └── v1alpha1/
│       ├── terminal_types.go         # CRD types (empty spec, status fields)
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
├── internal/
│   ├── config/
│   │   └── config.go                 # Operator configuration
│   └── controller/
│       └── terminal_controller.go    # Reconciler with LifecycleManager
├── pkg/
│   └── subroutines/
│       ├── lifetime.go               # TTL-based cleanup
│       ├── pod.go                    # Pod creation/deletion
│       ├── service.go                # Service creation/deletion
│       └── httproute.go              # HTTPRoute creation/deletion
├── config/
│   ├── crd/bases/                    # Generated CRDs (controller-gen)
│   ├── resources/                    # Generated APIResourceSchemas (apigen)
│   ├── rbac/                         # Controller RBAC
│   └── samples/                      # Example Terminal CR
├── images/
│   └── terminal/
│       ├── Dockerfile                # Terminal pod image
│       ├── entrypoint.sh             # ttyd startup
│       └── setup.sh                  # Token injection script
├── test-ui/
│   ├── index.html                    # Minimal xterm.js test UI
│   └── README.md                     # Test UI usage instructions
├── hack/
│   └── boilerplate.go.txt            # License header for generated files
├── Taskfile.yaml                     # Build tasks
├── go.mod
└── go.sum
```

## Configuration

Configuration is controlled via controller flags (viper-based):

| Flag | Default | Description |
|------|---------|-------------|
| `--terminal-image` | `ghcr.io/platform-mesh/terminal:latest` | Terminal pod image |
| `--terminal-namespace` | `terminal-sessions` | Namespace for terminal pods |
| `--terminal-lifetime` | `2h` | Terminal lifetime before auto-cleanup |
| `--terminal-host-alias-ip` | (none) | Host alias IP (for local dev) |
| `--terminal-host-alias-names` | (none) | Host alias names (for local dev) |
| `--gateway-name` | `k8sapi-gateway` | Gateway name for HTTPRoutes |
| `--gateway-namespace` | `platform-mesh-system` | Gateway namespace |
| `--gateway-hostnames` | `portal.localhost,*.portal.localhost` | Hostnames for HTTPRoutes |
| `--kcp-api-export-endpoint-slice-name` | `terminal.platform-mesh.io` | KCP APIExport name |
| `--kcp-kubeconfig` | (in-cluster) | Path to KCP kubeconfig |

## Technical Implementation

### Framework Stack

| Dependency | Purpose |
|------------|---------|
| [multicluster-runtime](https://github.com/kubernetes-sigs/multicluster-runtime) | Multi-cluster controller framework |
| [multicluster-provider](https://github.com/kcp-dev/multicluster-provider) | KCP APIExport provider for workspace discovery |
| platform-mesh/golang-commons | Lifecycle management, logging, configuration |
| gateway-api | HTTPRoute for terminal routing |
| ttyd | WebSocket terminal server in pod |

### Entry Point Pattern (main.go)

```go
package main

import "github.com/platform-mesh/terminal-controller-manager/cmd"

func main() {
    cmd.Execute()
}
```

### Dual-Client Setup (cmd/operator.go)

```go
func RunController(_ *cobra.Command, _ []string) {
    // KCP config for watching Terminal CRs via APIExport
    kcpCfg, _ := loadKcpConfig(operatorCfg.Kcp.Kubeconfig)

    // Runtime cluster config for pod/service/HTTPRoute management
    runtimeCfg := ctrl.GetConfigOrDie()

    // Create KCP APIExport provider
    provider, _ := apiexport.New(kcpCfg, operatorCfg.Kcp.APIExportEndpointSliceName, ...)

    // Create multicluster manager (connects to KCP)
    mgr, _ := mcmanager.New(kcpCfg, provider, ...)

    // Create runtime client for local resources
    runtimeClient, _ := client.New(runtimeCfg, client.Options{Scheme: scheme})

    // Pass both to reconciler
    terminalReconciler := controller.NewTerminalReconciler(log, mgr, operatorCfg, runtimeClient)
}
```

**Key Pattern:**
- `mcmanager` with `apiexport.Provider` → Watches Terminal CRs across KCP workspaces
- `client.New(runtimeCfg)` → Manages pods, services, HTTPRoutes on runtime cluster

### Subroutine Pattern

Each subroutine handles a specific resource type:

```go
type PodSubroutine struct {
    mgr            mcmanager.Manager  // For accessing KCP workspace clients
    runtimeClient  client.Client      // For managing pods on runtime cluster
    // ...
}

func (r *PodSubroutine) Process(ctx context.Context, ro runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError) {
    terminal := ro.(*v1alpha1.Terminal)

    // Get workspace info from KCP
    clusterName, _ := mccontext.ClusterFrom(ctx)
    cluster, _ := r.mgr.GetCluster(ctx, clusterName)
    accountInfo := &accountv1alpha1.AccountInfo{}
    cluster.GetClient().Get(ctx, client.ObjectKey{Name: "account"}, accountInfo)

    // Create pod on runtime cluster
    pod := r.buildTerminalPod(terminal, accountInfo.Spec.Account.URL, accountInfo.Spec.ClusterInfo.CA)
    r.runtimeClient.Create(ctx, pod)

    return ctrl.Result{}, nil
}

func (r *PodSubroutine) Finalize(ctx context.Context, ro runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError) {
    terminal := ro.(*v1alpha1.Terminal)

    // Delete pod from runtime cluster
    pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: terminal.Status.PodName, Namespace: r.namespace}}
    r.runtimeClient.Delete(ctx, pod)

    return ctrl.Result{}, nil
}
```

## Open Questions

### 1. Token expiry handling
**Problem:** OIDC tokens expire, long sessions may fail mid-command.

**Current approach:** Session ends on token expiry. Token refresh function `__update_token__` is available but requires frontend support to call it.

**Future options:**
- Frontend detects 401 errors and sends refreshed token
- Background token refresh via portal signaling

### 2. Multi-workspace support
**Question:** One terminal per workspace, or allow switching?

**Current:** One terminal per workspace. `kubectl ws` plugin allows switching workspaces within session if user's token has permissions.

### 3. Idle timeout vs lifetime
**Question:** Should terminals have idle detection in addition to fixed lifetime?

**Current:** Only fixed lifetime (2h default). No idle detection implemented.

**Future:** Could track last activity time and add idle timeout subroutine.

## Comparison with Gardener

| Aspect | Gardener | Platform-Mesh |
|--------|----------|---------------|
| Target | Shoots, Seeds, Garden | KCP Workspaces |
| Auth | ServiceAccount, ShootRef | OIDC tokens (frontend-injected) |
| Token Storage | Secrets in cluster | Never stored (tmpfs only) |
| WebSocket | Dashboard proxies | ttyd + Gateway API HTTPRoutes |
| Clusters | Multi-cluster (host/target) | Split-cluster (KCP + runtime) |
| Frontend | Vue.js | Angular |
| CRD | Full spec with target/host | Empty spec (controller-driven) |
| Terminal Server | Custom exec proxy | ttyd |

## References

- [RFC 002: Terminal Controller Manager](https://github.com/platform-mesh/architecture/pull/8) - Architecture RFC for this project
- [Gardener terminal-controller-manager](https://github.com/gardener/terminal-controller-manager)
- [ttyd - Share your terminal over the web](https://github.com/tsl0922/ttyd)
- [xterm.js](https://xtermjs.org/)
- [KCP](https://github.com/kcp-dev/kcp)
- [Gateway API](https://gateway-api.sigs.k8s.io/)
- [multicluster-runtime](https://github.com/kubernetes-sigs/multicluster-runtime)