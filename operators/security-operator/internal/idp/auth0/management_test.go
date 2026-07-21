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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/auth0/go-auth0/v2/management/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closedServer returns a server that is immediately shut down so any request
// to it fails with "connection refused", simulating a network error.
func closedServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	return srv
}

func managementClient(t *testing.T, srv *httptest.Server) *ManagementClient {
	t.Helper()
	return NewManagementClient(srv.URL, "m2m-client", "m2m-secret", option.WithoutRetries())
}

// withToken serves a management token on /oauth/token and delegates all other
// requests to next, asserting the bearer token is attached.
func withToken(t *testing.T, next http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			assert.Equal(t, http.MethodPost, r.Method)
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "client_credentials", r.Form.Get("grant_type"))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"access_token": "mgmt-token-123", "token_type": "Bearer", "expires_in": 3600}) //nolint:errcheck
			return
		}
		assert.Equal(t, "Bearer mgmt-token-123", r.Header.Get("Authorization"))
		next(w, r)
	}
}

func listClientsJSON(page int) string {
	clients := []map[string]string{}
	if page == 0 {
		clients = append(clients, map[string]string{"client_id": "client-id-1", "name": "my-client"})
	}
	b, _ := json.Marshal(map[string]any{
		"start":   page * 50,
		"limit":   50,
		"total":   1,
		"clients": clients,
	})
	return string(b)
}

func pageOf(r *http.Request) int {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	return page
}

func orgJSON(id, name string) string {
	b, _ := json.Marshal(map[string]string{"id": id, "name": name})
	return string(b)
}

func TestManagementClient_TokenForRegistration(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantToken   string
		wantErr     bool
	}{
		{
			name: "successful token retrieval",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "/oauth/token", r.URL.Path)
					require.NoError(t, r.ParseForm())
					assert.Equal(t, "client_credentials", r.Form.Get("grant_type"))
					assert.Equal(t, "m2m-client", r.Form.Get("client_id"))
					assert.Equal(t, "m2m-secret", r.Form.Get("client_secret"))
					assert.NotEmpty(t, r.Form.Get("audience"))
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"access_token": "initial-access-token-123", "token_type": "Bearer", "expires_in": 3600}) //nolint:errcheck
				}))
			},
			wantToken: "initial-access-token-123",
		},
		{
			name: "server returns error",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, "access denied") //nolint:errcheck
				}))
			},
			wantErr: true,
		},
		{
			name:        "connection refused",
			setupServer: closedServer,
			wantErr:     true,
		},
		{
			name: "decode error",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, "not-json") //nolint:errcheck
				}))
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			defer srv.Close()
			token, err := managementClient(t, srv).TokenForRegistration(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

func TestManagementClient_RefreshToken(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantToken   string
		wantErr     bool
	}{
		{
			name: "successful token refresh",
			setupServer: func(t *testing.T) *httptest.Server {
				call := 0
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					call++
					assert.Equal(t, "/oauth/token", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"access_token": fmt.Sprintf("token-%d", call), "token_type": "Bearer", "expires_in": 3600}) //nolint:errcheck
				}))
			},
			wantToken: "token-2",
		},
		{
			name: "token endpoint fails",
			setupServer: func(t *testing.T) *httptest.Server {
				call := 0
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					call++
					if call == 1 {
						w.Header().Set("Content-Type", "application/json")
						json.NewEncoder(w).Encode(map[string]any{"access_token": "token-1", "token_type": "Bearer", "expires_in": 3600}) //nolint:errcheck
						return
					}
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			wantErr: true,
		},
		{
			name:        "connection refused",
			setupServer: closedServer,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			defer srv.Close()
			c := managementClient(t, srv)
			// Prime the cached token; RefreshToken must discard it. The
			// clientID argument is ignored by Auth0.
			_, _ = c.TokenForRegistration(context.Background())
			token, err := c.RefreshToken(context.Background(), "ignored-client-id")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

func TestManagementClient_RegistrationEndpoint(t *testing.T) {
	assert.Equal(t,
		"https://my-tenant.eu.auth0.com/oidc/register",
		NewManagementClient("https://my-tenant.eu.auth0.com", "id", "secret").RegistrationEndpoint(),
	)
}

func TestManagementClient_RegistrationEndpoint_TrailingSlash(t *testing.T) {
	assert.Equal(t,
		"https://my-tenant.eu.auth0.com/oidc/register",
		NewManagementClient("https://my-tenant.eu.auth0.com/", "id", "secret").RegistrationEndpoint(),
	)
}

func TestManagementClient_RegistrationEndpoint_NoScheme(t *testing.T) {
	assert.Equal(t,
		"https://my-tenant.eu.auth0.com/oidc/register",
		NewManagementClient("my-tenant.eu.auth0.com", "id", "secret").RegistrationEndpoint(),
	)
}

func TestManagementClient_OrganizationExists(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantExists  bool
		wantErr     bool
	}{
		{
			name: "organization exists",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, "/api/v2/organizations/name/test-org", r.URL.Path)
					fmt.Fprint(w, orgJSON("org_123", "test-org")) //nolint:errcheck
				}))
			},
			wantExists: true,
		},
		{
			name: "organization not found",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			wantExists: false,
		},
		{
			name: "server error",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			wantErr: true,
		},
		{
			name:        "connection refused",
			setupServer: closedServer,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			defer srv.Close()
			exists, err := managementClient(t, srv).OrganizationExists(context.Background(), "test-org")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestManagementClient_CreateOrUpdateOrganization(t *testing.T) {
	tests := []struct {
		name        string
		config      OrganizationConfig
		setupServer func(t *testing.T) *httptest.Server
		wantCreated bool
		wantErr     bool
	}{
		{
			name:   "organization created",
			config: OrganizationConfig{Name: "new-org", DisplayName: "New Org", Metadata: map[string]string{"tenant": "new-org"}},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "/api/v2/organizations", r.URL.Path)
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					assert.Equal(t, "new-org", body["name"])
					assert.Equal(t, "New Org", body["display_name"])
					w.WriteHeader(http.StatusCreated)
					fmt.Fprint(w, orgJSON("org_123", "new-org")) //nolint:errcheck
				}))
			},
			wantCreated: true,
		},
		{
			name:   "organization updated on conflict",
			config: OrganizationConfig{Name: "existing-org", DisplayName: "Existing Org"},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					switch r.Method {
					case http.MethodPost:
						w.WriteHeader(http.StatusConflict)
					case http.MethodGet:
						assert.Equal(t, "/api/v2/organizations/name/existing-org", r.URL.Path)
						fmt.Fprint(w, orgJSON("org_123", "existing-org")) //nolint:errcheck
					default:
						assert.Equal(t, http.MethodPatch, r.Method)
						assert.Equal(t, "/api/v2/organizations/org_123", r.URL.Path)
						fmt.Fprint(w, orgJSON("org_123", "existing-org")) //nolint:errcheck
					}
				}))
			},
			wantCreated: false,
		},
		{
			name:   "conflict but organization vanished",
			config: OrganizationConfig{Name: "gone-org"},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost {
						w.WriteHeader(http.StatusConflict)
						return
					}
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			wantErr: true,
		},
		{
			name:   "server error on create",
			config: OrganizationConfig{Name: "error-org"},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			wantErr: true,
		},
		{
			name:        "connection refused on create",
			config:      OrganizationConfig{Name: "my-org"},
			setupServer: closedServer,
			wantErr:     true,
		},
		{
			name:   "update organization returns error status",
			config: OrganizationConfig{Name: "my-org"},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					switch r.Method {
					case http.MethodPost:
						w.WriteHeader(http.StatusConflict)
					case http.MethodGet:
						fmt.Fprint(w, orgJSON("org_123", "my-org")) //nolint:errcheck
					default:
						w.WriteHeader(http.StatusInternalServerError)
						fmt.Fprint(w, `{"message":"update failed"}`) //nolint:errcheck
					}
				}))
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			defer srv.Close()
			created, err := managementClient(t, srv).CreateOrUpdateOrganization(context.Background(), tt.config)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, created)
		})
	}
}

