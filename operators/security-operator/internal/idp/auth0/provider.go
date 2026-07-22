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

	"go.platform-mesh.io/security-operator/internal/idp"
	"go.platform-mesh.io/security-operator/internal/idp/dcr"
)

type ManagementClient struct {
	mgmt         *mgmtclient.Management
	baseURL      string
	oauth2Config clientcredentials.Config

	mu sync.Mutex
	ts oauth2.TokenSource
}

func NewManagementClient(ctx context.Context, baseURL, clientID, clientSecret string, opts ...option.RequestOption) *ManagementClient {
	baseURL = strings.TrimSuffix(baseURL, "/")
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + baseURL
	}

	c := &ManagementClient{
		baseURL: baseURL,
		oauth2Config: clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     baseURL + "/oauth/token",
			AuthStyle:    oauth2.AuthStyleInParams,
			EndpointParams: url.Values{
				"audience": {baseURL + "/api/v2/"},
			},
		},
	}
	c.ts = c.oauth2Config.TokenSource(ctx)

	options := append([]option.RequestOption{
		option.WithBaseURL(baseURL + "/api/v2"),
		option.WithTokenSource(managementTokenSource{c}),
	}, opts...)
	c.mgmt = mgmtclient.NewWithOptions(options...)

	return c
}

func (c *ManagementClient) token() (*oauth2.Token, error) {
	c.mu.Lock()
	ts := c.ts
	c.mu.Unlock()
	return ts.Token()
}

type managementTokenSource struct {
	c *ManagementClient
}

func (s managementTokenSource) Token() (*oauth2.Token, error) {
	return s.c.token()
}

type registrationTokenProvider struct {
	c *ManagementClient
}

func (p registrationTokenProvider) TokenForRegistration(ctx context.Context) (string, error) {
	return p.c.GetInitialAccessToken(ctx, "")
}

func (c *ManagementClient) dcrClient() dcr.Client {
	return dcr.NewClient(dcr.WithTokenProvider(registrationTokenProvider{c}))
}

func (c *ManagementClient) RegisterClient(ctx context.Context, metadata dcr.ClientMetadata) (dcr.ClientInformation, error) {
	return c.dcrClient().Register(ctx, c.baseURL+"/oidc/register", metadata)
}

func (c *ManagementClient) GetClient(ctx context.Context, clientID, registrationURI, registrationToken string) (dcr.ClientInformation, error) {
	return c.dcrClient().Read(ctx, clientID, registrationURI, registrationToken)
}

func (c *ManagementClient) UpdateClient(ctx context.Context, registrationURI, registrationToken string, metadata dcr.ClientMetadata) (dcr.ClientInformation, error) {
	return c.dcrClient().Update(ctx, registrationURI, registrationToken, metadata)
}

func (c *ManagementClient) DeleteClient(ctx context.Context, clientID, registrationURI, registrationToken string) error {
	return c.dcrClient().Delete(ctx, clientID, registrationURI, registrationToken)
}

func (c *ManagementClient) CreateTenant(ctx context.Context, config idp.TenantConfig) (created bool, err error) {
	_, err = c.mgmt.Organizations.Create(ctx, &management.CreateOrganizationRequestContent{
		Name: config.Realm,
	})
	if err == nil {
		return true, nil
	}
	if !isStatus(err, http.StatusConflict) {
		return false, fmt.Errorf("failed to create organization %q: %w", config.Realm, err)
	}

	return false, nil
}

func (c *ManagementClient) UpdateTenant(ctx context.Context, tenantID string, config idp.TenantConfig) error {
	org, err := c.getOrganizationByName(ctx, tenantID)
	if err != nil {
		return err
	}
	if org == nil {
		return fmt.Errorf("organization %q not found for update", tenantID)
	}

	name := config.Realm
	if _, err := c.mgmt.Organizations.Update(ctx, deref(org.ID), &management.UpdateOrganizationRequestContent{
		Name: &name,
	}); err != nil {
		return fmt.Errorf("failed to update organization %q: %w", tenantID, err)
	}

	return nil
}

