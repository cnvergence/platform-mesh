package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/golang-commons/logger/testlogger"
	"github.com/stretchr/testify/assert"
)

func TestStoreLoggerMiddleware(t *testing.T) {
	testLog := testlogger.New()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify logger is stored in context
		logFromContext := logger.LoadLoggerFromContext(r.Context())
		assert.NotNil(t, logFromContext)

		// The logger should be the same instance we passed
		assert.Equal(t, testLog.Logger, logFromContext)

		w.WriteHeader(http.StatusOK)
	})

	middleware := StoreLoggerMiddleware(testLog.Logger)
	handlerToTest := middleware(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	recorder := httptest.NewRecorder()

	handlerToTest.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestStoreLoggerMiddleware_NilLogger(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Even with nil logger, the middleware should not panic
		w.WriteHeader(http.StatusOK)
	})

	middleware := StoreLoggerMiddleware(nil)
	handlerToTest := middleware(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	recorder := httptest.NewRecorder()

	// Should not panic
	assert.NotPanics(t, func() {
		handlerToTest.ServeHTTP(recorder, req)
	})

	assert.Equal(t, http.StatusOK, recorder.Code)
}
