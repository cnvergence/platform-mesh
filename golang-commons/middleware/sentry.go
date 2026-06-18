package middleware

import (
	"net/http"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/platform-mesh/golang-commons/logger"
)

// Recoverer implements a middleware that recover from panics, sends them to Sentry
// SentryRecoverer returns an http.Handler that wraps next and recovers from panics.
//
// If a panic occurs (except http.ErrAbortHandler) the middleware logs the panic and stack
// trace, reports the error to the current Sentry hub, flushes Sentry events (up to 5s),
// and responds with HTTP 500 Internal Server Error. The returned handler otherwise
// delegates to next.ServeHTTP.
func SentryRecoverer(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil && err != http.ErrAbortHandler {
				log := logger.LoadLoggerFromContext(r.Context())
				log.Error().Interface("panic", err).Interface("stack", debug.Stack()).Msg("recovered http panic")
				sentry.CurrentHub().Recover(err)
				sentry.Flush(time.Second * 5)

				w.WriteHeader(http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
