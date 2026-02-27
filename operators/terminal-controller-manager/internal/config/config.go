/*
Copyright 2024.

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

package config

import "time"

// OperatorConfig holds the configuration for the terminal-controller-manager
type OperatorConfig struct {
	Subroutines struct {
		Lifetime struct {
			Enabled bool `mapstructure:"subroutines-lifetime-enabled" default:"true"`
		} `mapstructure:",squash"`
		Pod struct {
			Enabled bool `mapstructure:"subroutines-pod-enabled" default:"true"`
		} `mapstructure:",squash"`
		Service struct {
			Enabled bool `mapstructure:"subroutines-service-enabled" default:"true"`
		} `mapstructure:",squash"`
		HTTPRoute struct {
			Enabled bool `mapstructure:"subroutines-httproute-enabled" default:"true"`
		} `mapstructure:",squash"`
	} `mapstructure:",squash"`
	Kcp struct {
		APIExportEndpointSliceName string `mapstructure:"kcp-api-export-endpoint-slice-name" default:"terminal.platform-mesh.io"`
		// Kubeconfig is the path to the kubeconfig file for connecting to KCP.
		// If empty, falls back to in-cluster config.
		Kubeconfig string `mapstructure:"kcp-kubeconfig" default:""`
	} `mapstructure:",squash"`
	Terminal struct {
		Image          string        `mapstructure:"terminal-image" default:"ghcr.io/platform-mesh/terminal:latest"`
		Namespace      string        `mapstructure:"terminal-namespace" default:"terminal-sessions"`
		Lifetime       time.Duration `mapstructure:"terminal-lifetime" default:"2h"`
		HostAliasIP    string        `mapstructure:"terminal-host-alias-ip" default:""`
		HostAliasNames []string      `mapstructure:"terminal-host-alias-names"`
	} `mapstructure:",squash"`
	Gateway struct {
		Name      string   `mapstructure:"gateway-name" default:"k8sapi-gateway"`
		Namespace string   `mapstructure:"gateway-namespace" default:"platform-mesh-system"`
		Hostnames []string `mapstructure:"gateway-hostnames"`
	} `mapstructure:",squash"`
}
