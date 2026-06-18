package subroutine

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/errors"
)

type Subroutine interface {
	Process(ctx context.Context, instance runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError)
	Finalize(ctx context.Context, instance runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError)
	GetName() string
	Finalizers(instance runtimeobject.RuntimeObject) []string
}

// Terminator can be implemeted to act as KCP terminator[0], i.e. act on
// reconciles for LogicalClusters with deletion timestamp but without finalizer.
// Use together with LifecycleManager.WithTerminator(string) to ensure removal
// of the terminator on successfull reconciliation.
//
// [0] https://docs.kcp.io/kcp/v0.30/concepts/workspaces/workspace-termination/
type Terminator interface {
	Terminate(ctx context.Context, instance runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError)
}

// Initializer can be implemented to act as KCP initializer[0], i.e. act on
// reconciles for LogicalClusters without deletion timestamp. Use together with
// LifecycleManager.WithInitializer(string) to ensure removal of the terminator
// on successfull reconciliation.
//
// [0]
// https://docs.kcp.io/kcp/v0.30/concepts/workspaces/workspace-initialization/
type Initializer interface {
	Initialize(ctx context.Context, instance runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError)
}
