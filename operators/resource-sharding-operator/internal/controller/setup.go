package controller

import (
	"github.com/platform-mesh/golang-commons/logger"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
)

func SetupWithManager(mgr ctrl.Manager, log *logger.Logger) error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	registry := NewDynamicControllerRegistry()

	reconciler := &ResourceShardingReconciler{
		Client:    mgr.GetClient(),
		Discovery: discoveryClient,
		Registry:  registry,
		Manager:   mgr,
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return err
	}

	_ = log
	return nil
}
