package middleware

import (
	"net/http"
)

// CreateAuthMiddleware returns a slice of middleware functions for authentication and authorization.
// The returned middlewares are: StoreWebToken, StoreAuthHeader, and StoreSpiffeHeader.
func CreateAuthMiddleware() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		StoreWebToken(),
		StoreAuthHeader(),
		StoreSpiffeHeader(),
	}
}
