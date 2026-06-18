package middleware

import (
	"net/http"

	"github.com/platform-mesh/golang-commons/logger"
)

// CreateMiddleware creates a middleware chain with logging, tracing, and optional authentication.
// It attaches a request-scoped logger (using the provided logger), assigns a request ID, and propagates that ID into the logger.
// When auth is true, authentication middlewares (StoreWebToken, StoreAuthHeader, StoreSpiffeHeader) are included.
func CreateMiddleware(log *logger.Logger, auth bool) []func(http.Handler) http.Handler {
	mws := []func(http.Handler) http.Handler{
		SetOtelTracingContext(),
		SentryRecoverer,
		StoreLoggerMiddleware(log),
		SetRequestId(),
		SetRequestIdInLogger(),
	}

	if auth {
		mws = append(mws, CreateAuthMiddleware()...)
	}
	return mws
}