func (c *ManagementClient) DeleteTenant(ctx context.Context, tenantID string) error {
	org, err := c.getOrganizationByName(ctx, tenantID)
	if err != nil {
		return err
	}
	if org == nil {
		return nil
	}

	if err := c.mgmt.Organizations.Delete(ctx, deref(org.ID)); err != nil && !isStatus(err, http.StatusNotFound) {
		return fmt.Errorf("failed to delete organization %q: %w", tenantID, err)
	}

	return nil
}

func (c *ManagementClient) TenantExists(ctx context.Context, tenantID string) (bool, error) {
	org, err := c.getOrganizationByName(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return org != nil, nil
}

func (c *ManagementClient) getOrganizationByName(ctx context.Context, name string) (*management.GetOrganizationByNameResponseContent, error) {
	org, err := c.mgmt.Organizations.GetByName(ctx, name)
	if err != nil {
		if isStatus(err, http.StatusNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get organization %q: %w", name, err)
	}
	return org, nil
}

func (c *ManagementClient) GetInitialAccessToken(ctx context.Context, tenantID string) (string, error) {
	token, err := c.token()
	if err != nil {
		return "", fmt.Errorf("failed to get management token: %w", err)
	}
	return token.AccessToken, nil
}

func (c *ManagementClient) RefreshRegistrationToken(ctx context.Context, tenantID, clientID string) (string, error) {
	c.mu.Lock()
	c.ts = c.oauth2Config.TokenSource(ctx)
	ts := c.ts
	c.mu.Unlock()

	token, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("failed to refresh management token: %w", err)
	}
	return token.AccessToken, nil
}

func (c *ManagementClient) GetUserByEmail(ctx context.Context, tenantID, email string) (*idp.User, error) {
	users, err := c.mgmt.Users.ListUsersByEmail(ctx, &management.ListUsersByEmailRequestParameters{
		Email: email,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}
	return toUser(users[0]), nil
}

func (c *ManagementClient) ListUsers(ctx context.Context, tenantID string, opts idp.ListUsersOptions) ([]*idp.User, error) {
	if opts.Email != "" {
		user, err := c.GetUserByEmail(ctx, tenantID, opts.Email)
		if err != nil {
			return nil, err
		}
		return []*idp.User{user}, nil
	}

	page, err := c.mgmt.Users.List(ctx, &management.ListUsersRequestParameters{})
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}

	var users []*idp.User
	iter := page.Iterator()
	for iter.Next(ctx) {
		users = append(users, toUser(iter.Current()))
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}

	return users, nil
}

func (c *ManagementClient) IssuerURL(tenantID string) string {
	return c.baseURL + "/"
}

func (c *ManagementClient) JWKSURL(tenantID string) string {
	return c.baseURL + "/.well-known/jwks.json"
}

func (c *ManagementClient) AuthorizationEndpoint(tenantID string) string {
	return c.baseURL + "/authorize"
}

func (c *ManagementClient) TokenEndpoint(tenantID string) string {
	return c.baseURL + "/oauth/token"
}

func toUser(u *management.UserResponseSchema) *idp.User {
	return &idp.User{
		ID:      deref(u.UserID),
		Email:   deref(u.Email),
		Enabled: u.Blocked == nil || !*u.Blocked,
	}
}

func isStatus(err error, statusCode int) bool {
	if apiErr, ok := errors.AsType[*core.APIError](err); ok {
		return apiErr.StatusCode == statusCode
	}
	return false
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

//  dcr.TokenRefresher is not implemented and not needed. Auth0 never builds a RetryTransport, so nothing consumes RefreshToken(ctx, clientID)
var (
	_ idp.Provider       = (*ManagementClient)(nil)
	_ oauth2.TokenSource = managementTokenSource{}
	_ dcr.TokenProvider  = registrationTokenProvider{}
)