func TestManagementClient_DeleteOrganization(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantErr     bool
	}{
		{
			name: "successful delete",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						assert.Equal(t, "/api/v2/organizations/name/my-org", r.URL.Path)
						fmt.Fprint(w, orgJSON("org_123", "my-org")) //nolint:errcheck
						return
					}
					assert.Equal(t, http.MethodDelete, r.Method)
					assert.Equal(t, "/api/v2/organizations/org_123", r.URL.Path)
					w.WriteHeader(http.StatusNoContent)
				}))
			},
		},
		{
			name: "organization not found is success",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
		},
		{
			name: "server error",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						fmt.Fprint(w, orgJSON("org_123", "my-org")) //nolint:errcheck
						return
					}
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			wantErr: true,
		},
		{
			name:        "connection refused",
			setupServer: closedServer,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			defer srv.Close()
			err := managementClient(t, srv).DeleteOrganization(context.Background(), "my-org")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestManagementClient_ListClients(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
	}{
		{
			name:        "connection refused",
			setupServer: closedServer,
		},
		{
			name: "non-OK response",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				}))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			defer srv.Close()
			_, err := managementClient(t, srv).ListClients(context.Background())
			require.Error(t, err)
		})
	}
}

func TestManagementClient_GetClientByName(t *testing.T) {
	tests := []struct {
		name        string
		clientName  string
		setupServer func(t *testing.T) *httptest.Server
		wantClient  *ClientInfo
		wantErr     bool
	}{
		{
			name:       "client found",
			clientName: "my-client",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v2/clients", r.URL.Path)
					fmt.Fprint(w, listClientsJSON(pageOf(r))) //nolint:errcheck
				}))
			},
			wantClient: &ClientInfo{ClientID: "client-id-1", Name: "my-client"},
		},
		{
			name:       "client not found",
			clientName: "missing-client",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `{"start":0,"limit":50,"total":0,"clients":[]}`) //nolint:errcheck
				}))
			},
			wantClient: nil,
		},
		{
			name:       "list error",
			clientName: "any",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			defer srv.Close()
			info, err := managementClient(t, srv).GetClientByName(context.Background(), tt.clientName)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, info)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantClient, info)
		})
	}
}

func TestManagementClient_GetClientSecret(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantSecret  string
		wantErr     bool
	}{
		{
			name:        "connection refused",
			setupServer: closedServer,
			wantErr:     true,
		},
		{
			name: "non-200 response",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			wantErr: true,
		},
		{
			name: "success",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(withToken(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v2/clients/client-id-1", r.URL.Path)
					assert.Equal(t, "client_secret", r.URL.Query().Get("fields"))
					fmt.Fprint(w, `{"client_secret":"my-secret"}`) //nolint:errcheck
				}))
			},
			wantSecret: "my-secret",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			defer srv.Close()
			secret, err := managementClient(t, srv).GetClientSecret(context.Background(), "client-id-1")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSecret, secret)
		})
	}
}
