# Terminal Controller Manager - Concept Document

## Context

Platform-mesh needs a browser-based terminal to connect to KCP (Kubernetes Control Plane) for running kubectl commands. Users should be able to open a terminal in the UI and interact with their KCP workspaces using kubectl and KCP plugins.

This implementation is inspired by [Gardener's terminal-controller-manager](https://github.com/gardener/terminal-controller-manager) but tailored specifically for KCP and platform-mesh architecture.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  Browser (generic-resource-ui / portal)                             │
│  ├─ xterm.js terminal emulator                                      │
│  ├─ Holds OIDC token (from Keycloak)                                │
│  └─ Connects to Terminal Controller WebSocket endpoint              │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ WebSocket (wss://terminal-controller/ws)
                           │ Token sent as first message
                           ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Runtime Cluster                                                    │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  Terminal Controller Manager                                   │  │
│  │  ├─ Watches Terminal CRDs (via KCP APIExport)                  │  │
│  │  ├─ Creates/manages terminal pods (on runtime cluster)         │  │
│  │  ├─ Exposes WebSocket endpoint for frontend                    │  │
│  │  └─ Proxies WebSocket to pod exec API                          │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                              │                                       │
│                              ▼                                       │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  Terminal Pod (ephemeral)                                      │  │
│  │  ├─ kubectl + KCP plugins pre-installed                        │  │
│  │  ├─ Reads token from stdin, creates kubeconfig in tmpfs        │  │
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
│     └─ Terminal CRs created here (user-facing API)                  │
└─────────────────────────────────────────────────────────────────────┘
```

### Multi-Cluster Design

The operator follows a **split-cluster architecture**:

| Cluster | Resources | Purpose |
|---------|-----------|---------|
| KCP (virtual workspaces) | Terminal CRs | User-facing API, watched via APIExport |
| Runtime cluster | Terminal pods, controller | Actual pod execution, WebSocket proxy |

This means:
- Users create `Terminal` resources in their KCP workspace
- Controller watches across all KCP workspaces via multicluster-runtime + apiexport provider
- Pods are created on the runtime cluster where the controller runs
- The controller needs two clients: one for KCP (watching CRs) and one for runtime (managing pods)

## Key Design Decision: Frontend Token Injection

**Problem:** Storing OIDC tokens in Kubernetes secrets creates security risks and complicates token lifecycle management.

**Solution:** The frontend passes the OIDC token at WebSocket connection time. The token is never stored in Kubernetes resources.

**Benefits:**
- Token never persisted in etcd/secrets
- Token lifecycle managed by frontend (refresh handled there)
- Simpler Terminal CRD (no credentials spec)
- Better security posture

## How It Works

1. **User requests terminal** - Frontend creates a `Terminal` custom resource (no credentials)
2. **Controller reconciles** - Creates pod with kubectl (no kubeconfig)
3. **Pod becomes ready** - Status updated with pod name
4. **Frontend connects** - WebSocket to controller's `/ws/{terminal-name}` endpoint
5. **Token handshake** - Frontend sends OIDC token as first message
6. **Pod receives token** - Setup script reads token, creates kubeconfig in tmpfs
7. **Shell ready** - User gets interactive shell with kubectl configured
8. **Session ends** - Terminal CR deleted, pod cleaned up

## Core Components

### 1. Terminal CRD

```yaml
apiVersion: terminal.platform-mesh.io/v1alpha1
kind: Terminal
metadata:
  name: user-abc123-workspace-xyz
  namespace: terminal-system
spec:
  # Target KCP workspace to connect to
  target:
    # KCP workspace path (e.g., "root:org:team:workspace")
    workspacePath: "root:myorg:myteam:dev"

    # KCP API server URL
    apiServerURL: "https://kcp.example.com"

    # Optional: specific namespace context
    namespace: "default"

  # Host cluster configuration (where terminal pod runs)
  host:
    # Namespace for terminal pod
    namespace: "terminal-sessions"

    # Create temporary namespace per session
    temporaryNamespace: false

    # Pod specification
    pod:
      image: "ghcr.io/platform-mesh/terminal:latest"
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
        limits:
          cpu: "500m"
          memory: "512Mi"

      # TTL after which idle terminal is cleaned up
      idleTimeout: "30m"

      # Maximum session duration
      maxSessionDuration: "4h"

status:
  # Current state: Pending, Creating, Ready, Failed, Terminating
  phase: Ready

  # Name of the created pod
  podName: "terminal-user-abc123-xyz"

  # When the terminal was last accessed
  lastActivityTime: "2024-01-15T10:30:00Z"

  # Conditions for detailed status
  conditions:
    - type: PodReady
      status: "True"
      reason: "PodRunning"
      message: "Terminal pod is running and ready"
```

Note: No `credentials` field - token is passed at connection time.

### 2. Terminal Controller Manager

**Responsibilities:**

| Responsibility | Description |
|----------------|-------------|
| Watch Terminal CRDs | Reconcile desired state to actual state |
| Create Terminal Pods | With kubectl, KCP plugins (no credentials) |
| WebSocket Endpoint | Accept frontend connections at `/ws/{terminal-name}` |
| Proxy to Pod Exec | Forward WebSocket traffic to pod exec API |
| Token Forwarding | Pass frontend token as first stdin message to pod |
| Handle Lifecycle | TTL-based cleanup, idle timeout detection |
| Status Updates | Report pod status, last activity |

**Reconciliation Flow:**

```
Terminal Created
       │
       ▼
┌──────────────────┐
│ Validate Spec    │ → Invalid → Set Failed status
└────────┬─────────┘
         │ Valid
         ▼
┌──────────────────┐
│ Create Namespace │ (if temporaryNamespace: true)
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Create Pod       │ (terminal image, no kubeconfig)
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Wait for Ready   │ → Update status.podName
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Set Ready Status │
└──────────────────┘
```

**WebSocket Connection Flow:**

```
Frontend connects to wss://terminal-controller/ws/{terminal-name}
                           │
                           ▼
┌──────────────────────────────────────┐
│ Validate Terminal CR exists & Ready  │ → Not Ready → Return error
└────────────────┬─────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────┐
│ Open exec WebSocket to terminal pod  │
└────────────────┬─────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────┐
│ Wait for token message from frontend │
└────────────────┬─────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────┐
│ Forward token to pod stdin           │
└────────────────┬─────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────┐
│ Bidirectional proxy: frontend ↔ pod  │
└──────────────────────────────────────┘
```

### 3. Terminal Pod Image

**Base Requirements:**
- Alpine or distroless base for minimal size
- kubectl (latest stable)
- KCP kubectl plugins (`kubectl ws`, etc.)
- bash/zsh shell
- Common tools: curl, jq, yq, vim/nano
- Setup script that handles token injection

**Dockerfile Concept:**

```dockerfile
FROM alpine:3.19

# Install kubectl
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" \
    && chmod +x kubectl \
    && mv kubectl /usr/local/bin/

# Install KCP plugins
RUN curl -LO "https://github.com/kcp-dev/kcp/releases/latest/download/kubectl-kcp-plugin_linux_amd64.tar.gz" \
    && tar -xzf kubectl-kcp-plugin_linux_amd64.tar.gz \
    && mv kubectl-* /usr/local/bin/

# Install common tools
RUN apk add --no-cache bash curl jq yq vim

# Non-root user
RUN adduser -D -u 1000 terminal
USER terminal
WORKDIR /home/terminal

# Copy setup script
COPY --chown=terminal:terminal setup.sh /home/terminal/setup.sh
RUN chmod +x /home/terminal/setup.sh

# Kubeconfig will be created at runtime in tmpfs
ENV KUBECONFIG=/tmp/kubeconfig

ENTRYPOINT ["/home/terminal/setup.sh"]
```

**Setup Script (setup.sh):**

```bash
#!/bin/bash
set -e

# Read token from first line of stdin (sent by controller)
read -r TOKEN

# KCP API server URL is passed as environment variable
KCP_SERVER="${KCP_API_SERVER:-https://kcp.example.com}"
WORKSPACE_PATH="${KCP_WORKSPACE_PATH:-root}"

# Create kubeconfig in tmpfs (memory only, not persisted)
cat > /tmp/kubeconfig << EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: ${KCP_SERVER}/clusters/${WORKSPACE_PATH}
    insecure-skip-tls-verify: false
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

# Clear token from environment
unset TOKEN

echo "Connected to KCP workspace: ${WORKSPACE_PATH}"
echo "Type 'kubectl get pods' to test connectivity"
echo ""

# Drop to interactive shell
exec /bin/bash
```

### 4. Frontend Integration

**Technology:**
- xterm.js for terminal rendering
- WebSocket connection to Terminal Controller
- Integration with existing Angular app (generic-resource-ui)

**Connection Flow:**

```
1. User clicks "Open Terminal" in UI
2. Frontend creates Terminal CR via Kubernetes API
3. Frontend polls/watches for Ready status
4. Frontend opens WebSocket to:
   wss://terminal-controller.terminal-system.svc/ws/{terminal-name}
5. Frontend sends OIDC token as first message
6. Controller proxies to pod, pod creates kubeconfig
7. xterm.js renders terminal, streams I/O over WebSocket
8. On close: Terminal CR deleted, pod cleaned up
```

**Frontend Pseudo-code:**

```typescript
async function openTerminal(workspacePath: string) {
  // 1. Create Terminal CR
  const terminal = await createTerminalCR({
    target: {
      workspacePath,
      apiServerURL: 'https://kcp.example.com'
    }
  });

  // 2. Wait for Ready
  await waitForTerminalReady(terminal.name);

  // 3. Connect WebSocket
  const ws = new WebSocket(`wss://terminal-controller/ws/${terminal.name}`);

  // 4. Send token as first message
  ws.onopen = () => {
    const token = getOIDCToken(); // From Keycloak
    ws.send(token);
  };

  // 5. Connect to xterm.js
  const term = new Terminal();
  term.onData(data => ws.send(data));
  ws.onmessage = event => term.write(event.data);
}
```

## Security Considerations

### Authentication & Authorization

1. **User Authentication**: OIDC tokens from Keycloak (existing platform-mesh auth)
2. **Token Injection**: Frontend sends token over encrypted WebSocket, never stored
3. **Exec Authorization**: Controller uses its service account to exec into pods
4. **Workspace Isolation**: Each terminal targets a specific KCP workspace

### Token Security

| Aspect | Approach |
|--------|----------|
| Storage | Never stored in Kubernetes resources |
| Transport | Encrypted via TLS (wss://) |
| In Pod | Stored in tmpfs (memory only) |
| Lifetime | Managed by frontend, session ends on token expiry |
| Refresh | Frontend reconnects with new token if needed |

### Controller RBAC

```yaml
# Controller needs to exec into terminal pods
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: terminal-controller-manager
rules:
  # Terminal CRD management
  - apiGroups: ["terminal.platform-mesh.io"]
    resources: ["terminals"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["terminal.platform-mesh.io"]
    resources: ["terminals/status"]
    verbs: ["get", "update", "patch"]
  # Pod management
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create"]
  # Namespace management (if temporaryNamespace enabled)
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "watch", "create", "delete"]
```

### Security Hardening

| Area | Measures |
|------|----------|
| Pod Security | Non-root user, read-only root filesystem (except /tmp), no privileged escalation, resource limits |
| Network | Network policies restricting egress to KCP API only |
| Token | tmpfs storage, cleared from environment after kubeconfig creation |
| Audit | Terminal creation/deletion logged, commands auditable via KCP audit logs |
| Sessions | Idle timeout (30m default), max duration (4h), auto-cleanup on logout |

## Implementation Phases

### Phase 1: Core Controller (MVP)
- [ ] Terminal CRD definition (without credentials)
- [ ] Basic controller reconciliation (create pod, update status)
- [ ] Terminal pod image with kubectl + KCP plugins
- [ ] Setup script for token injection
- [ ] Simple cleanup on Terminal deletion

### Phase 2: Helm Chart & Platform Integration
- [ ] Create helm chart in `helm-charts/terminal-controller-manager/`
  - Deployment with multicluster-runtime configuration
  - ServiceAccount with RBAC for pod management
  - Service for WebSocket endpoint
  - ConfigMap for operator configuration
- [ ] Add to platform-mesh-operator for deployment
  - Add APIExport for `terminal.platform-mesh.io` API group
  - Add APIResourceSchema generated from CRDs (via apigen)
  - Add subroutine to deploy terminal-controller-manager HelmRelease
  - Feature toggle in PlatformMesh CR spec

### Phase 3: WebSocket Proxy
- [ ] WebSocket endpoint in controller (`/ws/{terminal-name}`)
- [ ] Token handshake protocol
- [ ] Bidirectional proxy to pod exec
- [ ] Connection lifecycle management

### Phase 4: Lifecycle Management
- [ ] Idle timeout detection (via activity tracking)
- [ ] TTL-based automatic cleanup
- [ ] Temporary namespace support
- [ ] Graceful shutdown handling

### Phase 5: Test UI
- [ ] Minimal standalone HTML/JS test page
- [ ] Manual token input for testing
- [ ] xterm.js terminal rendering
- [ ] Connect to local Kind cluster

### Phase 6: Frontend Integration
- [ ] xterm.js component in generic-resource-ui
- [ ] Terminal creation via Kubernetes API
- [ ] WebSocket connection to controller
- [ ] Session state management (NgRx)

### Phase 7: Production Hardening
- [ ] Pod security policies / Pod Security Standards
- [ ] Network policies
- [ ] Metrics and monitoring (Prometheus)
- [ ] Audit logging integration

## Directory Structure

```
terminal-controller-manager/
├── api/
│   └── v1alpha1/
│       ├── terminal_types.go      # CRD types
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
├── cmd/
│   └── manager/
│       └── main.go                # Entry point
├── internal/
│   ├── controller/
│   │   ├── terminal_controller.go # Main reconciliation
│   │   └── pod_builder.go         # Pod spec construction
│   └── websocket/
│       ├── handler.go             # WebSocket endpoint handler
│       └── proxy.go               # WebSocket to exec proxy
├── config/
│   ├── crd/
│   │   └── bases/                 # Generated CRDs
│   ├── rbac/                      # Controller RBAC
│   └── manager/                   # Deployment manifests
├── images/
│   └── terminal/
│       ├── Dockerfile             # Terminal pod image
│       └── setup.sh               # Token injection script
├── helm-charts/
│   └── terminal-controller-manager/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── Taskfile.yaml
├── go.mod
├── go.sum
└── README.md
```

## Technical Implementation

### Framework Stack

| Dependency | Purpose |
|------------|---------|
| [multicluster-runtime](https://github.com/kubernetes-sigs/multicluster-runtime) | Multi-cluster controller framework |
| [multicluster-provider](https://github.com/kcp-dev/multicluster-provider) | KCP APIExport provider for workspace discovery |
| platform-mesh/golang-commons | Lifecycle management, logging, configuration |
| gorilla/websocket | WebSocket handling for terminal connections |

### Project Structure (Platform-Mesh Pattern)

```
terminal-controller-manager/
├── main.go                           # Thin wrapper → cmd.Execute()
├── cmd/
│   ├── root.go                       # Cobra CLI setup, scheme registration
│   └── operator.go                   # Manager initialization, controller setup
├── api/
│   └── v1alpha1/
│       ├── terminal_types.go         # CRD types with conditions
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
├── internal/
│   ├── controller/
│   │   └── terminal_controller.go    # Reconciler with LifecycleManager
│   ├── subroutines/
│   │   ├── pod.go                    # Pod creation/deletion subroutine
│   │   ├── namespace.go              # Temporary namespace subroutine
│   │   └── cleanup.go                # TTL/idle cleanup subroutine
│   └── websocket/
│       ├── handler.go                # WebSocket endpoint handler
│       └── proxy.go                  # WebSocket to exec proxy
├── pkg/
│   └── config/
│       └── config.go                 # Operator-specific configuration
├── config/
│   ├── crd/bases/                    # Generated CRDs (controller-gen)
│   ├── resources/                    # Generated APIResourceSchemas (apigen)
│   ├── rbac/                         # Controller RBAC
│   └── manager/                      # Deployment manifests
├── hack/
│   └── boilerplate.go.txt            # License header for generated files
├── test-ui/
│   ├── index.html                    # Minimal xterm.js test UI
│   └── README.md                     # Test UI usage instructions
├── images/
│   └── terminal/
│       ├── Dockerfile
│       └── setup.sh
└── helm-charts/
    └── terminal-controller-manager/
```

### Build Tasks (Taskfile.yaml)

Key tasks following the platform-mesh pattern:

```yaml
version: '3'

vars:
  LOCAL_BIN: bin
  CRD_DIRECTORY: config/crd/bases
  APIRESOURCESCHEMA_DIRECTORY: config/resources
  CONTROLLER_GEN_VERSION: v0.16.1
  KCP_APIGEN_VERSION: v0.26.0
  GOLANGCI_LINT_VERSION: v2.1.6

tasks:
  ## Setup
  setup:controller-gen:
    internal: true
    cmds:
      - test -s {{.LOCAL_BIN}}/controller-gen || GOBIN=$(pwd)/{{.LOCAL_BIN}} go install sigs.k8s.io/controller-tools/cmd/controller-gen@{{.CONTROLLER_GEN_VERSION}}

  setup:kcp-api-gen:
    internal: true
    cmds:
      - test -s {{.LOCAL_BIN}}/apigen || GOBIN=$(pwd)/{{.LOCAL_BIN}} go install github.com/kcp-dev/sdk/cmd/apigen@{{.KCP_APIGEN_VERSION}}

  ## Code Generation
  manifests:
    desc: Generate CRD manifests
    deps: [setup:controller-gen, setup:kcp-api-gen]
    cmds:
      - "{{.LOCAL_BIN}}/controller-gen rbac:roleName=manager-role crd paths=./... output:crd:artifacts:config={{.CRD_DIRECTORY}}"

  generate:
    desc: Generate code, CRDs, and APIResourceSchemas
    cmds:
      - task: manifests
      - "{{.LOCAL_BIN}}/controller-gen object:headerFile=hack/boilerplate.go.txt paths=./..."
      - "{{.LOCAL_BIN}}/apigen --input-dir {{.CRD_DIRECTORY}} --output-dir {{.APIRESOURCESCHEMA_DIRECTORY}}"

  ## Development
  lint:
    desc: Run golangci-lint
    deps: [setup:golangci-lint]
    cmds:
      - "{{.LOCAL_BIN}}/golangci-lint run"

  test:
    desc: Run tests with envtest
    cmds:
      - go test ./... -coverprofile cover.out

  build:
    desc: Build the binary
    cmds:
      - go build -o {{.LOCAL_BIN}}/terminal-controller-manager main.go

  run:
    desc: Run locally
    cmds:
      - go run main.go operator

  validate:
    desc: Run lint + test
    cmds:
      - task: lint
      - task: test

  ## Docker / Local Development
  docker-build:
    desc: Build Docker image
    cmds:
      - "{{.CONTAINER_RUNTIME}} build -t {{.IMAGE_NAME}}:latest ."

  docker:kind:
    desc: Build image and load into local Kind cluster running platform-mesh
    vars:
      CONTAINER_RUNTIME: '{{.CONTAINER_RUNTIME | default "docker"}}'
      KIND_CLUSTER: '{{.KIND_CLUSTER | default "platform-mesh"}}'
      DEPLOYMENT_NAME: '{{.DEPLOYMENT_NAME | default "terminal-controller-manager"}}'
      DEPLOYMENT_NAMESPACE: '{{.DEPLOYMENT_NAMESPACE | default "platform-mesh-system"}}'
      IMAGE_TAG:
        sh: kubectl get deployment {{.DEPLOYMENT_NAME}} -n {{.DEPLOYMENT_NAMESPACE}} -o jsonpath='{.spec.template.spec.containers[0].image}' | cut -d':' -f2
      IMAGE_NAME: ghcr.io/platform-mesh/terminal-controller-manager:{{.IMAGE_TAG}}
    cmds:
      - echo "Building image with tag {{.IMAGE_TAG}} using {{.CONTAINER_RUNTIME}}"
      - "{{.CONTAINER_RUNTIME}} build -t {{.IMAGE_NAME}} ."
      - |
        if [ "{{.CONTAINER_RUNTIME}}" = "podman" ]; then
          {{.CONTAINER_RUNTIME}} save {{.IMAGE_NAME}} -o /tmp/kind-image.tar
          kind load image-archive /tmp/kind-image.tar --name {{.KIND_CLUSTER}}
          rm -f /tmp/kind-image.tar
        else
          kind load docker-image {{.IMAGE_NAME}} --name {{.KIND_CLUSTER}}
        fi
      - kubectl rollout restart deployment/{{.DEPLOYMENT_NAME}} -n {{.DEPLOYMENT_NAMESPACE}}
      - echo "Image loaded and deployment restarted"

  ## Test UI
  test-ui:
    desc: Serve the test UI for local development
    dir: test-ui
    cmds:
      - echo "Starting test UI at http://localhost:3000"
      - echo "Make sure to port-forward the controller: kubectl port-forward svc/terminal-controller-manager 8080:8080 -n platform-mesh-system"
      - npx serve -l 3000 .
```

The `apigen` tool converts CRDs to KCP APIResourceSchemas, which are required for exposing the Terminal API via KCP's APIExport mechanism.

### Entry Point Pattern (main.go)

```go
package main

import "github.com/platform-mesh/terminal-controller-manager/cmd"

func main() {
    cmd.Execute()
}
```

### Command Setup Pattern (cmd/root.go)

```go
package cmd

import (
    "github.com/platform-mesh/golang-commons/config"
    "github.com/platform-mesh/golang-commons/logger"
    terminalv1alpha1 "github.com/platform-mesh/terminal-controller-manager/api/v1alpha1"
    operatorconfig "github.com/platform-mesh/terminal-controller-manager/pkg/config"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    utilruntime "k8s.io/apimachinery/pkg/util/runtime"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
)

var (
    scheme      = runtime.NewScheme()
    operatorCfg operatorconfig.OperatorConfig
    defaultCfg  *config.CommonServiceConfig
    v           *viper.Viper
    log         *logger.Logger
)

var rootCmd = &cobra.Command{
    Use:   "terminal-controller-manager",
    Short: "Controller for managing browser-based terminal sessions to KCP workspaces",
}

func init() {
    // Register schemes
    utilruntime.Must(terminalv1alpha1.AddToScheme(scheme))

    rootCmd.AddCommand(operatorCmd)

    // Initialize common config with platform-mesh defaults
    var err error
    v, defaultCfg, err = config.NewDefaultConfig(rootCmd)
    if err != nil {
        panic(err)
    }

    // Bind operator-specific config
    err = config.BindConfigToFlags(v, operatorCmd, &operatorCfg)
    if err != nil {
        panic(err)
    }

    cobra.OnInitialize(initLog)
}

func initLog() {
    logcfg := logger.DefaultConfig()
    logcfg.Level = defaultCfg.Log.Level
    logcfg.NoJSON = defaultCfg.Log.NoJson

    var err error
    log, err = logger.New(logcfg)
    if err != nil {
        panic(err)
    }
    ctrl.SetLogger(log.Logr())
}

func Execute() {
    cobra.CheckErr(rootCmd.Execute())
}
```

### Operator-Specific Configuration (pkg/config/config.go)

```go
package config

type OperatorConfig struct {
    Kcp struct {
        ApiExportEndpointSliceName string `mapstructure:"api-export-endpoint-slice-name"`
    } `mapstructure:"kcp"`

    WebSocket struct {
        BindAddress string `mapstructure:"bind-address"`
    } `mapstructure:"websocket"`

    Terminal struct {
        DefaultImage           string `mapstructure:"default-image"`
        DefaultIdleTimeout     string `mapstructure:"default-idle-timeout"`
        DefaultMaxDuration     string `mapstructure:"default-max-duration"`
        SessionNamespace       string `mapstructure:"session-namespace"`
    } `mapstructure:"terminal"`

    Subroutines struct {
        Pod struct {
            Enabled bool `mapstructure:"enabled"`
        } `mapstructure:"pod"`
        Namespace struct {
            Enabled bool `mapstructure:"enabled"`
        } `mapstructure:"namespace"`
        Cleanup struct {
            Enabled bool `mapstructure:"enabled"`
        } `mapstructure:"cleanup"`
    } `mapstructure:"subroutines"`
}
```

### Controller Startup Pattern (cmd/operator.go)

```go
package cmd

import (
    "context"
    "net/http"

    "github.com/kcp-dev/multicluster-provider/apiexport"
    pmcontext "github.com/platform-mesh/golang-commons/context"
    "github.com/platform-mesh/terminal-controller-manager/internal/controller"
    "github.com/spf13/cobra"
    "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
    ctrl "sigs.k8s.io/controller-runtime"
    mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

var operatorCmd = &cobra.Command{
    Use:   "operator",
    Short: "Start the terminal controller manager",
    Run:   RunController,
}

func RunController(_ *cobra.Command, _ []string) {
    // Initialize context with lifecycle management
    ctx, _, shutdown := pmcontext.StartContext(log, operatorCfg, defaultCfg.ShutdownTimeout)
    defer shutdown()

    // Setup REST config with tracing
    restCfg := ctrl.GetConfigOrDie()
    restCfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
        return otelhttp.NewTransport(rt)
    })

    // Create KCP APIExport provider for workspace discovery
    provider, err := apiexport.New(restCfg, operatorCfg.Kcp.ApiExportEndpointSliceName, apiexport.Options{
        Log:    &ctrl.Log,
        Scheme: scheme,
    })
    if err != nil {
        log.Fatal().Err(err).Msg("creating APIExport provider")
    }

    // Create multicluster manager
    mgr, err := mcmanager.New(restCfg, provider, mcmanager.Options{
        Scheme:                        scheme,
        BaseContext:                   func() context.Context { return ctx },
        HealthProbeBindAddress:        defaultCfg.HealthProbeBindAddress,
        LeaderElection:                defaultCfg.LeaderElection.Enabled,
        LeaderElectionID:              "terminal-controller-manager.platform-mesh.io",
        LeaderElectionReleaseOnCancel: true,
    })
    if err != nil {
        log.Fatal().Err(err).Msg("unable to start manager")
    }

    // Setup Terminal reconciler
    terminalReconciler := controller.NewTerminalReconciler(log, mgr, operatorCfg)
    if err := terminalReconciler.SetupWithManager(mgr, defaultCfg, log); err != nil {
        log.Fatal().Err(err).Str("controller", "Terminal").Msg("unable to create controller")
    }

    // Setup health checks
    if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
        log.Fatal().Err(err).Msg("unable to set up health check")
    }
    if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
        log.Fatal().Err(err).Msg("unable to set up ready check")
    }

    // Start WebSocket server (in separate goroutine)
    go startWebSocketServer(ctx, mgr, log)

    // Start manager
    log.Info().Msg("starting manager")
    if err := mgr.Start(ctx); err != nil {
        log.Fatal().Err(err).Msg("problem running manager")
    }
}
```

### Reconciler with Lifecycle Manager (internal/controller/terminal_controller.go)

```go
package controller

