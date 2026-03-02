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

package cmd

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/kcp-dev/multicluster-provider/apiexport"
	platformmeshcontext "github.com/platform-mesh/golang-commons/context"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/golang-commons/traces"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"github.com/platform-mesh/extension-manager-operator/internal/config"
	"github.com/platform-mesh/extension-manager-operator/internal/controller/controllerruntime"
	"github.com/platform-mesh/extension-manager-operator/internal/controller/multiclusterruntime"
)

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "operator to reconcile ContentConfiguration",
	Run:   RunController,
}

func RunController(_ *cobra.Command, _ []string) { // coverage-ignore
	log.Info().Msg("Starting operator")
	ctrl.SetLogger(log.ComponentLogger("controller-runtime").Logr())

	ctx, _, shutdown := platformmeshcontext.StartContext(log, operatorCfg, defaultCfg.ShutdownTimeout)
	defer shutdown()

	var err error
	var providerShutdown func(ctx context.Context) error
	if defaultCfg.Tracing.Enabled {
		providerShutdown, err = traces.InitProvider(ctx, defaultCfg.Tracing.Collector)
		if err != nil {
			log.Fatal().Err(err).Msg("unable to start gRPC-Sidecar TracerProvider")
		}
	} else {
		providerShutdown, err = traces.InitLocalProvider(ctx, defaultCfg.Tracing.Collector, false)
		if err != nil {
			log.Fatal().Err(err).Msg("unable to start local TracerProvider")
		}
	}

	defer func() {
		if err := providerShutdown(ctx); err != nil {
			log.Fatal().Err(err).Msg("failed to shutdown TracerProvider")
		}
	}()

	kubeconfigPath := os.Getenv("KUBECONFIG")
	var restCfg *rest.Config
	if kubeconfigPath != "" {
		var err error
		restCfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			log.Fatal().Err(err).Msg("unable to load kubeconfig from KUBECONFIG env var")
		}
	} else {
		log.Info().Msg("KUBECONFIG not set, using GetConfigOrDie()")
		restCfg = ctrl.GetConfigOrDie()
	}
	restCfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt)
	})

	if operatorCfg.KCPEnabled {
		log.Info().Msg("KCP mode enabled, initializing multicluster manager")
		// Leader election: same as account-operator and security-operator — use in-cluster config for the lease; Fatal if not in cluster.
		var leaderElectionCfg *rest.Config
		if defaultCfg.LeaderElectionEnabled {
			leaderElectionCfg, err = rest.InClusterConfig()
			if err != nil {
				log.Fatal().Err(err).Msg("unable to get in-cluster config for leader election")
			}
		}
		initializeMultiClusterManager(ctx, leaderElectionCfg, restCfg, log, *operatorCfg)
	} else {
		log.Info().Msg("KCP mode disabled, using standard controller-runtime manager")
		initializeControllerRuntimeManager(ctx, restCfg)
	}
}

func initializeMultiClusterManager(ctx context.Context, leaderElectionCfg *rest.Config, kcpCfg *rest.Config, log *logger.Logger, operatorCfg config.OperatorConfig) {
	log.Info().Msg("Initializing multicluster manager")
	kcpCfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt)
	})

	endpointSliceName := operatorCfg.KCPAPIExportEndpointSliceName
	provider, err := apiexport.New(kcpCfg, endpointSliceName, apiexport.Options{
		Scheme: scheme,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("unable to construct cluster provider")
	}
	log.Info().Str("endpointSliceName", endpointSliceName).Msg("KCP cluster provider created")

	mgr, err := mcmanager.New(kcpCfg, provider, manager.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: defaultCfg.Metrics.BindAddress,
			TLSOpts: []func(*tls.Config){
				func(c *tls.Config) { c.NextProtos = []string{"http/1.1"} },
			},
		},
		BaseContext:                   func() context.Context { return ctx },
		HealthProbeBindAddress:        defaultCfg.HealthProbeBindAddress,
		LeaderElection:                defaultCfg.LeaderElectionEnabled,
		LeaderElectionID:              "eengiex3.platform-mesh.io",
		LeaderElectionReleaseOnCancel: true,
		LeaderElectionConfig:          leaderElectionCfg,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("unable to set up overall controller manager")
	}

	contentConfigurationReconciler := multiclusterruntime.NewContentConfigurationReconciler(log, mgr, operatorCfg)
	if err := contentConfigurationReconciler.SetupWithManager(mgr, defaultCfg, log); err != nil {
		log.Fatal().Err(err).Str("controller", "ContentConfiguration").Msg("unable to create controller")
	}
	log.Info().Str("controller", "ContentConfiguration").Msg("ContentConfiguration controller registered with multicluster manager")

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up ready check")
	}

	log.Info().Msg("starting multicluster manager")
	startCtx := ctrl.SetupSignalHandler()
	if err := mgr.Start(startCtx); err != nil {
		log.Fatal().Err(err).Msg("problem running manager")
	}
}

func initializeControllerRuntimeManager(ctx context.Context, restCfg *rest.Config) {
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: defaultCfg.Metrics.BindAddress,
			TLSOpts: []func(*tls.Config){
				func(c *tls.Config) {
					log.Info().Msg("disabling http/2")
					c.NextProtos = []string{"http/1.1"}
				},
			},
		},
		BaseContext:                   func() context.Context { return ctx },
		HealthProbeBindAddress:        defaultCfg.HealthProbeBindAddress,
		LeaderElection:                defaultCfg.LeaderElectionEnabled,
		LeaderElectionID:              "eengiex4.platform-mesh.io",
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("unable to start manager")
	}

	contentConfigurationReconciler := controllerruntime.NewContentConfigurationReconcilerCR(log, mgr, *operatorCfg)
	if err := contentConfigurationReconciler.SetupWithManager(mgr, defaultCfg, log); err != nil {
		log.Fatal().Err(err).Str("controller", "ContentConfiguration").Msg("unable to create controller")
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up ready check")
	}

	log.Info().Msg("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatal().Err(err).Msg("problem running manager")
	}
}
