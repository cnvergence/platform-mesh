package api

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/subroutine"
	"github.com/platform-mesh/golang-commons/errors"
	"github.com/platform-mesh/golang-commons/logger"
)

type Lifecycle interface {
	Config() Config
	Log() *logger.Logger
	Spreader() SpreadManager
	ConditionsManager() ConditionManager
	PrepareContextFunc() PrepareContextFunc
	Subroutines() []subroutine.Subroutine
}

type InitializingLifecycle interface {
	Initializer() string
}

type TerminatingLifecycle interface {
	Terminator() string
}

type PrepareContextFunc func(ctx context.Context, instance runtimeobject.RuntimeObject) (context.Context, errors.OperatorError)

type Config struct {
	OperatorName   string
	ControllerName string
	ReadOnly       bool
}

type ConditionManager interface {
	SetInstanceConditionUnknownIfNotSet(conditions *[]metav1.Condition) bool
	SetSubroutineConditionToUnknownIfNotSet(conditions *[]metav1.Condition, subroutine subroutine.Subroutine, isFinalize bool, log *logger.Logger) bool
	SetSubroutineCondition(conditions *[]metav1.Condition, subroutine subroutine.Subroutine, subroutineResult ctrl.Result, subroutineErr error, isFinalize bool, log *logger.Logger) bool
	SetInstanceConditionReady(conditions *[]metav1.Condition, status metav1.ConditionStatus) bool
}

type RuntimeObjectConditions interface {
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
}

type SpreadManager interface {
	ReconcileRequired(instance runtimeobject.RuntimeObject, log *logger.Logger) bool
	OnNextReconcile(instance runtimeobject.RuntimeObject, log *logger.Logger) (ctrl.Result, error)
	RemoveRefreshLabelIfExists(instance runtimeobject.RuntimeObject) bool
	SetNextReconcileTime(instanceStatusObj RuntimeObjectSpreadReconcileStatus, log *logger.Logger)
	UpdateObservedGeneration(instanceStatusObj RuntimeObjectSpreadReconcileStatus, log *logger.Logger)
}

type RuntimeObjectSpreadReconcileStatus interface {
	GetGeneration() int64
	GetObservedGeneration() int64
	SetObservedGeneration(int64)
	GetNextReconcileTime() metav1.Time
	SetNextReconcileTime(time metav1.Time)
}
