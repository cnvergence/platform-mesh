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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.platform-mesh.io/security-operator/internal/idp"
)

// testServer wraps the given handler with an OAuth token endpoint so the Auth0
// management client can obtain a token before every management API call.
func testServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"access_token": "management-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// closedServer returns a server that is immediately shut down so any request to
// it fails with "connection refused", simulating a network error.
func closedServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	return srv
}

func testClient(t *testing.T, srv *httptest.Server) *ManagementClient {
	t.Helper()
	return NewManagementClient(context.Background(), srv.URL, "client-id", "client-secret")
}

func TestNewManagementClient_BaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "plain domain gets https scheme", baseURL: "tenant.auth0.com", want: "https://tenant.auth0.com"},
		{name: "trailing slash is trimmed", baseURL: "https://tenant.auth0.com/", want: "https://tenant.auth0.com"},
		{name: "explicit scheme is preserved", baseURL: "http://localhost:8080", want: "http://localhost:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewManagementClient(context.Background(), tt.baseURL, "id", "secret")
			assert.Equal(t, tt.want, c.baseURL)
		})
	}
}

func TestManagementClient_CreateTenant(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantCreated bool
		wantErr     bool
	}{
		{
			name: "organization created",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "/api/v2/organizations", r.URL.Path)
					json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
				})
			},
			wantCreated: true,
		},
		{
			name: "organization updated on conflict",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodPost && r.URL.Path == "/api/v2/organizations":
						w.WriteHeader(http.StatusConflict)
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/organizations/name/my-org":
						json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
					case r.Method == http.MethodPatch && r.URL.Path == "/api/v2/organizations/org-1":
						json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
					default:
						t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
					}
				})
			},
			wantCreated: false,
		},
		{
			name: "create returns error",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				})
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
			created, err := testClient(t, srv).CreateTenant(context.Background(), idp.TenantConfig{Realm: "my-org"})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, created)
		})
	}
}

func TestManagementClient_UpdateTenant(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantErr     bool
	}{
		{
			name: "successful update",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/organizations/name/my-org":
						json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
					case r.Method == http.MethodPatch && r.URL.Path == "/api/v2/organizations/org-1":
						json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
					default:
						t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
					}
				})
			},
		},
		{
			name: "organization not found",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				})
			},
			wantErr: true,
		},
		{
			name: "get organization error",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				})
			},
			wantErr: true,
		},
		{
			name: "update returns error",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
						return
					}
					w.WriteHeader(http.StatusForbidden)
				})
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
			err := testClient(t, srv).UpdateTenant(context.Background(), "my-org", idp.TenantConfig{Realm: "my-org"})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestManagementClient_DeleteTenant(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantErr     bool
	}{
		{
			name: "successful delete",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/organizations/name/my-org":
						json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
					case r.Method == http.MethodDelete && r.URL.Path == "/api/v2/organizations/org-1":
						w.WriteHeader(http.StatusNoContent)
					default:
						t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
					}
				})
			},
		},
		{
			name: "organization not found is success",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				})
			},
		},
		{
			name: "delete 404 is success",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
						return
					}
					w.WriteHeader(http.StatusNotFound)
				})
			},
		},
		{
			name: "delete returns error",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
						return
					}
					w.WriteHeader(http.StatusForbidden)
				})
			},
			wantErr: true,
		},
		{
			name: "get organization error",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				})
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
			err := testClient(t, srv).DeleteTenant(context.Background(), "my-org")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestManagementClient_TenantExists(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantExists  bool
		wantErr     bool
	}{
		{
			name: "organization exists",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v2/organizations/name/my-org", r.URL.Path)
					json.NewEncoder(w).Encode(map[string]any{"id": "org-1", "name": "my-org"}) //nolint:errcheck
				})
			},
			wantExists: true,
		},
		{
			name: "organization not found",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				})
			},
			wantExists: false,
		},
		{
			name: "server error",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				})
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
			exists, err := testClient(t, srv).TenantExists(context.Background(), "my-org")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestManagementClient_GetInitialAccessToken(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantToken   string
		wantErr     bool
	}{
		{
			name: "successful token retrieval",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {})
			},
			wantToken: "management-token",
		},
		{
			name:        "token endpoint unreachable",
			setupServer: closedServer,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			token, err := testClient(t, srv).GetInitialAccessToken(context.Background(), "")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

func TestManagementClient_RefreshRegistrationToken(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantToken   string
		wantErr     bool
	}{
		{
			name: "successful token refresh",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {})
			},
			wantToken: "management-token",
		},
		{
			name:        "token endpoint unreachable",
			setupServer: closedServer,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer(t)
			token, err := testClient(t, srv).RefreshRegistrationToken(context.Background(), "", "")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

func TestManagementClient_GetUserByEmail(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		wantUser    *idp.User
		wantErr     bool
	}{
		{
			name: "user found",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v2/users-by-email", r.URL.Path)
					json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck
						{"user_id": "auth0|123", "email": "user@example.com", "blocked": false},
					})
				})
			},
			wantUser: &idp.User{ID: "auth0|123", Email: "user@example.com", Enabled: true},
		},
		{
			name: "blocked user is disabled",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck
						{"user_id": "auth0|123", "email": "user@example.com", "blocked": true},
					})
				})
			},
			wantUser: &idp.User{ID: "auth0|123", Email: "user@example.com", Enabled: false},
		},
		{
			name: "user not found",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode([]map[string]any{}) //nolint:errcheck
				})
			},
			wantErr: true,
		},
		{
			name: "query error",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				})
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
			user, err := testClient(t, srv).GetUserByEmail(context.Background(), "my-org", "user@example.com")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUser, user)
		})
	}
}

