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

// Package auth0 provides an Auth0 Management API client (backed by the
// official github.com/auth0/go-auth0 SDK) that plugs into the clientreg package.
package auth0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/auth0/go-auth0/v2/management"
	mgmtclient "github.com/auth0/go-auth0/v2/management/client"
	"github.com/auth0/go-auth0/v2/management/core"
	"github.com/auth0/go-auth0/v2/management/option"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"go.platform-mesh.io/security-operator/internal/idp/dcr"
)

// Provider wraps the Auth0 Management API SDK and implements the
// dcr token interfaces for OIDC dynamic client registration.
type Provider struct {
	mgmt         *mgmtclient.Management
	domain       string
	oauth2Config clientcredentials.Config

	mu sync.Mutex
	ts oauth2.TokenSource
}

var (
	_ dcr.TokenProvider  = (*Provider)(nil)
	_ dcr.TokenRefresher = (*Provider)(nil)
	// _ idp.Provider       = (*Provider)(nil)
	_ oauth2.TokenSource = (*Provider)(nil)
)

// New creates a client for the Auth0 tenant at domain
func New(domain, clientID, clientSecret string, opts ...option.RequestOption) *Provider {
	domain = strings.TrimSuffix(domain, "/")
	if !strings.Contains(domain, "://") {
		domain = "https://" + domain
	}

	c := &Provider{
		domain: domain,
		oauth2Config: clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     domain + "/oauth/token",
			AuthStyle:    oauth2.AuthStyleInParams,
			EndpointParams: url.Values{
				"audience": {domain + "/api/v2/"},
			},
		},
	}
	c.ts = c.oauth2Config.TokenSource(context.Background())

	options := append([]option.RequestOption{
		option.WithBaseURL(domain + "/api/v2"),
		option.WithTokenSource(c),
	}, opts...)
	c.mgmt = mgmtclient.NewWithOptions(options...)

	return c
}

func (c *Provider) Management() *mgmtclient.Management {
	return c.mgmt
}

func (c *Provider) RegistrationEndpoint() string {
	return c.domain + "/oidc/register"
}

func (c *Provider) Token() (*oauth2.Token, error) {
	c.mu.Lock()
	ts := c.ts
	c.mu.Unlock()
	return ts.Token()
}

func (c *Provider) TokenForRegistration(ctx context.Context) (string, error) {
	c.mu.Lock()
	ts := c.ts
	c.mu.Unlock()

	token, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get management token: %w", err)
	}
	return token.AccessToken, nil
}

func (c *Provider) RefreshToken(ctx context.Context, _ string) (string, error) {
	c.mu.Lock()
	c.ts = c.oauth2Config.TokenSource(context.Background())
	ts := c.ts
	c.mu.Unlock()

	token, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("failed to refresh management token: %w", err)
	}
	return token.AccessToken, nil
}

// OrganizationConfig describes an Auth0 Organization, the Auth0 equivalent
// of a platform-mesh tenant.
type OrganizationConfig struct {
	Name        string
	DisplayName string
	Metadata    map[string]string
}

type OrganizationInfo struct {
	ID          string
	Name        string
	DisplayName string
}

type ClientInfo struct {
	ClientID string
	Name     string
	Secret   string
}

func (c *Provider) OrganizationExists(ctx context.Context, name string) (bool, error) {
	org, err := c.GetOrganizationByName(ctx, name)
	if err != nil {
		return false, err
	}
	return org != nil, nil
}

func (c *Provider) GetOrganizationByName(ctx context.Context, name string) (*OrganizationInfo, error) {
	org, err := c.mgmt.Organizations.GetByName(ctx, name)
	if err != nil {
		if isStatus(err, http.StatusNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get organization %q: %w", name, err)
	}

	return &OrganizationInfo{
		ID:          deref(org.ID),
		Name:        deref(org.Name),
		DisplayName: deref(org.DisplayName),
	}, nil
}

func (c *Provider) CreateOrUpdateOrganization(ctx context.Context, config OrganizationConfig) (created bool, err error) {
	createReq := &management.CreateOrganizationRequestContent{
		Name: config.Name,
	}
	if config.DisplayName != "" {
		createReq.DisplayName = &config.DisplayName
	}
	if len(config.Metadata) > 0 {
		metadata := toOrganizationMetadata(config.Metadata)
		createReq.Metadata = &metadata
	}

	_, err = c.mgmt.Organizations.Create(ctx, createReq)
	if err == nil {
		return true, nil
	}
	if !isStatus(err, http.StatusConflict) {
		return false, fmt.Errorf("failed to create organization %q: %w", config.Name, err)
	}

	return false, c.updateOrganization(ctx, config)
}

func (c *Provider) updateOrganization(ctx context.Context, config OrganizationConfig) error {
	org, err := c.GetOrganizationByName(ctx, config.Name)
	if err != nil {
		return err
	}
	if org == nil {
		return fmt.Errorf("organization %q not found for update", config.Name)
	}

	updateReq := &management.UpdateOrganizationRequestContent{}
	if config.DisplayName != "" {
		updateReq.DisplayName = &config.DisplayName
	}
	if len(config.Metadata) > 0 {
		metadata := toOrganizationMetadata(config.Metadata)
		updateReq.Metadata = &metadata
	}

	if _, err := c.mgmt.Organizations.Update(ctx, org.ID, updateReq); err != nil {
		return fmt.Errorf("failed to update organization %q: %w", config.Name, err)
	}

	return nil
}

func (c *Provider) DeleteOrganization(ctx context.Context, name string) error {
	org, err := c.GetOrganizationByName(ctx, name)
	if err != nil {
		return err
	}
	if org == nil {
		return nil
	}

	if err := c.mgmt.Organizations.Delete(ctx, org.ID); err != nil && !isStatus(err, http.StatusNotFound) {
		return fmt.Errorf("failed to delete organization %q: %w", name, err)
	}

	return nil
}

func (c *Provider) ListClients(ctx context.Context) ([]ClientInfo, error) {
	fields := "client_id,name"
	includeFields := true

	page, err := c.mgmt.Clients.List(ctx, &management.ListClientsRequestParameters{
		Fields:        &fields,
		IncludeFields: &includeFields,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list clients: %w", err)
	}

	var clients []ClientInfo
	iter := page.Iterator()
	for iter.Next(ctx) {
		client := iter.Current()
		clients = append(clients, ClientInfo{
			ClientID: deref(client.ClientID),
			Name:     deref(client.Name),
		})
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list clients: %w", err)
	}

	return clients, nil
}

func (c *Provider) GetClientByName(ctx context.Context, clientName string) (*ClientInfo, error) {
	clients, err := c.ListClients(ctx)
	if err != nil {
		return nil, err
	}

	for _, client := range clients {
		if client.Name == clientName {
			return &client, nil
		}
	}

	return nil, nil
}

func (c *Provider) GetClientSecret(ctx context.Context, clientID string) (string, error) {
	fields := "client_secret"
	includeFields := true

	client, err := c.mgmt.Clients.Get(ctx, clientID, &management.GetClientRequestParameters{
		Fields:        &fields,
		IncludeFields: &includeFields,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get client secret for %q: %w", clientID, err)
	}

	return deref(client.ClientSecret), nil
}

func isStatus(err error, statusCode int) bool {
	if apiErr, ok := errors.AsType[*core.APIError](err); ok {
		return apiErr.StatusCode == statusCode
	}
	return false
}

func toOrganizationMetadata(m map[string]string) management.OrganizationMetadata {
	metadata := make(management.OrganizationMetadata, len(m))
	for k, v := range m {
		metadata[k] = &v
	}
	return metadata
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
