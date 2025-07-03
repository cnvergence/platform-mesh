package testing

import (
	"context"
	"time"

	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/controller/testSupport"
	"github.com/platform-mesh/golang-commons/errors"
)

const SubroutineFinalizer = "finalizer"

type FinalizerSubroutine struct {
	Client       client.Client
	Err          error
	RequeueAfter time.Duration
}

func (c FinalizerSubroutine) Process(_ context.Context, runtimeObj runtimeobject.RuntimeObject) (controllerruntime.Result, errors.OperatorError) {
	instance := runtimeObj.(*testSupport.TestApiObject)
	instance.Status.Some = "other string"
	return controllerruntime.Result{}, nil
}

func (c FinalizerSubroutine) Finalize(_ context.Context, _ runtimeobject.RuntimeObject) (controllerruntime.Result, errors.OperatorError) {
	if c.Err != nil {
		return controllerruntime.Result{}, errors.NewOperatorError(c.Err, true, true)
	}
	if c.RequeueAfter > 0 {
		return controllerruntime.Result{RequeueAfter: c.RequeueAfter}, nil
	}

	return controllerruntime.Result{}, nil
}

func (c FinalizerSubroutine) GetName() string {
	return "changeStatus"
}

func (c FinalizerSubroutine) Finalizers() []string {
	return []string{
		SubroutineFinalizer,
	}
}
