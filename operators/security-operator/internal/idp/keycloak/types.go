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

package keycloak

import (
	"time"

	"go.platform-mesh.io/security-operator/internal/idp"
)

const (
	RequiredActionVerifyEmail    string = "VERIFY_EMAIL"
	RequiredActionUpdatePassword string = "UPDATE_PASSWORD"
	UserDefaultPasswordType      string = "password"
	UserDefaultPasswordValue     string = "password"
)

type Config struct {
	AccessTokenLifespan time.Duration
	SetDefaultPassword  bool
	SMTP                *SMTPConfig
}

type TenantConfig struct {
	Realm string `json:"realm"`
}

type ListUsersOptions struct {
	Email string `json:"email"`
}

// TODO: is this the same/superset as UserInfo?
type user struct {
	ID              string       `json:"id,omitempty"`
	Email           string       `json:"email,omitempty"`
	RequiredActions []string     `json:"requiredActions,omitempty"`
	Enabled         bool         `json:"enabled,omitempty"`
	Credentials     []credential `json:"credentials,omitempty"`
}

func (u *user) ToPublic() *idp.User {
	credentials := make([]idp.Credential, 0, len(u.Credentials))
	for _, c := range u.Credentials {
		public := c.ToPublic()
		credentials = append(credentials, *public)
	}

	return &idp.User{
		ID:              u.ID,
		Email:           u.Email,
		RequiredActions: u.RequiredActions,
		Enabled:         u.Enabled,
		Credentials:     credentials,
	}
}

type credential struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

func (c *credential) ToPublic() *idp.Credential {
	return &idp.Credential{
		Type:      c.Type,
		Value:     c.Value,
		Temporary: c.Temporary,
	}
}

type client struct {
	ID       string `json:"id,omitempty"`
	ClientID string `json:"clientId,omitempty"`
}

type realmConfig struct {
	Realm                       string      `json:"realm"`
	DisplayName                 string      `json:"displayName,omitempty"`
	Enabled                     bool        `json:"enabled"`
	LoginWithEmailAllowed       bool        `json:"loginWithEmailAllowed,omitempty"`
	RegistrationEmailAsUsername bool        `json:"registrationEmailAsUsername,omitempty"`
	RegistrationAllowed         bool        `json:"registrationAllowed,omitempty"`
	SSOSessionIdleTimeout       int         `json:"ssoSessionIdleTimeout,omitempty"`
	AccessTokenLifespan         int         `json:"accessTokenLifespan,omitempty"`
	SMTPServer                  *SMTPConfig `json:"smtpServer,omitempty"`
}

type SMTPConfig struct {
	Host     string `json:"host,omitempty"`
	Port     string `json:"port,omitempty"`
	From     string `json:"from,omitempty"`
	SSL      bool   `json:"ssl,omitempty"`
	StartTLS bool   `json:"starttls,omitempty"`
	Auth     bool   `json:"auth,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

type clientInfo struct {
	ID       string `json:"id"`
	ClientID string `json:"clientId"`
	Name     string `json:"name"`
	Secret   string `json:"secret"`
}

type serviceAccountClientConfig struct {
	ClientID               string `json:"clientId"`
	Name                   string `json:"name,omitempty"`
	Enabled                bool   `json:"enabled"`
	ServiceAccountsEnabled bool   `json:"serviceAccountsEnabled"`
	PublicClient           bool   `json:"publicClient"`
}

type userInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func (u *userInfo) ToPublic() *idp.UserInfo {
	return &idp.UserInfo{
		ID:       u.ID,
		Username: u.Username,
	}
}

type roleInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func createRoleInfo(ri idp.RoleInfo) roleInfo {
	return roleInfo{
		ID:   ri.ID,
		Name: ri.Name,
	}
}

func (r *roleInfo) ToPublic() *idp.RoleInfo {
	return &idp.RoleInfo{
		ID:   r.ID,
		Name: r.Name,
	}
}
