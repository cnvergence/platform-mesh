package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	toolscache "k8s.io/client-go/tools/cache"

	"github.com/platform-mesh/resource-sharding-operator/api/v1alpha1"
)

func StartDynamicController(ctx context.Context, mgr ctrl.Manager, rs *v1alpha1.ResourceSharding, gvr schema.GroupVersionResource) (*RunningController, error) {
	labelKey := rs.Spec.ShardLabelKey
	if labelKey == "" {
		labelKey = "sharding.platform-mesh.io/shard"
	}

	selector, err := labels.Parse("!" + labelKey)
	if err != nil {
		return nil, fmt.Errorf("parsing label selector: %w", err)
	}

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
	ctrlCtx, cancel := context.WithCancel(ctx)
	logger := log.FromContext(ctx)

	// Get the informer BEFORE starting the cache
	informer, err := informerCache.GetInformer(ctrlCtx, obj)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("getting informer: %w", err)
	}

	// Create a work queue
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	// Register event handler BEFORE cache starts — ensures initial LIST items are delivered
	_, err = informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			meta, ok := obj.(*metav1.PartialObjectMetadata)
			if !ok {
				return
			}
			queue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      meta.Name,
					Namespace: meta.Namespace,
				},
			})
		},
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("adding event handler: %w", err)
	}

	// Now start the cache — initial LIST will deliver to our registered handler
	go func() {
		_ = informerCache.Start(ctrlCtx)
	}()

	if !informerCache.WaitForCacheSync(ctrlCtx) {
		cancel()
		return nil, fmt.Errorf("cache sync failed for %s", gvr.String())
	}
	logger.Info("dynamic controller started", "gvr", gvr.String(), "gvk", gvk.String())

	// Process the work queue
	go func() {
		for {
			item, shutdown := queue.Get()
			if shutdown {
				return
			}

			req := item
			func() {
				defer queue.Done(req)

				patchObj := &metav1.PartialObjectMetadata{}
				patchObj.SetGroupVersionKind(gvk)

				if getErr := mgr.GetClient().Get(ctrlCtx, req.NamespacedName, patchObj); getErr != nil {
					queue.Forget(req)
					return
				}

				if _, exists := patchObj.Labels[labelKey]; exists {
					queue.Forget(req)
					return
				}

				shard := assigner.Next()
				patch := client.MergeFrom(patchObj.DeepCopy())
				if patchObj.Labels == nil {
					patchObj.Labels = make(map[string]string)
				}
				patchObj.Labels[labelKey] = shard

				if patchErr := mgr.GetClient().Patch(ctrlCtx, patchObj, patch); patchErr != nil {
					logger.Error(patchErr, "failed to assign shard", "resource", req.NamespacedName)
					queue.AddRateLimited(req)
					return
				}

				logger.Info("assigned shard", "resource", req.NamespacedName, "shard", shard)
				queue.Forget(req)
			}()
		}
	}()

	// Shutdown queue when context is cancelled
	go func() {
		<-ctrlCtx.Done()
		time.Sleep(100 * time.Millisecond)
		queue.ShutDown()
	}()

	return &RunningController{
		Cancel:   cancel,
		GVR:      gvr,
		Assigner: assigner,
	}, nil
}
