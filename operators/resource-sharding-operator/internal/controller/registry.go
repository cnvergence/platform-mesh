package controller

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type RunningController struct {
	Cancel   context.CancelFunc
	GVR      schema.GroupVersionResource
	LabelKey string
	Assigner *ShardAssigner
}

type DynamicControllerRegistry struct {
	mu      sync.Mutex
	running map[types.UID]*RunningController
}

func NewDynamicControllerRegistry() *DynamicControllerRegistry {
	return &DynamicControllerRegistry{
		running: make(map[types.UID]*RunningController),
	}
}

func (r *DynamicControllerRegistry) Get(uid types.UID) (*RunningController, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrl, ok := r.running[uid]
	return ctrl, ok
}

func (r *DynamicControllerRegistry) Register(uid types.UID, ctrl *RunningController) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running[uid] = ctrl
}

func (r *DynamicControllerRegistry) Deregister(uid types.UID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctrl, ok := r.running[uid]; ok {
		ctrl.Cancel()
		delete(r.running, uid)
	}
}

func (r *DynamicControllerRegistry) HasGVR(gvr schema.GroupVersionResource, excludeUID types.UID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for uid, ctrl := range r.running {
		if uid != excludeUID && ctrl.GVR == gvr {
			return true
		}
	}
	return false
}

// FindByGVR returns the first RunningController whose GVR matches all three fields
// (Group, Version, Resource). Returns nil if no match is found.
func (r *DynamicControllerRegistry) FindByGVR(group, version, resource string) *RunningController {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rc := range r.running {
		if rc.GVR.Group == group && rc.GVR.Version == version && rc.GVR.Resource == resource {
			return rc
		}
	}
	return nil
}