import (
    "context"

    "github.com/platform-mesh/golang-commons/controller/lifecycle/builder"
    mclifecycle "github.com/platform-mesh/golang-commons/controller/lifecycle/multicluster"
    "github.com/platform-mesh/golang-commons/controller/lifecycle/subroutine"
    "github.com/platform-mesh/golang-commons/logger"
    terminalv1alpha1 "github.com/platform-mesh/terminal-controller-manager/api/v1alpha1"
    "github.com/platform-mesh/terminal-controller-manager/internal/subroutines"
    "github.com/platform-mesh/terminal-controller-manager/pkg/config"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/predicate"
    mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
    mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

type TerminalReconciler struct {
    cfg       config.OperatorConfig
    lifecycle *mclifecycle.LifecycleManager
}

func NewTerminalReconciler(
    log *logger.Logger,
    mgr mcmanager.Manager,
    cfg config.OperatorConfig,
) *TerminalReconciler {
    // Compose subroutines based on configuration
    subs := []subroutine.Subroutine{}

    if cfg.Subroutines.Namespace.Enabled {
        subs = append(subs, subroutines.NewNamespaceSubroutine(mgr))
    }

    if cfg.Subroutines.Pod.Enabled {
        subs = append(subs, subroutines.NewPodSubroutine(mgr, cfg))
    }

    if cfg.Subroutines.Cleanup.Enabled {
        subs = append(subs, subroutines.NewCleanupSubroutine(mgr, cfg))
    }

    return &TerminalReconciler{
        cfg: cfg,
        lifecycle: builder.NewBuilder("terminal-controller-manager", "TerminalReconciler", subs, log).
            WithConditionManagement().
            BuildMultiCluster(mgr),
    }
}

func (r *TerminalReconciler) SetupWithManager(
    mgr mcmanager.Manager,
    cfg *config.CommonServiceConfig,
    log *logger.Logger,
    eventPredicates ...predicate.Predicate,
) error {
    return r.lifecycle.SetupWithManager(
        mgr,
        cfg.MaxConcurrentReconciles,
        "TerminalReconciler",
        &terminalv1alpha1.Terminal{},
        cfg.DebugLabelValue,
        r,
        log,
        eventPredicates...,
    )
}

func (r *TerminalReconciler) Reconcile(
    ctx context.Context,
    req mcreconcile.Request,
) (ctrl.Result, error) {
    return r.lifecycle.Reconcile(ctx, req, &terminalv1alpha1.Terminal{})
}
```

### Subroutine Example (internal/subroutines/pod.go)

This subroutine demonstrates the **dual-client pattern**: Terminal CRs are read from KCP workspaces (via multicluster manager), but pods are managed on the runtime cluster (via separate in-cluster client).

```go
package subroutines

import (
    "context"
    "fmt"
    "time"

    "github.com/platform-mesh/golang-commons/controller/lifecycle/errors"
    "github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
    "github.com/platform-mesh/golang-commons/logger"
    terminalv1alpha1 "github.com/platform-mesh/terminal-controller-manager/api/v1alpha1"
    "github.com/platform-mesh/terminal-controller-manager/pkg/config"
    corev1 "k8s.io/api/core/v1"
    kerrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

const podFinalizer = "terminal.platform-mesh.io/pod-finalizer"

type PodSubroutine struct {
    runtimeClient client.Client  // Separate client for runtime cluster (pod management)
    cfg           config.OperatorConfig
}

func NewPodSubroutine(runtimeClient client.Client, cfg config.OperatorConfig) *PodSubroutine {
    return &PodSubroutine{
        runtimeClient: runtimeClient,
        cfg:           cfg,
    }
}

func (s *PodSubroutine) GetName() string {
    return "PodSubroutine"
}

func (s *PodSubroutine) Finalizers(_ runtimeobject.RuntimeObject) []string {
    return []string{podFinalizer}
}

func (s *PodSubroutine) Process(ctx context.Context, ro runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError) {
    // Terminal CR comes from KCP workspace (via multicluster manager)
    terminal := ro.(*terminalv1alpha1.Terminal)
    log := logger.LoadLoggerFromContext(ctx)

    // Pods are managed on the RUNTIME cluster using separate client
    podName := fmt.Sprintf("terminal-%s", terminal.Name)
    namespace := s.cfg.Terminal.SessionNamespace

    // Check if pod already exists on runtime cluster
    existingPod := &corev1.Pod{}
    err := s.runtimeClient.Get(ctx, client.ObjectKey{
        Namespace: namespace,
        Name:      podName,
    }, existingPod)

    if err == nil {
        if existingPod.Status.Phase == corev1.PodRunning {
            log.Info().Str("pod", podName).Msg("terminal pod is running")
            return ctrl.Result{}, nil
        }
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    if !kerrors.IsNotFound(err) {
        return ctrl.Result{}, errors.NewOperatorError(err, true, true)
    }

    // Create new pod on runtime cluster
    pod := s.buildTerminalPod(terminal, podName, namespace)
    if err := s.runtimeClient.Create(ctx, pod); err != nil {
        return ctrl.Result{}, errors.NewOperatorError(err, true, true)
    }

    log.Info().Str("pod", podName).Str("namespace", namespace).Msg("created terminal pod on runtime cluster")
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (s *PodSubroutine) Finalize(ctx context.Context, ro runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError) {
    terminal := ro.(*terminalv1alpha1.Terminal)
    log := logger.LoadLoggerFromContext(ctx)

    podName := fmt.Sprintf("terminal-%s", terminal.Name)
    pod := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Namespace: s.cfg.Terminal.SessionNamespace,
            Name:      podName,
        },
    }

    if err := s.runtimeClient.Delete(ctx, pod); err != nil && !kerrors.IsNotFound(err) {
        return ctrl.Result{}, errors.NewOperatorError(err, true, true)
    }

    log.Info().Str("pod", podName).Msg("deleted terminal pod from runtime cluster")
    return ctrl.Result{}, nil
}

func (s *PodSubroutine) buildTerminalPod(terminal *terminalv1alpha1.Terminal, name, namespace string) *corev1.Pod {
    return &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
            Labels: map[string]string{
                "app.kubernetes.io/name":                 "terminal",
                "app.kubernetes.io/managed-by":           "terminal-controller-manager",
                "terminal.platform-mesh.io/workspace":    terminal.Spec.Target.WorkspacePath,
            },
        },
        Spec: corev1.PodSpec{
            Containers: []corev1.Container{{
                Name:  "terminal",
                Image: s.cfg.Terminal.DefaultImage,
                Env: []corev1.EnvVar{
                    {Name: "KCP_API_SERVER", Value: terminal.Spec.Target.APIServerURL},
                    {Name: "KCP_WORKSPACE_PATH", Value: terminal.Spec.Target.WorkspacePath},
                },
                Stdin: true,
                TTY:   true,
            }},
            RestartPolicy: corev1.RestartPolicyNever,
        },
    }
}
```

### Runtime Client Setup (cmd/operator.go)

The runtime client is created separately from the multicluster manager:

```go
func RunController(_ *cobra.Command, _ []string) {
    // ... context setup ...

    // KCP config for multicluster manager (watches Terminal CRs across workspaces)
    kcpCfg := ctrl.GetConfigOrDie()

    // Runtime cluster config for pod management (in-cluster)
    runtimeCfg, err := rest.InClusterConfig()
    if err != nil {
        log.Fatal().Err(err).Msg("unable to get runtime cluster config")
    }

    // Create runtime client for pod operations
    runtimeClient, err := client.New(runtimeCfg, client.Options{Scheme: scheme})
    if err != nil {
        log.Fatal().Err(err).Msg("unable to create runtime client")
    }

    // Create KCP APIExport provider
    provider, err := apiexport.New(kcpCfg, operatorCfg.Kcp.ApiExportEndpointSliceName, apiexport.Options{...})

    // Create multicluster manager (connects to KCP only)
    mgr, err := mcmanager.New(kcpCfg, provider, mcmanager.Options{...})

    // Pass runtimeClient to subroutines that need to manage pods
    terminalReconciler := controller.NewTerminalReconciler(log, mgr, runtimeClient, operatorCfg)
    // ...
}
```

**Key Pattern:**
- `mcmanager` with `apiexport.Provider` → Watches Terminal CRs across KCP workspaces
- `client.New(rest.InClusterConfig())` → Manages pods on the runtime cluster where controller runs

### CRD Types with Conditions (api/v1alpha1/terminal_types.go)

```go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TerminalSpec defines the desired state of Terminal
type TerminalSpec struct {
    Target TerminalTarget `json:"target"`
    Host   TerminalHost   `json:"host"`
}

