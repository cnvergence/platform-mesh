package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/platform-mesh/golang-commons/context/keys"
	"github.com/platform-mesh/golang-commons/logger"
)

const requestIdHeader = "X-Request-Id"

// SetRequestId returns an HTTP middleware that ensures each request has a request ID.
// It reads the `X-Request-Id` header (used only if exactly one value is present); otherwise
// it generates a new UUID. The request ID is stored in the request context under
// keys.RequestIdCtxKey and the request is forwarded to the next handler with the updated context.
func SetRequestId() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			ctx := request.Context()
			var requestId string
			if ids, ok := request.Header[requestIdHeader]; ok && len(ids) == 1 {
				requestId = ids[0]
			} else {
				// Generate a new request id, header was not received.
				requestId = uuid.New().String()
			}
			ctx = context.WithValue(ctx, keys.RequestIdCtxKey, requestId)
			next.ServeHTTP(responseWriter, request.WithContext(ctx))
		})
	}
}

// SetRequestIdInLogger returns HTTP middleware that injects a request-scoped logger into the request context.
//
// The middleware loads the current logger from the request context, creates a per-request logger using
// logger.NewRequestLoggerFromZerolog(ctx, log.Logger), and stores the resulting logger back into the context
// before calling the next handler. This ensures handlers downstream receive a logger enriched for the current request.
func SetRequestIdInLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			ctx := request.Context()
			log := logger.LoadLoggerFromContext(ctx)
			log = logger.NewRequestLoggerFromZerolog(ctx, log.Logger)
			ctx = logger.SetLoggerInContext(ctx, log)
			next.ServeHTTP(responseWriter, request.WithContext(ctx))
		})
	}
}

// GetRequestId returns the request ID stored in ctx under keys.RequestIdCtxKey.
// If the value is missing or not a string, it returns the empty string.
func GetRequestId(ctx context.Context) string {
	if val, ok := ctx.Value(keys.RequestIdCtxKey).(string); ok {
		return val
	}
	return ""
}
