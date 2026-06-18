package middleware

import (
	"net/http"

	"github.com/platform-mesh/golang-commons/logger"
)

// StoreLoggerMiddleware returns an HTTP middleware that injects the provided
// logger into each request's context so downstream handlers can retrieve it.
func StoreLoggerMiddleware(log *logger.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = logger.StdLogger
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := logger.SetLoggerInContext(r.Context(), log)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
