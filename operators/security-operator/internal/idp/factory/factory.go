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

package factory

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-oidc"
	"go.platform-mesh.io/security-operator/internal/config"
	"go.platform-mesh.io/security-operator/internal/idp"
	"go.platform-mesh.io/security-operator/internal/idp/keycloak"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

func Create2LeggedProvider(cfg *config.Config) (idp.Provider, error) {
	switch "TODO: cfg.IDP.Implementation" {
	case "keycloak":
		ctx := context.Background()

		issuer := fmt.Sprintf("%s/realms/master", cfg.Keycloak.BaseURL)
		provider, err := oidc.NewProvider(ctx, issuer)
		if err != nil {
			return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
		}

		cCfg := clientcredentials.Config{
			ClientID:     cfg.Keycloak.ClientID,
			ClientSecret: cfg.Keycloak.ClientSecret,
			TokenURL:     provider.Endpoint().TokenURL,
		}

		baseHTTPClient := cCfg.Client(ctx)

		return keycloak.New(baseHTTPClient, cfg.Keycloak.BaseURL, createKeycloakConfig(cfg)), nil

	default:
		panic("invalid IDP provider")
	}
}

func Create3LeggedProvider(cfg *config.InitContainerConfiguration, globalConfig *config.Config, password string) (idp.Provider, error) {
	switch "TODO: cfg.IDP.Implementation" {
	case "keycloak":
		ctx := context.Background()

		issuer := fmt.Sprintf("%s/realms/master", cfg.IDPBaseURL)
		provider, err := oidc.NewProvider(ctx, issuer)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OIDC provider: %w", err)
		}

		oauthCfg := oauth2.Config{
			ClientID: cfg.IDPClientID,
			Endpoint: provider.Endpoint(),
		}

		token, err := oauthCfg.PasswordCredentialsToken(ctx, cfg.IDPUser, password)
		if err != nil {
			return nil, fmt.Errorf("failed to get token: %w", err)
		}

		httpClient := oauthCfg.Client(ctx, token)

		return keycloak.New(httpClient, cfg.IDPBaseURL, createKeycloakConfig(globalConfig)), nil

	default:
		panic("invalid IDP provider")
	}
}

func createKeycloakConfig(cfg *config.Config) keycloak.Config {
	return keycloak.Config{
		AccessTokenLifespan: time.Duration(cfg.IDP.AccessTokenLifespan) * time.Second,
		SetDefaultPassword:  cfg.SetDefaultPassword,
		SMTP:                createKeycloakSMTP(cfg.IDP),
	}
}

func createKeycloakSMTP(cfg config.IDPConfig) *keycloak.SMTPConfig {
	if cfg.SMTPServer == "" {
		return nil
	}

	smtp := &keycloak.SMTPConfig{
		Host:     cfg.SMTPServer,
		Port:     cfg.SMTPPort,
		From:     cfg.FromAddress,
		SSL:      cfg.SSL,
		StartTLS: cfg.StartTLS,
	}

	if cfg.SMTPUser != "" {
		smtp.Auth = true
		smtp.User = cfg.SMTPUser
		smtp.Password = cfg.SMTPPassword
	}

	return smtp
}
