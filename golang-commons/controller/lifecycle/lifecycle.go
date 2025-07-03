package lifecycle

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/conditions"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/spread"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/subroutine"
	"github.com/platform-mesh/golang-commons/errors"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/golang-commons/sentry"
)

type Lifecycle interface {
	Config() Config
	Log() *logger.Logger
	Spreader() *spread.Spreader
	ConditionsManager() *conditions.ConditionManager
	PrepareContextFunc() PrepareContextFunc
	Subroutines() []subroutine.Subroutine
}

type Config struct {
	OperatorName   string
	ControllerName string
	ReadOnly       bool
}

type PrepareContextFunc func(ctx context.Context, instance runtimeobject.RuntimeObject) (context.Context, errors.OperatorError)

func Reconcile(ctx context.Context, req ctrl.Request, instance runtimeobject.RuntimeObject, cl client.Client, l Lifecycle) (ctrl.Result, error) {
	ctx, span := otel.Tracer(l.Config().OperatorName).Start(ctx, fmt.Sprintf("%s.Reconcile", l.Config().ControllerName))
	defer span.End()

	result := ctrl.Result{}
	reconcileId := uuid.New().String()

	log := l.Log().MustChildLoggerWithAttributes("name", req.Name, "namespace", req.Namespace, "reconcile_id", reconcileId)
	sentryTags := sentry.Tags{"namespace": req.Namespace, "name": req.Name}

	ctx = logger.SetLoggerInContext(ctx, log)
	ctx = sentry.ContextWithSentryTags(ctx, sentryTags)

	log.Info().Msg("start reconcile")
	err := cl.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if kerrors.IsNotFound(err) {
			log.Info().Msg("instance not found. It was likely deleted")
			return ctrl.Result{}, nil
		}
		return HandleClientError("failed to retrieve instance", log, err, true, sentryTags)
	}

	originalCopy := instance.DeepCopyObject()
	inDeletion := instance.GetDeletionTimestamp() != nil
	generationChanged := true

	if l.Spreader() != nil && instance.GetDeletionTimestamp().IsZero() {
		instanceStatusObj := l.Spreader().MustToRuntimeObjectSpreadReconcileStatusInterface(instance, log)
		generationChanged = instance.GetGeneration() != instanceStatusObj.GetObservedGeneration()
		isAfterNextReconcileTime := v1.Now().UTC().After(instanceStatusObj.GetNextReconcileTime().UTC())
		refreshRequested := slices.Contains(maps.Keys(instance.GetLabels()), spread.ReconcileRefreshLabel)

		reconcileRequired := generationChanged || isAfterNextReconcileTime || refreshRequested
		if !reconcileRequired {
			log.Info().Msg("skipping reconciliation, spread reconcile is active. No processing needed")
			return l.Spreader().OnNextReconcile(instanceStatusObj, log)
		}
	}

	// Manage Finalizers
	ferr := AddFinalizersIfNeeded(ctx, cl, instance, l.Subroutines(), l.Config().ReadOnly)
	if ferr != nil {
		return ctrl.Result{}, ferr
	}

	var condArr []v1.Condition
	if l.ConditionsManager() != nil {
		condArr = l.ConditionsManager().MustToRuntimeObjectConditionsInterface(instance, log).GetConditions()
		l.ConditionsManager().SetInstanceConditionUnknownIfNotSet(&condArr)
	}

	if l.PrepareContextFunc() != nil {
		localCtx, oErr := l.PrepareContextFunc()(ctx, instance)
		if oErr != nil {
			return HandleOperatorError(ctx, oErr, "failed to prepare context", generationChanged, l.Log())
		}
		ctx = localCtx
	}

	// In case of deletion execute the finalize subroutines in the reverse order as subroutine processing
	subroutines := make([]subroutine.Subroutine, len(l.Subroutines()))
	copy(subroutines, l.Subroutines())
	if inDeletion {
		slices.Reverse(subroutines)
	}

	// Continue with reconciliation
	for _, s := range subroutines {
		if l.ConditionsManager() != nil {
			l.ConditionsManager().SetSubroutineConditionToUnknownIfNotSet(&condArr, s, inDeletion, log)
		}

		// Set current condArr before reconciling the s
		if l.ConditionsManager() != nil {
			l.ConditionsManager().MustToRuntimeObjectConditionsInterface(instance, log).SetConditions(condArr)
		}
		subResult, retry, err := reconcileSubroutine(ctx, instance, s, cl, l, log, generationChanged, sentryTags)
		// Update condArr with any changes the s did
		if l.ConditionsManager() != nil {
			condArr = l.ConditionsManager().MustToRuntimeObjectConditionsInterface(instance, log).GetConditions()
		}
		if err != nil {
			if l.ConditionsManager() != nil {
				l.ConditionsManager().SetSubroutineCondition(&condArr, s, result, err, inDeletion, log)
				l.ConditionsManager().SetInstanceConditionReady(&condArr, v1.ConditionFalse)
				l.ConditionsManager().MustToRuntimeObjectConditionsInterface(instance, log).SetConditions(condArr)
			}
			if !retry {
				MarkResourceAsFinal(instance, log, condArr, v1.ConditionFalse, l)
			}
			if !l.Config().ReadOnly {
				_ = updateStatus(ctx, cl, originalCopy, instance, log, generationChanged, sentryTags)
			}
			if !retry {
				return ctrl.Result{}, nil
			}
			return subResult, err
		}
		if subResult.RequeueAfter > 0 {
			if subResult.RequeueAfter < result.RequeueAfter || result.RequeueAfter == 0 {
				result.RequeueAfter = subResult.RequeueAfter
			}
		}
		if l.ConditionsManager() != nil {
			if subResult.RequeueAfter == 0 {
				l.ConditionsManager().SetSubroutineCondition(&condArr, s, subResult, err, inDeletion, log)
			}
		}
	}

	if result.RequeueAfter == 0 {
		// Reconciliation was successful
		MarkResourceAsFinal(instance, log, condArr, v1.ConditionTrue, l)
	} else {
		if l.ConditionsManager() != nil {
			l.ConditionsManager().SetInstanceConditionReady(&condArr, v1.ConditionFalse)
		}
	}

	if l.ConditionsManager() != nil {
		l.ConditionsManager().MustToRuntimeObjectConditionsInterface(instance, log).SetConditions(condArr)
	}

	if !l.Config().ReadOnly {
		err = updateStatus(ctx, cl, originalCopy, instance, log, generationChanged, sentryTags)
		if err != nil {
			return result, err
		}
	}

	if l.Spreader() != nil && instance.GetDeletionTimestamp().IsZero() {
		original := instance.DeepCopyObject().(client.Object)
		removed := l.Spreader().RemoveRefreshLabelIfExists(instance)
		if removed {
			updateErr := cl.Patch(ctx, instance, client.MergeFrom(original))
			if updateErr != nil {
				return HandleClientError("failed to update instance", log, err, generationChanged, sentryTags)
			}
		}
	}

	log.Info().Msg("end reconcile")
	return result, nil
}

