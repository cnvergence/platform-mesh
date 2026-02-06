package config

// OperatorConfig holds the configuration for the KCP Migration Operator (main mode)
type OperatorConfig struct {
	// Subroutines configuration for enabling/disabling individual subroutines
	Subroutines struct {
		// ValidateSpec validates the KCPMigration spec
		ValidateSpec struct {
			Enabled bool `mapstructure:"subroutines-validate-spec-enabled" default:"true"`
		} `mapstructure:",squash"`
		// CreateConfigMap creates the sync configuration ConfigMap
		CreateConfigMap struct {
			Enabled bool `mapstructure:"subroutines-create-configmap-enabled" default:"true"`
		} `mapstructure:",squash"`
		// CreateChildOperator creates the child operator deployment
		CreateChildOperator struct {
			Enabled bool `mapstructure:"subroutines-create-child-operator-enabled" default:"true"`
		} `mapstructure:",squash"`
		// UpdateStatus updates the KCPMigration status
		UpdateStatus struct {
			Enabled bool `mapstructure:"subroutines-update-status-enabled" default:"true"`
		} `mapstructure:",squash"`
	} `mapstructure:",squash"`

	// ChildOperator configuration for the dynamically spawned child operators
	ChildOperator struct {
		// Image is the container image for child operators
		Image string `mapstructure:"child-operator-image" default:"ghcr.io/platform-mesh/kcp-migration-operator:latest"`
		// Resources configuration for child operator pods
		Resources struct {
			CPURequest    string `mapstructure:"child-operator-cpu-request" default:"100m"`
			CPULimit      string `mapstructure:"child-operator-cpu-limit" default:"500m"`
			MemoryRequest string `mapstructure:"child-operator-memory-request" default:"128Mi"`
			MemoryLimit   string `mapstructure:"child-operator-memory-limit" default:"256Mi"`
		} `mapstructure:",squash"`
	} `mapstructure:",squash"`

	// Secrets configuration for kubeconfig references
	Secrets struct {
		// KCPKubeconfig is the secret name containing the KCP kubeconfig
		KCPKubeconfig string `mapstructure:"secrets-kcp-kubeconfig" default:"kcp-kubeconfig"`
		// SourceKubeconfig is the secret name containing the source cluster kubeconfig
		SourceKubeconfig string `mapstructure:"secrets-source-kubeconfig" default:"source-kubeconfig"`
	} `mapstructure:",squash"`
}

// SourceConfig defines the source resource to watch
type SourceConfig struct {
	// APIVersion of the source resource (e.g., "fabric.foundation.sap.com/v1alpha1")
	APIVersion string `yaml:"apiVersion" mapstructure:"source-api-version"`
	// Kind of the source resource (e.g., "Account")
	Kind string `yaml:"kind" mapstructure:"source-kind"`
	// Namespace to filter source resources (optional, empty = all namespaces)
	Namespace string `yaml:"namespace,omitempty" mapstructure:"source-namespace"`
	// LabelSelectors is a list of label selectors to filter source resources
	// Resources must match ALL selectors (AND logic)
	LabelSelectors []string `yaml:"labelSelectors,omitempty" mapstructure:"source-label-selectors"`
}

// TargetConfig defines the target workspace and namespace
type TargetConfig struct {
	// WorkspaceExpression is a Go template to derive the target workspace
	WorkspaceExpression string `yaml:"workspaceExpression" mapstructure:"target-workspace-expression"`
	// Namespace in the target workspace (optional)
	Namespace string `yaml:"namespace,omitempty" mapstructure:"target-namespace"`
}

