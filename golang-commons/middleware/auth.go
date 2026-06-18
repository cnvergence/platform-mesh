package middleware

import (
	"net/http"

	appctx "github.com/platform-mesh/golang-commons/context"
)

const AuthorizationHeader = "Authorization"

// StoreAuthHeader returns HTTP middleware that reads the request's Authorization header and stores it in the request context.
// The middleware wraps a handler, extracts the Authorization header (using AuthorizationHeader), calls
// appctx.AddAuthHeaderToContext with the existing request context and the header value, and invokes the next handler
// with the request updated to use that context. If the Authorization header is absent or empty, nothing is stored.
func StoreAuthHeader() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			auth := request.Header.Get(AuthorizationHeader)
			ctx := request.Context()
			if auth != "" {
				ctx = appctx.AddAuthHeaderToContext(ctx, auth)
			}
			next.ServeHTTP(responseWriter, request.WithContext(ctx))
		})
	}
}
