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

package controller

import (
	"context"

	platformmeshconfig "github.com/platform-mesh/golang-commons/config"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/builder"
	mclifecycle "github.com/platform-mesh/golang-commons/controller/lifecycle/multicluster"
	lifecyclesubroutine "github.com/platform-mesh/golang-commons/controller/lifecycle/subroutine"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/terminal-controller-manager/api/v1alpha1"
	"github.com/platform-mesh/terminal-controller-manager/internal/config"
	"github.com/platform-mesh/terminal-controller-manager/pkg/subroutines"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	mccontext "sigs.k8s.io/multicluster-runtime/pkg/context"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

const (
	operatorName           = "terminal-controller-manager"
	terminalReconcilerName = "TerminalReconciler"
)

// TerminalReconciler orchestrates Terminal resources across logical clusters.
type TerminalReconciler struct {
	cfg       config.OperatorConfig
	lifecycle *mclifecycle.LifecycleManager
}

func NewTerminalReconciler(log *logger.Logger, mgr mcmanager.Manager, cfg config.OperatorConfig, runtimeClient client.Client) *TerminalReconciler { // coverage-ignore
	subs := []lifecyclesubroutine.Subroutine{}

	// Lifetime subroutine runs first to check for expired terminals
	if cfg.Subroutines.Lifetime.Enabled {
		subs = append(subs, subroutines.NewLifetimeSubroutine(mgr, cfg.Terminal.Lifetime))
	}

	if cfg.Subroutines.Pod.Enabled {
		subs = append(subs, subroutines.NewPodSubroutine(
			mgr,
			runtimeClient,
			cfg.Terminal.Image,
			cfg.Terminal.Namespace,
			cfg.Terminal.HostAliasIP,
			cfg.Terminal.HostAliasNames,
		))
	}

	if cfg.Subroutines.Service.Enabled {
		subs = append(subs, subroutines.NewServiceSubroutine(runtimeClient, cfg.Terminal.Namespace))
	}

	if cfg.Subroutines.HTTPRoute.Enabled {
		subs = append(subs, subroutines.NewHTTPRouteSubroutine(
			runtimeClient,
			cfg.Terminal.Namespace,
			cfg.Gateway.Name,
			cfg.Gateway.Namespace,
			cfg.Gateway.Hostnames,
		))
	}

	return &TerminalReconciler{
		cfg: cfg,
		lifecycle: builder.NewBuilder(operatorName, terminalReconcilerName, subs, log).
			WithConditionManagement().
			BuildMultiCluster(mgr),
	}
}

func (r *TerminalReconciler) SetupWithManager(mgr mcmanager.Manager, cfg *platformmeshconfig.CommonServiceConfig, log *logger.Logger, eventPredicates ...predicate.Predicate) error { // coverage-ignore
	return r.lifecycle.SetupWithManager(mgr, cfg.MaxConcurrentReconciles, terminalReconcilerName, &v1alpha1.Terminal{}, cfg.DebugLabelValue, r, log, eventPredicates...)
}

func (r *TerminalReconciler) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) { // coverage-ignore
	return r.lifecycle.Reconcile(mccontext.WithCluster(ctx, req.ClusterName), req, &v1alpha1.Terminal{})
}