// TransformConfig defines the transformation template
type TransformConfig struct {
	// TemplatePath is the path to a template file (relative to templates directory)
	TemplatePath string `yaml:"templatePath,omitempty" mapstructure:"template-path"`
	// Template is an inline template (used if templatePath is not set)
	Template string `yaml:"template,omitempty" mapstructure:"template"`
	// ConfigMapName is the optional ConfigMap name containing the template
	ConfigMapName string `yaml:"configMapName,omitempty" mapstructure:"template-configmap-name"`
	// ConfigMapKey is the key in the ConfigMap containing the template
	ConfigMapKey string `yaml:"configMapKey,omitempty" mapstructure:"template-configmap-key" default:"template.yaml"`
}

// PerformanceConfig defines performance tuning options
type PerformanceConfig struct {
	// MaxWorkers is the number of concurrent reconciliation workers
	MaxWorkers int `yaml:"maxWorkers,omitempty" mapstructure:"max-workers" default:"1"`
	// RateLimitResourcesPerSecond limits sync operations per second
	RateLimitResourcesPerSecond int `yaml:"rateLimitResourcesPerSecond,omitempty" mapstructure:"rate-limit-resources-per-second" default:"50"`
	// RateLimitBurst is the burst size for rate limiting
	RateLimitBurst int `yaml:"rateLimitBurst,omitempty" mapstructure:"rate-limit-burst" default:"100"`
}

// SyncConfig holds the configuration for the sync mode (child operator)
type SyncConfig struct {
	// MigrationName is the name of the KCPMigration resource this sync is for
	MigrationName string `mapstructure:"migration-name"`

	// MigrationNamespace is the namespace of the KCPMigration resource
	MigrationNamespace string `mapstructure:"migration-namespace"`

	// Source configuration (embedded struct)
	Source SourceConfig `yaml:"source" mapstructure:",squash"`

	// Target configuration (embedded struct)
	Target TargetConfig `yaml:"target" mapstructure:",squash"`

	// Transform configuration (embedded struct)
	Transform TransformConfig `yaml:"transform" mapstructure:",squash"`

	// Performance configuration (embedded struct)
	Performance PerformanceConfig `yaml:"performance" mapstructure:",squash"`

	// KCPKubeconfigPath is the path to the KCP kubeconfig file
	KCPKubeconfigPath string `mapstructure:"kcp-kubeconfig-path" default:"/etc/kcp/kubeconfig"`

	// SourceKubeconfigPath is the path to the source cluster kubeconfig file
	SourceKubeconfigPath string `mapstructure:"source-kubeconfig-path"`
}

// ResourceSyncConfig defines a single resource type to sync
// This is used in the multi-resource sync file format
type ResourceSyncConfig struct {
	// Name is a unique identifier for this sync configuration
	Name string `yaml:"name" mapstructure:"name"`

	// Source configuration (reuses SourceConfig struct)
	Source SourceConfig `yaml:"source" mapstructure:"source"`

	// Target configuration (reuses TargetConfig struct)
	Target TargetConfig `yaml:"target" mapstructure:"target"`

	// Transform configuration (reuses TransformConfig struct)
	Transform TransformConfig `yaml:"transform,omitempty" mapstructure:"transform"`

	// Performance configuration (reuses PerformanceConfig struct)
	Performance PerformanceConfig `yaml:"performance,omitempty" mapstructure:"performance"`
}

// MultiSyncConfig holds configuration for multi-resource sync mode
// This is loaded from a YAML configuration file
type MultiSyncConfig struct {
	// KCPKubeconfigPath is the path to the KCP kubeconfig file
	KCPKubeconfigPath string `yaml:"kcpKubeconfigPath" mapstructure:"kcp-kubeconfig-path"`

	// SourceKubeconfigPath is the path to the source cluster kubeconfig file
	SourceKubeconfigPath string `yaml:"sourceKubeconfigPath,omitempty" mapstructure:"source-kubeconfig-path"`

	// TemplatesDir is the directory containing template files
	// Template paths in resource configs are relative to this directory
	TemplatesDir string `yaml:"templatesDir,omitempty" mapstructure:"templates-dir"`

	// Resources is the list of resource types to sync
	Resources []ResourceSyncConfig `yaml:"resources" mapstructure:"resources"`
}
