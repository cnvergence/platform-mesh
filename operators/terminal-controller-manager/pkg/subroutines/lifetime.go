/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package subroutines

import (
	"context"
	"time"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/errors"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/terminal-controller-manager/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	mccontext "sigs.k8s.io/multicluster-runtime/pkg/context"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	LifetimeSubroutineName = "LifetimeSubroutine"
)

// LifetimeSubroutine manages terminal lifetime and triggers deletion when expired
type LifetimeSubroutine struct {
	mgr      mcmanager.Manager
	lifetime time.Duration
}

func NewLifetimeSubroutine(mgr mcmanager.Manager, lifetime time.Duration) *LifetimeSubroutine {
	// Default to 2h if invalid
	if lifetime <= 0 {
		lifetime = 2 * time.Hour
	}

	return &LifetimeSubroutine{
		mgr:      mgr,
		lifetime: lifetime,
	}
}

func (r *LifetimeSubroutine) GetName() string {
	return LifetimeSubroutineName
}

func (r *LifetimeSubroutine) Finalizers(_ runtimeobject.RuntimeObject) []string {
	return nil // No finalizer needed - this subroutine only checks lifetime
}

func (r *LifetimeSubroutine) Finalize(_ context.Context, _ runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError) {
	// Nothing to clean up
	return ctrl.Result{}, nil
}

func (r *LifetimeSubroutine) Process(ctx context.Context, ro runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError) {
	instance := ro.(*v1alpha1.Terminal)
	log := logger.LoadLoggerFromContext(ctx)

	// Check if terminal has exceeded its lifetime
	terminalAge := time.Since(instance.CreationTimestamp.Time)
	if terminalAge > r.lifetime {
		log.Info().
			Str("terminalName", instance.Name).
			Dur("age", terminalAge).
			Dur("lifetime", r.lifetime).
			Msg("terminal exceeded lifetime, triggering deletion")

		// Delete the terminal CR - this will trigger finalization
		if instance.DeletionTimestamp == nil {
			clusterName, ok := mccontext.ClusterFrom(ctx)
			if !ok {
				return ctrl.Result{}, errors.NewOperatorError(
					errors.New("cluster name not found in context"), true, true)
			}
			cluster, err := r.mgr.GetCluster(ctx, clusterName)
			if err != nil {
				return ctrl.Result{}, errors.NewOperatorError(err, true, true)
			}
			if err := cluster.GetClient().Delete(ctx, instance); err != nil && !kerrors.IsNotFound(err) {
				return ctrl.Result{}, errors.NewOperatorError(err, true, true)
			}
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}
