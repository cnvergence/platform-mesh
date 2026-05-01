package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/platform-mesh/resource-sharding-operator/api/v1alpha1"
)

type AssignmentReconciler struct {
	Client   client.Client
	Assigner *ShardAssigner
	LabelKey string
	GVK      schema.GroupVersionKind
}

func (r *AssignmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("assignment reconcile triggered", "resource", req.NamespacedName)

	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(r.GVK)

	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		logger.Info("get failed", "error", err)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if _, exists := obj.Labels[r.LabelKey]; exists {
		return ctrl.Result{}, nil
	}

	shard := r.Assigner.Next()
	patch := client.MergeFrom(obj.DeepCopy())
	if obj.Labels == nil {
		obj.Labels = make(map[string]string)
	}
	obj.Labels[r.LabelKey] = shard

	if err := r.Client.Patch(ctx, obj, patch); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("assigned shard", "resource", req.NamespacedName, "shard", shard)
	return ctrl.Result{}, nil
}

func StartDynamicController(ctx context.Context, mgr ctrl.Manager, rs *v1alpha1.ResourceSharding, gvr schema.GroupVersionResource) (*RunningController, error) {
	labelKey := rs.Spec.ShardLabelKey
	if labelKey == "" {
		labelKey = "sharding.platform-mesh.io/shard"
	}

	selector, err := labels.Parse("!" + labelKey)
	if err != nil {
		return nil, fmt.Errorf("parsing label selector: %w", err)
	}

	// Resolve GVR → GVK via RESTMapper (resource plural → Kind)
	mapper := mgr.GetRESTMapper()
	gvk, err := mapper.KindFor(gvr)
	if err != nil {
		return nil, fmt.Errorf("resolving GVR %s to GVK: %w", gvr.String(), err)
	}

	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(gvk)

	informerCache, err := cache.New(mgr.GetConfig(), cache.Options{
		Scheme: mgr.GetScheme(),
		ByObject: map[client.Object]cache.ByObject{
			obj: {Label: selector},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating cache: %w", err)
	}

	assigner := NewShardAssigner(shardNames(rs.Spec.Shards))

	c, err := controller.NewUnmanaged("shard-assign-"+rs.Name, controller.Options{
		Reconciler: &AssignmentReconciler{
			Client:   mgr.GetClient(),
			Assigner: assigner,
			LabelKey: labelKey,
			GVK:      gvk,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating controller: %w", err)
	}

	if err := c.Watch(source.Kind(informerCache, obj, &handler.TypedEnqueueRequestForObject[*metav1.PartialObjectMetadata]{})); err != nil {
		return nil, fmt.Errorf("setting up watch: %w", err)
	}

	ctrlCtx, cancel := context.WithCancel(ctx)
	logger := log.FromContext(ctx)

	// Start cache
	go func() {
		_ = informerCache.Start(ctrlCtx)
	}()

	if !informerCache.WaitForCacheSync(ctrlCtx) {
		cancel()
		return nil, fmt.Errorf("cache sync failed for %s", gvr.String())
	}
	logger.Info("dynamic controller cache synced", "gvr", gvr.String(), "gvk", obj.GroupVersionKind().String())

	// Start the controller
	go func() {
		logger.Info("starting dynamic assignment controller", "name", "shard-assign-"+rs.Name)
		if startErr := c.Start(ctrlCtx); startErr != nil {
			logger.Error(startErr, "dynamic controller exited with error", "name", "shard-assign-"+rs.Name)
		}
	}()

	// Process existing unlabeled resources from cache (initial backfill)
	go func() {
		// Give the controller a moment to start and register its source handler
		<-ctrlCtx.Done()
	}()
	go func() {
		list := &metav1.PartialObjectMetadataList{}
		list.SetGroupVersionKind(gvk)
		if listErr := informerCache.List(ctrlCtx, list); listErr != nil {
			logger.Error(listErr, "initial backfill list failed")
			return
		}
		logger.Info("initial backfill", "count", len(list.Items))
		for i := range list.Items {
			item := &list.Items[i]
			if _, exists := item.Labels[labelKey]; exists {
				continue
			}
			shard := assigner.Next()
			patch := client.MergeFrom(item.DeepCopy())
			if item.Labels == nil {
				item.Labels = make(map[string]string)
			}
			item.Labels[labelKey] = shard
			if patchErr := mgr.GetClient().Patch(ctrlCtx, item, patch); patchErr != nil {
				logger.Error(patchErr, "backfill patch failed", "resource", item.Name)
			}
		}
	}()

	return &RunningController{
		Cancel:   cancel,
		GVR:      gvr,
		Assigner: assigner,
	}, nil
}
