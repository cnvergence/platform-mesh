/*
Copyright The Platform Mesh Authors.

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

package webhook

import (
	"context"
	"fmt"
	"slices"
	"strings"

	pmcorev1alpha1 "go.platform-mesh.io/apis/core/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	mcruntime "sigs.k8s.io/multicluster-runtime"
)

type realmChecker interface {
	OrganizationExists(ctx context.Context, orgID string) (bool, error)
}

// SetupIdentityProviderConfigurationValidatingWebhookWithManager registers a validating webhook that prevents
// creation of an `IdentityProviderConfiguration` if the corresponding IDP tenant already exists.
// TODO: provider is a 2-legged provider.
func SetupIdentityProviderConfigurationValidatingWebhookWithManager(ctx context.Context, mgr ctrl.Manager, provider realmChecker, denyList []string) error {
	realmDenyList := slices.Clone(denyList) // TODO: Why are we cloning this slice?

	return mcruntime.NewWebhookManagedBy(mgr, &pmcorev1alpha1.IdentityProviderConfiguration{}).
		WithValidator(&identityProviderConfigurationValidator{provider: provider, realmDenyList: realmDenyList}).
		Complete()
}

var _ admission.Validator[*pmcorev1alpha1.IdentityProviderConfiguration] = (*identityProviderConfigurationValidator)(nil)

type identityProviderConfigurationValidator struct {
	provider      realmChecker
	realmDenyList []string
}

func (v *identityProviderConfigurationValidator) ValidateCreate(ctx context.Context, idp *pmcorev1alpha1.IdentityProviderConfiguration) (admission.Warnings, error) {
	tenantName := strings.TrimSpace(idp.GetName())
	if tenantName == "" {
		return nil, fmt.Errorf("tenant name must not be empty")
	}
	if tenantName == "master" {
		return nil, fmt.Errorf("creation of IdentityProviderConfiguration for tenant 'master' is not allowed")
	}
	if slices.Contains(v.realmDenyList, tenantName) {
		return nil, fmt.Errorf("creation of IdentityProviderConfiguration for tenant %q is not allowed", tenantName)
	}

	exists, err := v.provider.OrganizationExists(ctx, tenantName)
	if err != nil {
		return nil, fmt.Errorf("failed to check tenant existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("tenant %q already exists", tenantName)
	}

	return nil, nil
}

func (v *identityProviderConfigurationValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *pmcorev1alpha1.IdentityProviderConfiguration) (admission.Warnings, error) {
	// Intentionally allow updates to prevent deadlocks when reconcilers add status/finalizers.
	return nil, nil
}

func (v *identityProviderConfigurationValidator) ValidateDelete(ctx context.Context, obj *pmcorev1alpha1.IdentityProviderConfiguration) (admission.Warnings, error) {
	return nil, nil
}
