package cmd

import (
	"context"
	"crypto/tls"
	"net/http"

	platformmeshcontext "github.com/platform-mesh/golang-commons/context"
	"github.com/platform-mesh/resource-sharding-operator/internal/controller"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"github.com/kcp-dev/multicluster-provider/apiexport"

	"k8s.io/client-go/rest"
)

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "operator to manage resource sharding",
	Run:   RunController,
}

func RunController(_ *cobra.Command, _ []string) {
	var err error
	ctrl.SetLogger(log.ComponentLogger("controller-runtime").Logr())

	ctx, _, shutdown := platformmeshcontext.StartContext(log, operatorCfg, defaultCfg.ShutdownTimeout)
	defer shutdown()

	disableHTTP2 := func(c *tls.Config) {
		c.NextProtos = []string{"http/1.1"}
	}

	var tlsOpts []func(*tls.Config)
	if !defaultCfg.EnableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	restCfg := ctrl.GetConfigOrDie()
	restCfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt)
	})

	var leaderCfg *rest.Config
	if defaultCfg.LeaderElectionEnabled {
		leaderCfg, err = rest.InClusterConfig()
		if err != nil {
			log.Fatal().Err(err).Msg("unable to get in-cluster config")
		}
	}

	mgrOpts := manager.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   defaultCfg.Metrics.BindAddress,
			SecureServing: defaultCfg.Metrics.Secure,
			TLSOpts:       tlsOpts,
		},
		BaseContext:                   func() context.Context { return ctx },
		HealthProbeBindAddress:        defaultCfg.HealthProbeBindAddress,
		LeaderElection:                defaultCfg.LeaderElectionEnabled,
		LeaderElectionID:              "resource-sharding.platform-mesh.io",
		LeaderElectionConfig:          leaderCfg,
		LeaderElectionReleaseOnCancel: true,
	}

	var mgr ctrl.Manager
	if operatorCfg.Kcp.Enabled {
		provider, providerErr := apiexport.New(restCfg, operatorCfg.Kcp.ApiExportEndpointSliceName, apiexport.Options{
			Log:    &ctrl.Log,
			Scheme: scheme,
		})
		if providerErr != nil {
			log.Fatal().Err(providerErr).Msg("creating APIExport provider")
		}
		mcMgr, mcErr := mcmanager.New(restCfg, provider, mgrOpts)
		if mcErr != nil {
			log.Fatal().Err(mcErr).Msg("unable to start multicluster manager")
		}
		mgr = mcMgr.GetLocalManager()
	} else {
		mgr, err = ctrl.NewManager(restCfg, mgrOpts)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("unable to start manager")
	}

	setupOpts := controller.SetupOptions{WebhookEnabled: operatorCfg.WebhookEnabled}
	if err := controller.SetupWithManager(mgr, setupOpts); err != nil {
		log.Fatal().Err(err).Msg("unable to setup controllers")
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