type TerminalTarget struct {
    WorkspacePath string `json:"workspacePath"`
    APIServerURL  string `json:"apiServerURL"`
    Namespace     string `json:"namespace,omitempty"`
}

type TerminalHost struct {
    Namespace          string      `json:"namespace"`
    TemporaryNamespace bool        `json:"temporaryNamespace,omitempty"`
    Pod                TerminalPod `json:"pod,omitempty"`
}

type TerminalPod struct {
    Image              string            `json:"image,omitempty"`
    Resources          corev1.Resources  `json:"resources,omitempty"`
    IdleTimeout        metav1.Duration   `json:"idleTimeout,omitempty"`
    MaxSessionDuration metav1.Duration   `json:"maxSessionDuration,omitempty"`
}

// TerminalStatus defines the observed state of Terminal
type TerminalStatus struct {
    Phase            TerminalPhase      `json:"phase,omitempty"`
    PodName          string             `json:"podName,omitempty"`
    LastActivityTime *metav1.Time       `json:"lastActivityTime,omitempty"`
    Conditions       []metav1.Condition `json:"conditions,omitempty"`
}

type TerminalPhase string

const (
    TerminalPhasePending     TerminalPhase = "Pending"
    TerminalPhaseCreating    TerminalPhase = "Creating"
    TerminalPhaseReady       TerminalPhase = "Ready"
    TerminalPhaseFailed      TerminalPhase = "Failed"
    TerminalPhaseTerminating TerminalPhase = "Terminating"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Pod",type=string,JSONPath=`.status.podName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type Terminal struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   TerminalSpec   `json:"spec,omitempty"`
    Status TerminalStatus `json:"status,omitempty"`
}

