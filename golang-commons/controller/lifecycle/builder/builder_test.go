package builder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/ratelimiter"
	pmtesting "github.com/platform-mesh/golang-commons/controller/testSupport"
	"github.com/platform-mesh/golang-commons/logger"
)

func TestNewBuilder_Defaults(t *testing.T) {
	log := &logger.Logger{}
	b := NewBuilder("op", "ctrl", nil, log)
	if b.operatorName != "op" {
		t.Errorf("expected operatorName 'op', got %s", b.operatorName)
	}
	if b.controllerName != "ctrl" {
		t.Errorf("expected controllerName 'ctrl', got %s", b.controllerName)
	}
	if b.withConditionManagement {
		t.Error("expected withConditionManagement to be false")
	}
	if b.withSpreadingReconciles {
		t.Error("expected withSpreadingReconciles to be false")
	}
	if b.withReadOnly {
		t.Error("expected withReadOnly to be false")
	}
	if b.log != log {
		t.Error("expected log to be set")
	}
}

func TestBuilder_WithConditionManagement(t *testing.T) {
	b := NewBuilder("op", "ctrl", nil, &logger.Logger{})
	b.WithConditionManagement()
	if !b.withConditionManagement {
		t.Error("WithConditionManagement should set withConditionManagement to true")
	}
}

func TestBuilder_WithSpreadingReconciles(t *testing.T) {
	b := NewBuilder("op", "ctrl", nil, &logger.Logger{})
	b.WithSpreadingReconciles()
	if !b.withSpreadingReconciles {
		t.Error("WithSpreadingReconciles should set withSpreadingReconciles to true")
	}
}

func TestBuilder_WithReadOnly(t *testing.T) {
	b := NewBuilder("op", "ctrl", nil, &logger.Logger{})
	b.WithReadOnly()
	if !b.withReadOnly {
		t.Error("WithReadOnly should set withReadOnly to true")
	}
}

func TestBuilder_WithCustomRateLimiter(t *testing.T) {
	t.Run("With options", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{})
		opts := []ratelimiter.Option{
			ratelimiter.WithRequeueDelay(5 * time.Second),
			ratelimiter.WithStaticWindow(1 * time.Minute),
		}
		b.WithStaticThenExponentialRateLimiter(opts...)
		if b.rateLimiterOptions == nil {
			t.Error("expected rateLimiterOptions to be non-nil")
		}
		if got := len(*b.rateLimiterOptions); got != 2 {
			t.Errorf("expected 2 rate limiter options, got %d", got)
		}
	})
	t.Run("Without options", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{})
		b.WithStaticThenExponentialRateLimiter()
		if b.rateLimiterOptions == nil {
			t.Error("expected rateLimiterOptions to be non-nil even with no options")
		}
		if got := len(*b.rateLimiterOptions); got != 0 {
			t.Errorf("expected 0 rate limiter options, got %d", got)
		}
	})

	t.Run("Without custom rate limiter", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{})
		assert.Nil(t, b.rateLimiterOptions)
	})
}

func TestControllerRuntimeBuilder(t *testing.T) {
	t.Run("Minimal setup", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{})
		fakeClient := pmtesting.CreateFakeClient(t, &pmtesting.TestApiObject{})
		lm := b.BuildControllerRuntime(fakeClient)
		assert.NotNil(t, lm)
	})
	t.Run("All Options", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{}).WithConditionManagement().WithSpreadingReconciles()
		fakeClient := pmtesting.CreateFakeClient(t, &pmtesting.TestApiObject{})
		lm := b.BuildControllerRuntime(fakeClient)
		assert.NotNil(t, lm)
	})
	t.Run("ReadOnly", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{}).WithReadOnly()
		fakeClient := pmtesting.CreateFakeClient(t, &pmtesting.TestApiObject{})
		lm := b.BuildControllerRuntime(fakeClient)
		assert.NotNil(t, lm)
	})
	t.Run("WithCustomRateLimiter", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{}).WithStaticThenExponentialRateLimiter(
			ratelimiter.WithRequeueDelay(5*time.Second),
			ratelimiter.WithStaticWindow(1*time.Minute),
			ratelimiter.WithExponentialInitialBackoff(5*time.Second),
		)
		fakeClient := pmtesting.CreateFakeClient(t, &pmtesting.TestApiObject{})
		lm := b.BuildControllerRuntime(fakeClient)
		assert.NotNil(t, lm)
	})
}

func TestMulticontrollerRuntimeBuilder(t *testing.T) {
	t.Run("Minimal setup", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{})
		cfg := &rest.Config{}
		provider := pmtesting.NewFakeProvider(cfg)
		mgr, err := mcmanager.New(cfg, provider, mcmanager.Options{})
		assert.NoError(t, err)
		lm := b.BuildMultiCluster(mgr)
		assert.NotNil(t, lm)
	})
	t.Run("All Options", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{}).WithConditionManagement().WithSpreadingReconciles()
		cfg := &rest.Config{}
		provider := pmtesting.NewFakeProvider(cfg)
		mgr, err := mcmanager.New(cfg, provider, mcmanager.Options{})
		assert.NoError(t, err)
		lm := b.BuildMultiCluster(mgr)
		assert.NotNil(t, lm)
	})
	t.Run("ReadOnly", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{}).WithReadOnly()
		cfg := &rest.Config{}
		provider := pmtesting.NewFakeProvider(cfg)
		mgr, err := mcmanager.New(cfg, provider, mcmanager.Options{})
		assert.NoError(t, err)
		lm := b.BuildMultiCluster(mgr)
		assert.NotNil(t, lm)
	})
	t.Run("WithCustomRateLimiter", func(t *testing.T) {
		b := NewBuilder("op", "ctrl", nil, &logger.Logger{}).WithStaticThenExponentialRateLimiter(
			ratelimiter.WithRequeueDelay(5*time.Second),
			ratelimiter.WithStaticWindow(1*time.Minute),
			ratelimiter.WithExponentialInitialBackoff(5*time.Second),
		)
		cfg := &rest.Config{}
		provider := pmtesting.NewFakeProvider(cfg)
		mgr, err := mcmanager.New(cfg, provider, mcmanager.Options{})
		assert.NoError(t, err)
		lm := b.BuildMultiCluster(mgr)
		assert.NotNil(t, lm)
	})
}
