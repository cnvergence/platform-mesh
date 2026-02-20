package testSupport

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/api"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/subroutine"
	"github.com/platform-mesh/golang-commons/logger"
)

type TestLifecycleManager struct {
	Logger             *logger.Logger
	SubroutinesArr     []subroutine.Subroutine
	spreader           api.SpreadManager
	conditionsManager  api.ConditionManager
	ShouldReconcile    bool
	prepareContextFunc api.PrepareContextFunc
	terminator         string
	initializer        string
}

func (l *TestLifecycleManager) Config() api.Config {
	return api.Config{
		ControllerName: "test-controller",
		OperatorName:   "test-operator",
		ReadOnly:       false,
	}
}
func (l *TestLifecycleManager) Log() *logger.Logger                     { return l.Logger }
func (l *TestLifecycleManager) Spreader() api.SpreadManager             { return l.spreader }
func (l *TestLifecycleManager) ConditionsManager() api.ConditionManager { return l.conditionsManager }
func (l *TestLifecycleManager) PrepareContextFunc() api.PrepareContextFunc {
	return l.prepareContextFunc
}
func (l *TestLifecycleManager) Subroutines() []subroutine.Subroutine { return l.SubroutinesArr }
func (l *TestLifecycleManager) Terminator() string                   { return l.terminator }
func (l *TestLifecycleManager) Initializer() string                  { return l.initializer }
func (l *TestLifecycleManager) WithTerminator(terminator string) *TestLifecycleManager {
	l.terminator = terminator
	return l
}
func (l *TestLifecycleManager) WithInitializer(initializer string) *TestLifecycleManager {
	l.initializer = initializer
	return l
}
func (l *TestLifecycleManager) WithSpreadingReconciles() api.Lifecycle {
	l.spreader = &TestSpreader{ShouldReconcile: l.ShouldReconcile}
	return l
}
func (l *TestLifecycleManager) WithConditionManagement() api.Lifecycle {
	l.conditionsManager = &TestConditionManager{}
	return l
}
func (l *TestLifecycleManager) WithPrepareContextFunc(prepareFunction api.PrepareContextFunc) *TestLifecycleManager {
	l.prepareContextFunc = prepareFunction
	return l
}

type TestSpreader struct {
	ShouldReconcile bool
}

func (t TestSpreader) ReconcileRequired(runtimeobject.RuntimeObject, *logger.Logger) bool {
	return t.ShouldReconcile
}

func (t TestSpreader) ToRuntimeObjectSpreadReconcileStatusInterface() (api.RuntimeObjectSpreadReconcileStatus, error) {
	//TODO implement me
	panic("implement me")
}

func (t TestSpreader) MustToRuntimeObjectSpreadReconcileStatusInterface() api.RuntimeObjectSpreadReconcileStatus {

	//TODO implement me
	panic("implement me")
}

func (t TestSpreader) OnNextReconcile(runtimeobject.RuntimeObject, *logger.Logger) (ctrl.Result, error) {
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

func (t TestSpreader) RemoveRefreshLabelIfExists(instance runtimeobject.RuntimeObject) bool {
	lbs := instance.GetLabels()
	if _, ok := lbs["platform-mesh.io/refresh-reconcile"]; ok {
		delete(lbs, "platform-mesh.io/refresh-reconcile")
		instance.SetLabels(lbs)
		return true
	}
	return false
}

func (t TestSpreader) SetNextReconcileTime(instanceStatusObj api.RuntimeObjectSpreadReconcileStatus, _ *logger.Logger) {
	instanceStatusObj.SetNextReconcileTime(metav1.NewTime(time.Now().Add(10 * time.Hour)))
}

func (t TestSpreader) UpdateObservedGeneration(instanceStatusObj api.RuntimeObjectSpreadReconcileStatus, _ *logger.Logger) {
	instanceStatusObj.SetObservedGeneration(instanceStatusObj.GetGeneration())
}

type TestConditionManager struct{}

func (t TestConditionManager) SetInstanceConditionUnknownIfNotSet(conditions *[]metav1.Condition) bool {
	return meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionUnknown,
		Message: "The resource is in an unknown state",
		Reason:  "Unknown",
	})
}

func (t TestConditionManager) SetSubroutineConditionToUnknownIfNotSet(conditions *[]metav1.Condition, subroutine subroutine.Subroutine, _ bool, _ *logger.Logger) bool {
	return meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    fmt.Sprintf("%s_Ready", subroutine.GetName()),
		Status:  metav1.ConditionUnknown,
		Message: "The resource is in an unknown state",
		Reason:  "Unknown",
	})
}

func (t TestConditionManager) SetSubroutineCondition(conditions *[]metav1.Condition, subroutine subroutine.Subroutine, _ ctrl.Result, _ error, _ bool, _ *logger.Logger) bool {
	return meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    fmt.Sprintf("%s_Ready", subroutine.GetName()),
		Status:  metav1.ConditionTrue,
		Message: "The subroutine is complete",
		Reason:  "ok",
	})
}

func (t TestConditionManager) SetInstanceConditionReady(conditions *[]metav1.Condition, _ metav1.ConditionStatus) bool {
	return meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Message: "The resource is ready",
		Reason:  "ok",
	})
}