func TestManagementClient_ListUsers(t *testing.T) {
	tests := []struct {
		name        string
		opts        idp.ListUsersOptions
		setupServer func(t *testing.T) *httptest.Server
		wantUsers   []*idp.User
		wantErr     bool
	}{
		{
			name: "list users by email option",
			opts: idp.ListUsersOptions{Email: "user@example.com"},
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v2/users-by-email", r.URL.Path)
					json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck
						{"user_id": "auth0|123", "email": "user@example.com"},
					})
				})
			},
			wantUsers: []*idp.User{{ID: "auth0|123", Email: "user@example.com", Enabled: true}},
		},
		{
			name: "list users by email not found",
			opts: idp.ListUsersOptions{Email: "missing@example.com"},
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode([]map[string]any{}) //nolint:errcheck
				})
			},
			wantErr: true,
		},
		{
			name: "list all users",
			setupServer: func(t *testing.T) *httptest.Server {
				call := 0
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/api/v2/users", r.URL.Path)
					call++
					if call == 1 {
						json.NewEncoder(w).Encode(map[string]any{"users": []map[string]any{ //nolint:errcheck
							{"user_id": "auth0|1", "email": "a@example.com"},
							{"user_id": "auth0|2", "email": "b@example.com", "blocked": true},
						}})
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"users": []map[string]any{}}) //nolint:errcheck
				})
			},
			wantUsers: []*idp.User{
				{ID: "auth0|1", Email: "a@example.com", Enabled: true},
				{ID: "auth0|2", Email: "b@example.com", Enabled: false},
			},
		},
		{
			name: "list all users error",
			setupServer: func(t *testing.T) *httptest.Server {
				return testServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				})
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
			users, err := testClient(t, srv).ListUsers(context.Background(), "my-org", tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUsers, users)
		})
	}
}

func TestManagementClient_Endpoints(t *testing.T) {
	c := NewManagementClient(context.Background(), "https://tenant.auth0.com", "id", "secret")
	assert.Equal(t, "https://tenant.auth0.com/", c.IssuerURL(""))
	assert.Equal(t, "https://tenant.auth0.com/.well-known/jwks.json", c.JWKSURL(""))
	assert.Equal(t, "https://tenant.auth0.com/authorize", c.AuthorizationEndpoint(""))
	assert.Equal(t, "https://tenant.auth0.com/oauth/token", c.TokenEndpoint(""))
}

func TestDeref(t *testing.T) {
	s := "value"
	assert.Equal(t, "value", deref(&s))
	assert.Equal(t, "", deref(nil))
}
