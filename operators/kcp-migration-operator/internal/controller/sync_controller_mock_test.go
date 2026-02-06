package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/platform-mesh/kcp-migration-operator/internal/config"
)

// createTestController creates a SyncController for testing
func createTestController(cfg *config.SyncConfig) *SyncController {
	return &SyncController{
		Config: cfg,
	}
}

func TestPrepareTargetResource_PassThrough(t *testing.T) {
	tests := []struct {
		name           string
		source         *unstructured.Unstructured
		cfg            *config.SyncConfig
		expectNS       string
		checkCleanMeta bool
	}{
		{
			name: "should clean metadata and preserve data",
			source: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":              "test-cm",
						"namespace":         "source-ns",
						"uid":               "12345",
						"resourceVersion":   "67890",
						"creationTimestamp": "2024-01-01T00:00:00Z",
						"generation":        int64(1),
						"managedFields":     []interface{}{},
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			cfg:            &config.SyncConfig{},
			expectNS:       "source-ns",
			checkCleanMeta: true,
		},
		{
			name: "should override namespace from config",
			source: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "source-ns",
					},
				},
			},
			cfg: &config.SyncConfig{
				Target: config.TargetConfig{
					Namespace: "target-ns",
				},
			},
			expectNS: "target-ns",
		},
		{
			name: "should add source tracking annotations",
			source: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "default",
						"uid":       "test-uid-123",
					},
				},
			},
			cfg:      &config.SyncConfig{},
			expectNS: "default",
		},
		{
			name: "should preserve existing annotations",
			source: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "default",
						"uid":       "test-uid-123",
						"annotations": map[string]interface{}{
							"existing-annotation": "should-be-preserved",
						},
					},
				},
			},
			cfg:      &config.SyncConfig{},
			expectNS: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := createTestController(tt.cfg)
			target, err := controller.prepareTargetResource(tt.source)

			require.NoError(t, err)

			// Check namespace
			assert.Equal(t, tt.expectNS, target.GetNamespace())

			// Check that name is preserved
			assert.Equal(t, tt.source.GetName(), target.GetName())

			// Check that metadata is cleaned
			metadata := target.Object["metadata"].(map[string]interface{})
			if tt.checkCleanMeta {
				assert.Nil(t, metadata["uid"])
				assert.Nil(t, metadata["resourceVersion"])
				assert.Nil(t, metadata["creationTimestamp"])
				assert.Nil(t, metadata["managedFields"])
				assert.Nil(t, metadata["generation"])
			}

			// Check source tracking annotation exists
			annotations := metadata["annotations"].(map[string]interface{})
			assert.Contains(t, annotations, "migration.platform-mesh.io/source-uid")
			assert.Contains(t, annotations, "migration.platform-mesh.io/source-generation")

			// If source had existing annotations, check they're preserved
			if tt.name == "should preserve existing annotations" {
				assert.Equal(t, "should-be-preserved", annotations["existing-annotation"])
			}
		})
	}
}

func TestPrepareTargetResource_ClusterScopedResource(t *testing.T) {
	// Test with a cluster-scoped resource (no namespace)
	source := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "core.platform-mesh.io/v1alpha1",
			"kind":       "Account",
			"metadata": map[string]interface{}{
				"name": "test-account",
				"uid":  "account-uid-123",
			},
			"spec": map[string]interface{}{
				"type":        "account",
				"displayName": "Test Account",
			},
		},
	}

	cfg := &config.SyncConfig{}
	controller := createTestController(cfg)
	target, err := controller.prepareTargetResource(source)

	require.NoError(t, err)

	// Cluster-scoped resources should have empty namespace
	assert.Empty(t, target.GetNamespace())

	// Name should be preserved
	assert.Equal(t, "test-account", target.GetName())

	// Kind should be preserved
	assert.Equal(t, "Account", target.GetKind())

	// Spec should be preserved
	spec := target.Object["spec"].(map[string]interface{})
	assert.Equal(t, "account", spec["type"])
	assert.Equal(t, "Test Account", spec["displayName"])
}

func TestPrepareTargetResource_WithTemplate(t *testing.T) {
	// Test transformation with template
	source := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "fabric.foundation.sap.com/v1alpha1",
			"kind":       "Account",
			"metadata": map[string]interface{}{
				"name":      "test-project",
				"namespace": "account-2bzns",
				"uid":       "source-uid-123",
			},
			"spec": map[string]interface{}{
				"accountRole": "Project",
				"displayName": "Test Project",
				"owner":       "I123456",
			},
		},
	}

	template := `apiVersion: core.platform-mesh.io/v1alpha1
kind: Account
metadata:
  name: "{{ index .Source.metadata "name" }}"
spec:
  type: project
  displayName: "{{ getFieldStr .Source "spec.displayName" }}"
  creator: "{{ getFieldStr .Source "spec.owner" }}"`

	cfg := &config.SyncConfig{
		Transform: config.TransformConfig{
			Template: template,
		},
	}
	controller := createTestController(cfg)
	target, err := controller.prepareTargetResource(source)

	require.NoError(t, err)

	// Check target was transformed
	assert.Equal(t, "core.platform-mesh.io/v1alpha1", target.GetAPIVersion())
	assert.Equal(t, "Account", target.GetKind())
	assert.Equal(t, "test-project", target.GetName())

	// Check spec was transformed
	spec, found, err := unstructured.NestedStringMap(target.Object, "spec")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "project", spec["type"])
	assert.Equal(t, "Test Project", spec["displayName"])
	assert.Equal(t, "I123456", spec["creator"])

	// Check migration tracking annotations were added
	annotations := target.GetAnnotations()
	assert.Equal(t, "source-uid-123", annotations["migration.platform-mesh.io/source-uid"])
	assert.Equal(t, "test-project", annotations["migration.platform-mesh.io/source-name"])
	assert.Equal(t, "account-2bzns", annotations["migration.platform-mesh.io/source-namespace"])
}

func TestPrepareTargetResource_TemplateWithConditional(t *testing.T) {
	// Test template with conditional logic for account type mapping
	tests := []struct {
		name         string
		accountRole  string
		expectedType string
	}{
		{"Project maps to project", "Project", "project"},
		{"Team maps to team", "Team", "team"},
		{"Folder maps to org", "Folder", "org"},
	}

	template := `apiVersion: core.platform-mesh.io/v1alpha1
kind: Account
metadata:
  name: "{{ index .Source.metadata "name" }}"
spec:
  type: "{{ $role := getFieldStr .Source "spec.accountRole" }}{{ if eq $role "Project" }}project{{ else if eq $role "Team" }}team{{ else }}org{{ end }}"
  displayName: "{{ getFieldStr .Source "spec.displayName" }}"`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "fabric.foundation.sap.com/v1alpha1",
					"kind":       "Account",
					"metadata": map[string]interface{}{
						"name": "test-account",
					},
					"spec": map[string]interface{}{
						"accountRole": tt.accountRole,
						"displayName": "Test Account",
					},
				},
			}

			cfg := &config.SyncConfig{
				Transform: config.TransformConfig{
					Template: template,
				},
			}
			controller := createTestController(cfg)
			target, err := controller.prepareTargetResource(source)

			require.NoError(t, err)

			typeVal, found, err := unstructured.NestedString(target.Object, "spec", "type")
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, tt.expectedType, typeVal)
		})
	}
}
