package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// SetOtelTracingContext returns an HTTP middleware that extracts OpenTelemetry
// tracing context from incoming request headers and injects it into the request's
// context before passing the request to the next handler.
//
// The middleware uses the global OpenTelemetry text map propagator and
// propagation.HeaderCarrier to read trace/span context from the request headers.
// Any extraction behavior (including failure handling) is delegated to the
// propagator implementation.
func SetOtelTracingContext() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			ctx := otel.GetTextMapPropagator().Extract(request.Context(), propagation.HeaderCarrier(request.Header))
			next.ServeHTTP(responseWriter, request.WithContext(ctx))
		})
	}
}