func reconcileSubroutine(ctx context.Context, instance runtimeobject.RuntimeObject, subroutine subroutine.Subroutine, cl client.Client, l Lifecycle, log *logger.Logger, generationChanged bool, sentryTags map[string]string) (ctrl.Result, bool, error) {
	subroutineLogger := log.ChildLogger("subroutine", subroutine.GetName())
	ctx = logger.SetLoggerInContext(ctx, subroutineLogger)
	subroutineLogger.Debug().Msg("start subroutine")

	ctx, span := otel.Tracer(l.Config().OperatorName).Start(ctx, fmt.Sprintf("%s.reconcileSubroutine.%s", l.Config().ControllerName, subroutine.GetName()))
	defer span.End()
	var result ctrl.Result
	var err errors.OperatorError
	if instance.GetDeletionTimestamp() != nil {
		if containsFinalizer(instance, subroutine.Finalizers()) {
			subroutineLogger.Debug().Msg("finalizing instance")
			result, err = subroutine.Finalize(ctx, instance)
			subroutineLogger.Debug().Any("result", result).Msg("finalized instance")
			if err == nil {
				// Remove finalizers unless requeue is requested
				err = removeFinalizerIfNeeded(ctx, instance, subroutine, result, l.Config().ReadOnly, cl)
			}
		}
	} else {
		subroutineLogger.Debug().Msg("processing instance")
		result, err = subroutine.Process(ctx, instance)
		subroutineLogger.Debug().Any("result", result).Msg("processed instance")
	}

	if err != nil {
		if generationChanged && err.Sentry() {
			sentry.CaptureError(err.Err(), sentryTags)
		}
		subroutineLogger.Error().Err(err.Err()).Bool("retry", err.Retry()).Msg("subroutine ended with error")
		return result, err.Retry(), err.Err()
	}

	subroutineLogger.Debug().Msg("end subroutine")
	return result, false, nil
}

func containsFinalizer(o client.Object, subroutineFinalizers []string) bool {
	for _, subroutineFinalizer := range subroutineFinalizers {
		if controllerutil.ContainsFinalizer(o, subroutineFinalizer) {
			return true
		}
	}
	return false
}

