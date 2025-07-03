package controllerruntime

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/platform-mesh/golang-commons/controller/filter"
	"github.com/platform-mesh/golang-commons/controller/lifecycle"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/conditions"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/spread"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/subroutine"
	"github.com/platform-mesh/golang-commons/logger"
)

type LifecycleManager struct {
	log                *logger.Logger
	client             client.Client
	config             lifecycle.Config
	subroutines        []subroutine.Subroutine
	spreader           *spread.Spreader
	conditionsManager  *conditions.ConditionManager
	prepareContextFunc lifecycle.PrepareContextFunc
}

func NewLifecycleManager(log *logger.Logger, operatorName string, controllerName string, client client.Client, subroutines []subroutine.Subroutine) *LifecycleManager {
	log = log.MustChildLoggerWithAttributes("operator", operatorName, "controller", controllerName)
	return &LifecycleManager{
		log:         log,
		client:      client,
		subroutines: subroutines,
		config: lifecycle.Config{
			OperatorName:   operatorName,
			ControllerName: controllerName,
		},
	}
}

func (l *LifecycleManager) Config() lifecycle.Config {
	return l.config
}
func (l *LifecycleManager) Log() *logger.Logger {
	return l.log
}
func (l *LifecycleManager) Subroutines() []subroutine.Subroutine {
	return l.subroutines
}
func (l *LifecycleManager) PrepareContextFunc() lifecycle.PrepareContextFunc {
	return l.prepareContextFunc
}
func (l *LifecycleManager) ConditionsManager() *conditions.ConditionManager {
	return l.conditionsManager
}

func (l *LifecycleManager) Spreader() *spread.Spreader {
	return l.spreader
}

func (l *LifecycleManager) Reconcile(ctx context.Context, req ctrl.Request, instance runtimeobject.RuntimeObject) (ctrl.Result, error) {
	return lifecycle.Reconcile(ctx, req, instance, l.client, l)
}

func (l *LifecycleManager) validateInterfaces(instance runtimeobject.RuntimeObject, log *logger.Logger) error {
	if l.Spreader() != nil {
		_, err := l.Spreader().ToRuntimeObjectSpreadReconcileStatusInterface(instance, log)
		if err != nil {
			return err
		}
	}
	if l.ConditionsManager() != nil {
		_, err := l.ConditionsManager().ToRuntimeObjectConditionsInterface(instance, log)
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *LifecycleManager) SetupWithManagerBuilder(mgr ctrl.Manager, maxReconciles int, reconcilerName string, instance runtimeobject.RuntimeObject, debugLabelValue string, log *logger.Logger, eventPredicates ...predicate.Predicate) (*builder.Builder, error) {
	if err := l.validateInterfaces(instance, log); err != nil {
		return nil, err
	}

	if (l.ConditionsManager() != nil || l.Spreader() != nil) && l.Config().ReadOnly {
		return nil, fmt.Errorf("cannot use conditions or spread reconciles in read-only mode")
	}

	eventPredicates = append([]predicate.Predicate{filter.DebugResourcesBehaviourPredicate(debugLabelValue)}, eventPredicates...)
	return ctrl.NewControllerManagedBy(mgr).
		Named(reconcilerName).
		For(instance).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxReconciles}).
		WithEventFilter(predicate.And(eventPredicates...)), nil
}

func (l *LifecycleManager) SetupWithManager(mgr ctrl.Manager, maxReconciles int, reconcilerName string, instance runtimeobject.RuntimeObject, debugLabelValue string, r reconcile.Reconciler, log *logger.Logger, eventPredicates ...predicate.Predicate) error {
	b, err := l.SetupWithManagerBuilder(mgr, maxReconciles, reconcilerName, instance, debugLabelValue, log, eventPredicates...)
	if err != nil {
		return err
	}

	return b.Complete(r)
}

// WithPrepareContextFunc allows to set a function that prepares the context before each reconciliation
// This can be used to add additional information to the context that is needed by the subroutines
// You need to return a new context and an OperatorError in case of an error
func (l *LifecycleManager) WithPrepareContextFunc(prepareFunction lifecycle.PrepareContextFunc) *LifecycleManager {
	l.prepareContextFunc = prepareFunction
	return l
}

// WithReadOnly allows to set the controller to read-only mode
// In read-only mode, the controller will not update the status of the instance
func (l *LifecycleManager) WithReadOnly() *LifecycleManager {
	l.config.ReadOnly = true
	return l
}

// WithSpreadingReconciles sets the LifecycleManager to spread out the reconciles
func (l *LifecycleManager) WithSpreadingReconciles() *LifecycleManager {
	l.spreader = spread.NewSpreader()
	return l
}

func (l *LifecycleManager) WithConditionManagement() *LifecycleManager {
	l.conditionsManager = conditions.NewConditionManager()
	return l
}
