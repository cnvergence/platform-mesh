package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/platform-mesh/resource-sharding-operator/api/v1alpha1"
)

const (
	finalizerName = "sharding.platform-mesh.io/cleanup"

	ConditionReady              = "Ready"
	ConditionTargetNotFound     = "TargetNotFound"
	ConditionPermissionsMissing = "PermissionsMissing"
	ConditionConflict           = "Conflict"
)

type ResourceShardingReconciler struct {
	client.Client
	Discovery discovery.DiscoveryInterface
	Registry  *DynamicControllerRegistry
	Manager   ctrl.Manager
}

func (r *ResourceShardingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var rs v1alpha1.ResourceSharding
	if err := r.Get(ctx, req.NamespacedName, &rs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !rs.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &rs)
	}

	if !controllerutil.ContainsFinalizer(&rs, finalizerName) {
		controllerutil.AddFinalizer(&rs, finalizerName)
		if err := r.Update(ctx, &rs); err != nil {
			return ctrl.Result{}, err
		}
	}

	gvr := schema.GroupVersionResource{
		Group:    rs.Spec.Target.Group,
		Version:  rs.Spec.Target.Version,
		Resource: rs.Spec.Target.Resource,
	}

	if err := r.validateTarget(gvr); err != nil {
		meta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
			Type:               ConditionTargetNotFound,
			Status:             metav1.ConditionTrue,
			Reason:             "TargetGVRNotFound",
			Message:            fmt.Sprintf("Target resource %s not found: %v", gvr.String(), err),
			ObservedGeneration: rs.Generation,
		})
		meta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
			Type:               ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "TargetNotFound",
			Message:            "Target GVR does not exist in the cluster",
			ObservedGeneration: rs.Generation,
		})
		_ = r.Status().Update(ctx, &rs)
		return ctrl.Result{RequeueAfter: rs.Spec.Rebalance.Interval.Duration}, nil
	}
	meta.RemoveStatusCondition(&rs.Status.Conditions, ConditionTargetNotFound)

	if err := r.validateUniqueness(ctx, &rs, gvr); err != nil {
		meta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
			Type:               ConditionConflict,
			Status:             metav1.ConditionTrue,
			Reason:             "DuplicateTarget",
			Message:            err.Error(),
			ObservedGeneration: rs.Generation,
		})
		meta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
			Type:               ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "Conflict",
			Message:            "Another ResourceSharding targets the same GVR",
			ObservedGeneration: rs.Generation,
		})
		_ = r.Status().Update(ctx, &rs)
		return ctrl.Result{}, nil
	}
	meta.RemoveStatusCondition(&rs.Status.Conditions, ConditionConflict)

	if err := CheckTargetPermissions(ctx, r.Client, gvr); err != nil {
		meta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
			Type:               ConditionPermissionsMissing,
			Status:             metav1.ConditionTrue,
			Reason:             "InsufficientRBAC",
			Message:            fmt.Sprintf("Missing permissions on target: %v", err),
			ObservedGeneration: rs.Generation,
		})
		meta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
			Type:               ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "PermissionsMissing",
			Message:            "Operator lacks required permissions on target GVR",
			ObservedGeneration: rs.Generation,
		})
		_ = r.Status().Update(ctx, &rs)
		return ctrl.Result{RequeueAfter: rs.Spec.Rebalance.Interval.Duration}, nil
	}
	meta.RemoveStatusCondition(&rs.Status.Conditions, ConditionPermissionsMissing)

	if err := r.ensureDynamicController(ctx, &rs, gvr); err != nil {
		logger.Error(err, "failed to ensure dynamic controller")
		return ctrl.Result{}, err
	}

	// Resolve GVR → GVK for the rebalancer
	gvk, err := r.Manager.GetRESTMapper().KindFor(gvr)
	if err != nil {
		logger.Error(err, "failed to resolve GVK")
		return ctrl.Result{}, err
	}

	rebalancer := &Rebalancer{
		Client:   r.Client,
		LabelKey: rs.Spec.ShardLabelKey,
		GVK:      gvk,
		Shards:   shardNames(rs.Spec.Shards),
		Config:   rs.Spec.Rebalance,
	}

	result, err := rebalancer.Run(ctx)
	if err != nil {
		logger.Error(err, "rebalance failed")
		return ctrl.Result{RequeueAfter: rs.Spec.Rebalance.Interval.Duration}, nil
	}

	rs.Status.Distribution = result.Distribution
	rs.Status.TotalShards = len(rs.Spec.Shards)
	rs.Status.ObservedGeneration = rs.Generation
	if result.Moved > 0 {
		now := metav1.Now()
		rs.Status.LastRebalanceTime = &now
	}

	meta.SetStatusCondition(&rs.Status.Conditions, metav1.Condition{
		Type:               ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "ControllersRunning",
		Message:            fmt.Sprintf("Watching %s, %d shards configured", gvr.String(), len(rs.Spec.Shards)),
		ObservedGeneration: rs.Generation,
	})

	if err := r.Status().Update(ctx, &rs); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: rs.Spec.Rebalance.Interval.Duration}, nil
}

func (r *ResourceShardingReconciler) handleDeletion(ctx context.Context, rs *v1alpha1.ResourceSharding) (ctrl.Result, error) {
	r.Registry.Deregister(rs.UID)

	controllerutil.RemoveFinalizer(rs, finalizerName)
	if err := r.Update(ctx, rs); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ResourceShardingReconciler) validateTarget(gvr schema.GroupVersionResource) error {
	_, err := r.Discovery.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
	return err
}

func (r *ResourceShardingReconciler) validateUniqueness(ctx context.Context, rs *v1alpha1.ResourceSharding, gvr schema.GroupVersionResource) error {
	var list v1alpha1.ResourceShardingList
	if err := r.List(ctx, &list); err != nil {
		return err
	}
	for i := range list.Items {
		other := &list.Items[i]
		if other.UID == rs.UID {
			continue
		}
		otherGVR := schema.GroupVersionResource{
			Group:    other.Spec.Target.Group,
			Version:  other.Spec.Target.Version,
			Resource: other.Spec.Target.Resource,
		}
		if otherGVR == gvr {
			return fmt.Errorf("ResourceSharding %q already targets %s", other.Name, gvr.String())
		}
	}
	return nil
}

func (r *ResourceShardingReconciler) ensureDynamicController(ctx context.Context, rs *v1alpha1.ResourceSharding, gvr schema.GroupVersionResource) error {
	if _, exists := r.Registry.Get(rs.UID); exists {
		return nil
	}

	running, err := StartDynamicController(ctx, r.Manager, rs, gvr)
	if err != nil {
		return err
	}

	r.Registry.Register(rs.UID, running)
	return nil
}

func shardNames(refs []v1alpha1.ShardRef) []string {
	names := make([]string, len(refs))
	for i, ref := range refs {
		names[i] = ref.Name
	}
	return names
}

func (r *ResourceShardingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ResourceSharding{}).
		Complete(r)
}
