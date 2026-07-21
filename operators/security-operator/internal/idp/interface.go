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

package idp

import "context"

type ClientMetadata struct{}
type ClientInformation struct{}
type TenantConfig struct{}
type ListUsersOptions struct{}
type User struct{}

// Provider defines the interface for an external OIDC provider
type Provider interface {
	// Client Registration (DCR - RFC 7591/7592)
	RegisterClient(ctx context.Context, metadata ClientMetadata) (ClientInformation, error)
	GetClient(ctx context.Context, clientID, registrationURI, registrationToken string) (ClientInformation, error)
	UpdateClient(ctx context.Context, registrationURI, registrationToken string, metadata ClientMetadata) (ClientInformation, error)
	DeleteClient(ctx context.Context, clientID, registrationURI, registrationToken string) error

	// Realm (org) Management (provider-specific)
	CreateTenant(ctx context.Context, config TenantConfig) (created bool, err error)
	UpdateTenant(ctx context.Context, tenantID string, config TenantConfig) error
	DeleteTenant(ctx context.Context, tenantID string) error
	TenantExists(ctx context.Context, tenantID string) (bool, error)

	// Token Management
	GetInitialAccessToken(ctx context.Context, tenantID string) (string, error)
	RefreshRegistrationToken(ctx context.Context, tenantID, clientID string) (string, error)

	// User Management (SCIM or Userinfo)
	GetUserByEmail(ctx context.Context, tenantID, email string) (*User, error)
	ListUsers(ctx context.Context, tenantID string, opts ListUsersOptions) ([]*User, error)

	// Configuration
	IssuerURL(tenantID string) string
	JWKSURL(tenantID string) string
	AuthorizationEndpoint(tenantID string) string
	TokenEndpoint(tenantID string) string
}
