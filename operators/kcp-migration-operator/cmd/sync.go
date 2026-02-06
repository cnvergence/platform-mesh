package cmd

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/yaml"

	"github.com/platform-mesh/kcp-migration-operator/internal/config"
	"github.com/platform-mesh/kcp-migration-operator/internal/controller"
	"github.com/platform-mesh/kcp-migration-operator/internal/kcp"
)

var syncCfg config.SyncConfig

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Run in sync mode to migrate resources to KCP",
	Long: `Sync mode watches source resources and synchronizes them to KCP workspaces.

There are two modes of operation:

1. Single-resource mode (via flags):
   Sync a single resource type using command-line flags.

2. Multi-resource mode (via config file):
   Sync multiple resource types using a YAML configuration file.
   Use --config to specify the path to the config file.

Example (single-resource):
  kcp-migration-operator sync \
    --migration-name=my-migration \
    --source-api-version=fabric.foundation.sap.com/v1alpha1 \
    --source-kind=Account \
    --source-namespace=account-2bzns \
    --target-workspace-expression="root:orgs:sap" \
    --template-path=.templates/account.yaml

Example (multi-resource):
  kcp-migration-operator sync --config=config/sync-config.yaml`,
	Run: RunSync,
}

func init() {
	// Bind sync-specific flags
	syncCmd.Flags().String("config", "", "Path to YAML config file for multi-resource sync")
	syncCmd.Flags().String("migration-name", "", "Name of the KCPMigration resource")
	syncCmd.Flags().String("migration-namespace", "", "Namespace of the KCPMigration resource")
	syncCmd.Flags().String("source-api-version", "", "API version of source resources to watch")
	syncCmd.Flags().String("source-kind", "", "Kind of source resources to watch")
	syncCmd.Flags().String("source-namespace", "", "Namespace to watch for source resources (empty = all namespaces)")
	syncCmd.Flags().StringSlice("source-label-selectors", []string{}, "Label selectors to filter source resources (e.g., 'app=myapp,env=prod')")
	syncCmd.Flags().String("target-workspace-expression", "", "Go template for target workspace")
	syncCmd.Flags().String("target-namespace", "", "Target namespace in KCP workspace")
	syncCmd.Flags().String("template", "", "Inline transformation template")
	syncCmd.Flags().String("template-path", "", "Path to template file (useful for local development)")
	syncCmd.Flags().String("template-configmap-name", "", "ConfigMap containing template")
	syncCmd.Flags().String("template-configmap-key", "template.yaml", "Key in ConfigMap for template")
	syncCmd.Flags().String("kcp-kubeconfig-path", "/etc/kcp/kubeconfig", "Path to KCP kubeconfig")
	syncCmd.Flags().String("source-kubeconfig-path", "", "Path to source cluster kubeconfig")
	syncCmd.Flags().Int("rate-limit-resources-per-second", 50, "Max resources to sync per second")
	syncCmd.Flags().Int("rate-limit-burst", 100, "Rate limiter burst size")
	syncCmd.Flags().Int("max-workers", 1, "Max concurrent workers for reconciliation queue")

	_ = v.BindPFlags(syncCmd.Flags())
}

func RunSync(_ *cobra.Command, _ []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info().Msg("Received shutdown signal")
		cancel()
	}()

	// Check if config file is specified for multi-resource mode
	configPath := v.GetString("config")
	if configPath != "" {
		runMultiSync(ctx, configPath)
		return
	}

	// Single-resource mode via flags
	runSingleSync(ctx)
}

