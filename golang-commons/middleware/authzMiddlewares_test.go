package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateAuthMiddleware(t *testing.T) {
	middlewares := CreateAuthMiddleware()

	// Expect 3 middlewares: StoreWebToken, StoreAuthHeader, StoreSpiffeHeader
	assert.Len(t, middlewares, 3)

	// Each middleware should not be nil
	for _, mw := range middlewares {
		assert.NotNil(t, mw)
	}
}

func TestCreateAuthMiddleware_ReturnsCorrectMiddlewares(t *testing.T) {
	middlewares := CreateAuthMiddleware()

	// Should return exactly 3 middlewares
	assert.Len(t, middlewares, 3)

	// Each middleware should be a valid function
	for _, mw := range middlewares {
		assert.NotNil(t, mw)
		// Signature is implicitly tested by compilation and return type
	}
}
