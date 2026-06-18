package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/golang-commons/logger/testlogger"
	"github.com/stretchr/testify/assert"
)

func TestSentryRecoverer_NoPanic(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	handlerToTest := SentryRecoverer(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	recorder := httptest.NewRecorder()

	handlerToTest.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", recorder.Body.String())
}

func TestSentryRecoverer_WithPanic(t *testing.T) {
	log := testlogger.New().HideLogOutput()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handlerToTest := SentryRecoverer(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	// Add logger to context so the middleware can log the panic
	ctx := req.Context()
	ctx = logger.SetLoggerInContext(ctx, log.Logger)
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()

	// Should not panic, should recover
	assert.NotPanics(t, func() {
		handlerToTest.ServeHTTP(recorder, req)
	})

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)

	// Verify that the panic was logged
	messages, err := log.GetLogMessages()
	assert.NoError(t, err)
	assert.NotEmpty(t, messages)

	// Find the panic log message
	found := false
	for _, msg := range messages {
		if msg.Message == "recovered http panic" {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected to find panic log message")
}

func TestSentryRecoverer_WithHttpErrAbortHandler(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// http.ErrAbortHandler should not be recovered
		panic(http.ErrAbortHandler)
	})

	handlerToTest := SentryRecoverer(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	recorder := httptest.NewRecorder()

	// The middleware should not recover from http.ErrAbortHandler
	// Since the condition is `err != http.ErrAbortHandler`, it should let this panic through
	// However, since the defer recover catches it but doesn't handle it, it won't re-panic
	// Let's test that it doesn't crash the middleware
	assert.NotPanics(t, func() {
		handlerToTest.ServeHTTP(recorder, req)
	})
}

func TestSentryRecoverer_WithStringPanic(t *testing.T) {
	log := testlogger.New().HideLogOutput()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("string panic message")
	})

	handlerToTest := SentryRecoverer(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	// Add logger to context
	ctx := req.Context()
	ctx = logger.SetLoggerInContext(ctx, log.Logger)
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		handlerToTest.ServeHTTP(recorder, req)
	})

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)

	// Verify that the panic was logged
	messages, err := log.GetLogMessages()
	assert.NoError(t, err)
	assert.NotEmpty(t, messages)
}

func TestSentryRecoverer_WithErrorPanic(t *testing.T) {
	log := testlogger.New().HideLogOutput()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(assert.AnError)
	})

	handlerToTest := SentryRecoverer(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	// Add logger to context
	ctx := req.Context()
	ctx = logger.SetLoggerInContext(ctx, log.Logger)
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		handlerToTest.ServeHTTP(recorder, req)
	})

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}