// runSingleSync runs the sync controller for a single resource type using CLI flags
func runSingleSync(ctx context.Context) {
	// Unmarshal sync config
	if err := v.Unmarshal(&syncCfg); err != nil {
		log.Fatal().Err(err).Msg("failed to unmarshal sync config")
	}

	// Load template from file if template-path is specified
	if syncCfg.Transform.TemplatePath != "" {
		templateBytes, err := os.ReadFile(syncCfg.Transform.TemplatePath)
		if err != nil {
			log.Fatal().Err(err).Str("path", syncCfg.Transform.TemplatePath).Msg("failed to read template file")
		}
		syncCfg.Transform.Template = string(templateBytes)
		log.Info().Str("path", syncCfg.Transform.TemplatePath).Msg("loaded template from file")
	}

	// Load template from ConfigMap if configmap-name is specified
	if syncCfg.Transform.ConfigMapName != "" && syncCfg.Transform.Template == "" {
		log.Info().
			Str("configMapName", syncCfg.Transform.ConfigMapName).
			Str("configMapKey", syncCfg.Transform.ConfigMapKey).
			Msg("loading template from ConfigMap")

		// Create a temporary client to read the ConfigMap
		inClusterConfig := ctrl.GetConfigOrDie()
		tmpClient, err := client.New(inClusterConfig, client.Options{Scheme: scheme})
		if err != nil {
			log.Fatal().Err(err).Msg("failed to create client for ConfigMap lookup")
		}

		cm := &corev1.ConfigMap{}
		cmKey := syncCfg.Transform.ConfigMapKey
		if cmKey == "" {
			cmKey = "template.yaml"
		}

		// Get namespace from environment or default to platform-mesh-system
		namespace := os.Getenv("POD_NAMESPACE")
		if namespace == "" {
			namespace = "platform-mesh-system"
		}

		if err := tmpClient.Get(context.Background(), client.ObjectKey{
			Namespace: namespace,
			Name:      syncCfg.Transform.ConfigMapName,
		}, cm); err != nil {
			log.Fatal().Err(err).
				Str("configMapName", syncCfg.Transform.ConfigMapName).
				Str("namespace", namespace).
				Msg("failed to get ConfigMap")
		}

		template, ok := cm.Data[cmKey]
		if !ok {
			log.Fatal().
				Str("configMapName", syncCfg.Transform.ConfigMapName).
				Str("key", cmKey).
				Msg("template key not found in ConfigMap")
		}

		syncCfg.Transform.Template = template
		log.Info().
			Str("configMapName", syncCfg.Transform.ConfigMapName).
			Str("key", cmKey).
			Msg("loaded template from ConfigMap")
	}

	log.Info().
		Str("migrationName", syncCfg.MigrationName).
		Str("migrationNamespace", syncCfg.MigrationNamespace).
		Str("sourceAPIVersion", syncCfg.Source.APIVersion).
		Str("sourceKind", syncCfg.Source.Kind).
		Str("sourceNamespace", syncCfg.Source.Namespace).
		Strs("sourceLabelSelectors", syncCfg.Source.LabelSelectors).
		Int("maxWorkers", syncCfg.Performance.MaxWorkers).
		Msg("starting sync controller (single-resource mode)")

	// Validate required config
	if syncCfg.MigrationName == "" || syncCfg.Source.APIVersion == "" || syncCfg.Source.Kind == "" {
		log.Fatal().Msg("migration-name, source-api-version, and source-kind are required")
	}

	// Get REST config - use source kubeconfig if provided, otherwise use in-cluster config
	var restConfig *rest.Config
	var err error
	if syncCfg.SourceKubeconfigPath != "" {
		restConfig, err = clientcmd.BuildConfigFromFlags("", syncCfg.SourceKubeconfigPath)
		if err != nil {
			log.Fatal().Err(err).Str("path", syncCfg.SourceKubeconfigPath).Msg("failed to load source kubeconfig")
		}
		log.Info().Str("path", syncCfg.SourceKubeconfigPath).Msg("using source kubeconfig for watching resources")
	} else {
		restConfig = ctrl.GetConfigOrDie()
		log.Info().Msg("using in-cluster config for watching resources")
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: ":9090",
		},
		HealthProbeBindAddress: ":8090",
		Logger:                 log.ComponentLogger("sync-controller-runtime").Logr(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("unable to create manager")
	}

	// Create workspace client factory if KCP kubeconfig is configured
	var workspaceFactory kcp.WorkspaceClientFactory
	if syncCfg.KCPKubeconfigPath != "" {
		var err error
		workspaceFactory, err = kcp.NewWorkspaceClientFactory(syncCfg.KCPKubeconfigPath, scheme)
		if err != nil {
			log.Fatal().Err(err).Str("path", syncCfg.KCPKubeconfigPath).Msg("failed to create KCP workspace client factory")
		}
		log.Info().Str("path", syncCfg.KCPKubeconfigPath).Msg("KCP workspace client factory created")
	} else {
		log.Warn().Msg("no KCP kubeconfig path configured, sync to KCP will be disabled")
	}

	syncController := controller.NewSyncController(
		mgr.GetClient(),
		log,
		&syncCfg,
		workspaceFactory,
	)

	if err := syncController.SetupWithManager(mgr); err != nil {
		log.Fatal().Err(err).Msg("unable to setup sync controller")
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up ready check")
	}

	log.Info().Msg("starting sync manager")
	if err := mgr.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("problem running sync manager")
	}
}