// GetConditions returns the status conditions - required by lifecycle manager
func (t *Terminal) GetConditions() []metav1.Condition {
    return t.Status.Conditions
}

// SetConditions sets the status conditions - required by lifecycle manager
func (t *Terminal) SetConditions(conditions []metav1.Condition) {
    t.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

type TerminalList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []Terminal `json:"items"`
}

func init() {
    SchemeBuilder.Register(&Terminal{}, &TerminalList{})
}
```

## Dependencies

| Dependency | Purpose |
|------------|---------|
| sigs.k8s.io/multicluster-runtime | Multi-cluster controller framework |
| github.com/kcp-dev/multicluster-provider | KCP APIExport provider |
| github.com/platform-mesh/golang-commons | Lifecycle, logging, config utilities |
| github.com/gorilla/websocket | WebSocket handling |
| github.com/spf13/cobra | CLI framework |
| github.com/spf13/viper | Configuration management |

## Open Questions

### 1. Where should terminal pods run?
**Options:**
- Same cluster as controller (simplest)
- Dedicated "terminal" cluster (isolation)
- User's workspace cluster (closest to resources)

**Recommendation:** Start with same cluster, allow configuration for dedicated cluster later.

### 2. Token expiry handling
**Problem:** OIDC tokens expire, long sessions may fail mid-command.

**Options:**
- Session ends on token expiry (simple, user reconnects)
- Frontend sends refresh token, controller signals pod to update kubeconfig
- Pod detects 401 errors and requests new token via controller

**Recommendation:** Start simple - session ends on token expiry. User reconnects with fresh token.

### 3. Multi-workspace support
**Question:** One terminal per workspace, or allow switching?

**Recommendation:** Start with one terminal per workspace. `kubectl ws` plugin allows switching workspaces within session if user's token has permissions.

### 4. WebSocket authentication
**Question:** How does controller verify the frontend is authorized to connect to a specific terminal?

**Options:**
- Terminal CR includes owner reference, controller validates
- Separate authorization check via OIDC token validation
- Trust based on terminal namespace RBAC

**Recommendation:** Controller validates that the OIDC token (sent as first message) is valid before proxying.

## Comparison with Gardener

| Aspect | Gardener | Platform-Mesh |
|--------|----------|---------------|
| Target | Shoots, Seeds, Garden | KCP Workspaces |
| Auth | ServiceAccount, ShootRef | OIDC tokens (frontend-injected) |
| Token Storage | Secrets in cluster | Never stored (tmpfs only) |
| WebSocket | Dashboard proxies | Controller proxies |
| Clusters | Multi-cluster (host/target) | Single or dedicated cluster |
| Frontend | Vue.js | Angular |
| Complexity | High (many cluster types) | Medium (KCP-focused) |

## References

- [Gardener terminal-controller-manager](https://github.com/gardener/terminal-controller-manager)
- [Gardener Dashboard](https://github.com/gardener/dashboard)
- [xterm.js](https://xtermjs.org/)
- [KCP](https://github.com/kcp-dev/kcp)
- [Kubernetes exec API](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#exec-options-v1-core)
- [gorilla/websocket](https://github.com/gorilla/websocket)
