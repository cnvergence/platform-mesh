package config

import "github.com/spf13/pflag"

type OperatorConfig struct {
	Kcp            KcpConfig
	WebhookEnabled bool
}

type KcpConfig struct {
	Enabled                    bool
	ApiExportEndpointSliceName string
}

func NewOperatorConfig() OperatorConfig {
	return OperatorConfig{
		Kcp: KcpConfig{
			Enabled:                    false,
			ApiExportEndpointSliceName: "resource-sharding",
		},
		WebhookEnabled: true,
	}
}

func (c *OperatorConfig) AddFlags(flags *pflag.FlagSet) {
	flags.BoolVar(&c.Kcp.Enabled, "kcp-enabled", c.Kcp.Enabled, "Enable KCP multicluster provider")
	flags.StringVar(&c.Kcp.ApiExportEndpointSliceName, "kcp-api-export-endpoint-slice-name", c.Kcp.ApiExportEndpointSliceName, "Name of the APIExportEndpointSlice to use for KCP")
	flags.BoolVar(&c.WebhookEnabled, "webhook-enabled", c.WebhookEnabled, "Enable mutating admission webhook registration")
}
