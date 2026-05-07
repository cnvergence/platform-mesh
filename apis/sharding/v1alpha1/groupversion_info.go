// Package v1alpha1 contains API Schema definitions for the sharding v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=sharding.platform-mesh.io
package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion = schema.GroupVersion{Group: "sharding.platform-mesh.io", Version: "v1alpha1"}

	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	AddToScheme = SchemeBuilder.AddToScheme
)
