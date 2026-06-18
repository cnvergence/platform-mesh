package config_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/platform-mesh/golang-commons/config"
)

func TestSetConfigInContext(t *testing.T) {
	ctx := context.Background()
	configStr := "test"
	ctx = config.SetConfigInContext(ctx, configStr)

	retrievedConfig := config.LoadConfigFromContext(ctx)
	assert.Equal(t, configStr, retrievedConfig)
}

func TestNewDefaultConfig(t *testing.T) {
	cfg := config.NewDefaultConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, 10, cfg.MaxConcurrentReconciles)
	assert.Equal(t, "local", cfg.Region)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, ":9090", cfg.Metrics.BindAddress)
	assert.Equal(t, ":8090", cfg.HealthProbeBindAddress)
	assert.Equal(t, time.Minute, cfg.ShutdownTimeout)
	assert.True(t, cfg.EnableHTTP2)
}
