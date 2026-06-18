package sentry

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStart(t *testing.T) {
	err := Start(context.Background(), "", "", "", "", "")
	assert.NoError(t, err)
}

func TestCaptureError(t *testing.T) {
	assert.NotPanics(t, func() {
		err := fmt.Errorf("test error")
		CaptureError(err, nil)
	})
}

func TestCaptureErrorNil(t *testing.T) {
	assert.NotPanics(t, func() {
		CaptureError(nil, nil)
	})
}

func TestCaptureSentryError(t *testing.T) {

	assert.NotPanics(t, func() {
		err := SentryError(fmt.Errorf("test error"))
		CaptureSentryError(err, nil)
	})
}

func TestWrap_NoPanic(t *testing.T) {
	called := false

	wrapped := Wrap(func() {
		called = true
	}, nil)

	assert.NotPanics(t, func() {
		wrapped()
	})

	assert.True(t, called, "wrapped function should be executed")
}

func TestWrap_PanicIsRecovered(t *testing.T) {
	wrapped := Wrap(func() {
		panic("nil pointer exception")
	}, Tags{"component": "test"})

	assert.NotPanics(t, func() {
		wrapped()
	})
}