func removeFinalizerIfNeeded(ctx context.Context, instance runtimeobject.RuntimeObject, subroutine subroutine.Subroutine, result ctrl.Result, readonly bool, cl client.Client) errors.OperatorError {
	if readonly {
		return nil
	}

	if result.RequeueAfter == 0 {
		update := false
		original := instance.DeepCopyObject().(client.Object)
		for _, f := range subroutine.Finalizers() {
			needsUpdate := controllerutil.RemoveFinalizer(instance, f)
			if needsUpdate {
				update = true
			}
		}
		if update {
			err := cl.Patch(ctx, instance, client.MergeFrom(original))
			if err != nil {
				return errors.NewOperatorError(errors.Wrap(err, "failed to update instance"), true, false)
			}
		}
	}

	return nil
}

func updateStatus(ctx context.Context, cl client.Client, original runtime.Object, current runtimeobject.RuntimeObject, log *logger.Logger, generationChanged bool, sentryTags sentry.Tags) error {
	currentUn, err := runtime.DefaultUnstructuredConverter.ToUnstructured(current)
	if err != nil {
		return err
	}

	originalUn, err := runtime.DefaultUnstructuredConverter.ToUnstructured(original)
	if err != nil {
		return err
	}

	currentStatus, hasField, err := unstructured.NestedFieldCopy(currentUn, "status")
	if err != nil {
		return err
	}
	if !hasField {
		return fmt.Errorf("status field not found in current object")
	}

	originalStatus, hasField, err := unstructured.NestedFieldCopy(originalUn, "status")
	if err != nil {
		return err
	}
	if !hasField {
		return fmt.Errorf("status field not found in current object")
	}

	if equality.Semantic.DeepEqual(currentStatus, originalStatus) {
		log.Info().Msg("skipping status update, since they are equal")
		return nil
	}

	log.Info().Msg("updating resource status")
	err = cl.Status().Update(ctx, current)
	if err != nil {
		if !kerrors.IsConflict(err) {
			log.Error().Err(err).Msg("cannot update status, kubernetes client error")
			if generationChanged {
				sentry.CaptureError(err, sentryTags, sentry.Extras{"message": "Updating of instance status failed"})
			}
		}
		log.Error().Err(err).Msg("cannot update reconciliation Conditions, kubernetes client error")
		return err
	}

	return nil
}

func HandleClientError(msg string, log *logger.Logger, err error, generationChanged bool, sentryTags sentry.Tags) (ctrl.Result, error) {
	log.Error().Err(err).Msg(msg)
	if generationChanged {
		sentry.CaptureError(err, sentryTags)
	}

	return ctrl.Result{}, err
}

func MarkResourceAsFinal(instance runtimeobject.RuntimeObject, log *logger.Logger, conditions []v1.Condition, status v1.ConditionStatus, l Lifecycle) {
	if l.Spreader() != nil && instance.GetDeletionTimestamp().IsZero() {
		instanceStatusObj := l.Spreader().MustToRuntimeObjectSpreadReconcileStatusInterface(instance, log)
		l.Spreader().SetNextReconcileTime(instanceStatusObj, log)
		l.Spreader().UpdateObservedGeneration(instanceStatusObj, log)
	}

	if l.ConditionsManager() != nil {
		l.ConditionsManager().SetInstanceConditionReady(&conditions, status)
	}
}

func AddFinalizersIfNeeded(ctx context.Context, cl client.Client, instance runtimeobject.RuntimeObject, subroutines []subroutine.Subroutine, readonly bool) error {
	if readonly {
		return nil
	}

	if !instance.GetDeletionTimestamp().IsZero() {
		return nil
	}

	update := false
	original := instance.DeepCopyObject().(client.Object)
	for _, s := range subroutines {
		if len(s.Finalizers()) > 0 {
			needsUpdate := AddFinalizerIfNeeded(instance, s)
			if needsUpdate {
				update = true
			}
		}
	}
	if update {
		err := cl.Patch(ctx, instance, client.MergeFrom(original))
		if err != nil {
			return err
		}
	}
	return nil
}

func AddFinalizerIfNeeded(instance runtimeobject.RuntimeObject, subroutine subroutine.Subroutine) bool {
	update := false
	for _, f := range subroutine.Finalizers() {
		needsUpdate := controllerutil.AddFinalizer(instance, f)
		if needsUpdate {
			update = true
		}
	}
	return update
}

func HandleOperatorError(ctx context.Context, operatorError errors.OperatorError, msg string, generationChanged bool, log *logger.Logger) (ctrl.Result, error) {
	log.Error().Bool("retry", operatorError.Retry()).Bool("sentry", operatorError.Sentry()).Err(operatorError.Err()).Msg(msg)
	if generationChanged && operatorError.Sentry() {
		sentry.CaptureError(operatorError.Err(), sentry.GetSentryTagsFromContext(ctx))
	}

	if operatorError.Retry() {
		return ctrl.Result{}, operatorError.Err()
	}

	return ctrl.Result{}, nil
}
