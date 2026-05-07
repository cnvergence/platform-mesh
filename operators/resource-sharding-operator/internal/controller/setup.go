package controller

import (
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"k8s.io/client-go/discovery"
)

type SetupOptions struct {
	WebhookEnabled bool
}

func SetupWithManager(mgr ctrl.Manager, opts ...SetupOptions) error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	registry := NewDynamicControllerRegistry()

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
	serviceName := os.Getenv("WEBHOOK_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "resource-sharding-operator-webhook"
	}

	reconciler := &ResourceShardingReconciler{
		Client:             mgr.GetClient(),
		Discovery:          discoveryClient,
		Registry:           registry,
		Manager:            mgr,
		WebhookNamespace:   namespace,
		WebhookServiceName: serviceName,
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return err
	}

	webhookEnabled := len(opts) > 0 && opts[0].WebhookEnabled
	if webhookEnabled {
		webhookServer := mgr.GetWebhookServer()
		webhookServer.Register("/mutate-shard-assign", &webhook.Admission{
			Handler: &ShardAssignHandler{Registry: registry},
		})
	}

	return nil
}