// runMultiSync runs multiple sync controllers based on a YAML configuration file
func runMultiSync(ctx context.Context, configPath string) {
	log.Info().Str("config", configPath).Msg("starting sync controller (multi-resource mode)")

	// Read config file
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", configPath).Msg("failed to read config file")
	}

	// Parse config
	var multiCfg config.MultiSyncConfig
	if err := yaml.Unmarshal(configBytes, &multiCfg); err != nil {
		log.Fatal().Err(err).Msg("failed to parse config file")
	}

	// Validate config
	if len(multiCfg.Resources) == 0 {
		log.Fatal().Msg("no resources configured in config file")
	}

	log.Info().
		Str("kcpKubeconfigPath", multiCfg.KCPKubeconfigPath).
		Str("templatesDir", multiCfg.TemplatesDir).
		Int("resourceCount", len(multiCfg.Resources)).
		Msg("loaded multi-sync config")

	// Get REST config - use source kubeconfig if provided, otherwise use in-cluster config
	var restConfig *rest.Config
	if multiCfg.SourceKubeconfigPath != "" {
		var err error
		restConfig, err = clientcmd.BuildConfigFromFlags("", multiCfg.SourceKubeconfigPath)
		if err != nil {
			log.Fatal().Err(err).Str("path", multiCfg.SourceKubeconfigPath).Msg("failed to load source kubeconfig")
		}
		log.Info().Str("path", multiCfg.SourceKubeconfigPath).Msg("using source kubeconfig for watching resources")
	} else {
		restConfig = ctrl.GetConfigOrDie()
		log.Info().Msg("using in-cluster config for watching resources")
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: ":9090",
		},
		HealthProbeBindAddress: ":8090",
		Logger:                 log.ComponentLogger("sync-controller-runtime").Logr(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("unable to create manager")
	}

	// Create workspace client factory if KCP kubeconfig is configured
	var workspaceFactory kcp.WorkspaceClientFactory
	if multiCfg.KCPKubeconfigPath != "" {
		var err error
		workspaceFactory, err = kcp.NewWorkspaceClientFactory(multiCfg.KCPKubeconfigPath, scheme)
		if err != nil {
			log.Fatal().Err(err).Str("path", multiCfg.KCPKubeconfigPath).Msg("failed to create KCP workspace client factory")
		}
		log.Info().Str("path", multiCfg.KCPKubeconfigPath).Msg("KCP workspace client factory created")
	} else {
		log.Warn().Msg("no KCP kubeconfig path configured, sync to KCP will be disabled")
	}

	// Setup a sync controller for each resource configuration
	for _, resCfg := range multiCfg.Resources {
		// Convert ResourceSyncConfig to SyncConfig
		cfg := resourceSyncConfigToSyncConfig(resCfg, multiCfg.TemplatesDir)

		log.Info().
			Str("name", resCfg.Name).
			Str("sourceAPIVersion", cfg.Source.APIVersion).
			Str("sourceKind", cfg.Source.Kind).
			Str("sourceNamespace", cfg.Source.Namespace).
			Strs("sourceLabelSelectors", cfg.Source.LabelSelectors).
			Int("maxWorkers", cfg.Performance.MaxWorkers).
			Msg("setting up sync controller for resource")

		syncController := controller.NewSyncController(
			mgr.GetClient(),
			log,
			cfg,
			workspaceFactory,
		)

		if err := syncController.SetupWithManager(mgr); err != nil {
			log.Fatal().Err(err).Str("name", resCfg.Name).Msg("unable to setup sync controller")
		}
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Fatal().Err(err).Msg("unable to set up ready check")
	}

	log.Info().Int("resourceCount", len(multiCfg.Resources)).Msg("starting sync manager with multiple resources")
	if err := mgr.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("problem running sync manager")
	}
}

// resourceSyncConfigToSyncConfig converts a ResourceSyncConfig to a SyncConfig
func resourceSyncConfigToSyncConfig(resCfg config.ResourceSyncConfig, templatesDir string) *config.SyncConfig {
	cfg := &config.SyncConfig{
		MigrationName: resCfg.Name,
		Source:        resCfg.Source,
		Target:        resCfg.Target,
		Transform:     resCfg.Transform,
		Performance:   resCfg.Performance,
	}

	// Set defaults
	if cfg.Performance.MaxWorkers == 0 {
		cfg.Performance.MaxWorkers = 1
	}
	if cfg.Performance.RateLimitResourcesPerSecond == 0 {
		cfg.Performance.RateLimitResourcesPerSecond = 50
	}
	if cfg.Performance.RateLimitBurst == 0 {
		cfg.Performance.RateLimitBurst = 100
	}

	// Handle template loading from file
	if cfg.Transform.Template == "" && cfg.Transform.TemplatePath != "" {
		// Load template from file
		templatePath := cfg.Transform.TemplatePath
		if templatesDir != "" {
			templatePath = filepath.Join(templatesDir, templatePath)
		}
		templateBytes, err := os.ReadFile(templatePath)
		if err != nil {
			log.Fatal().Err(err).
				Str("path", templatePath).
				Str("resource", resCfg.Name).
				Msg("failed to read template file")
		}
		cfg.Transform.Template = string(templateBytes)
		log.Info().
			Str("path", templatePath).
			Str("resource", resCfg.Name).
			Msg("loaded template from file")
	}

	return cfg
}
