package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/platform-mesh/golang-commons/context"
	"github.com/stretchr/testify/assert"
)

func TestStoreWebToken_WithFakeBearerToken(t *testing.T) {
	token := "fake.invalid.token"
	authHeader := "Bearer " + token

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Token parsing will fail due to fake token, which is expected in tests
		// The middleware should handle this gracefully
		_, err := context.GetWebTokenFromContext(r.Context())
		// For test purposes, we just verify the middleware doesn't crash
		// and that token validation fails as expected with fake tokens
		assert.Error(t, err) // This is expected behavior when token validation fails

		w.WriteHeader(http.StatusOK)
	})

	middleware := StoreWebToken()
	handlerToTest := middleware(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	req.Header.Set(AuthorizationHeader, authHeader)
	recorder := httptest.NewRecorder()

	handlerToTest.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestStoreWebToken_WithoutAuthHeader(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Context should not have a token
		_, err := context.GetWebTokenFromContext(r.Context())
		assert.Error(t, err) // Should return an error when no token is present

		w.WriteHeader(http.StatusOK)
	})

	middleware := StoreWebToken()
	handlerToTest := middleware(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	// No authorization header set
	recorder := httptest.NewRecorder()

	handlerToTest.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestStoreWebToken_WithNonBearerToken(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Context should not have a token
		_, err := context.GetWebTokenFromContext(r.Context())
		assert.Error(t, err) // Should return an error when no valid token is present

		w.WriteHeader(http.StatusOK)
	})

	middleware := StoreWebToken()
	handlerToTest := middleware(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	req.Header.Set(AuthorizationHeader, "Basic dXNlcjpwYXNz") // Basic auth, not Bearer
	recorder := httptest.NewRecorder()

	handlerToTest.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestStoreWebToken_WithEmptyBearerToken(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Context should not have a token due to empty token
		_, err := context.GetWebTokenFromContext(r.Context())
		assert.Error(t, err)

		w.WriteHeader(http.StatusOK)
	})

	middleware := StoreWebToken()
	handlerToTest := middleware(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	req.Header.Set(AuthorizationHeader, "Bearer ")
	recorder := httptest.NewRecorder()

	handlerToTest.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestStoreWebToken_WithFakeBearerTokenLowercase(t *testing.T) {
	token := "fake.invalid.token"
	authHeader := "bearer " + token // lowercase

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Token parsing will fail due to fake token, which is expected in tests
		// The middleware should process lowercase bearer tokens but validation will fail
		_, err := context.GetWebTokenFromContext(r.Context())
		// This is expected behavior when token validation fails with fake tokens
		assert.Error(t, err)

		w.WriteHeader(http.StatusOK)
	})

	middleware := StoreWebToken()
	handlerToTest := middleware(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	req.Header.Set(AuthorizationHeader, authHeader)
	recorder := httptest.NewRecorder()

	handlerToTest.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestStoreWebToken_WithMalformedAuthHeader(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Context should not have a token
		_, err := context.GetWebTokenFromContext(r.Context())
		assert.Error(t, err)

		w.WriteHeader(http.StatusOK)
	})

	middleware := StoreWebToken()
	handlerToTest := middleware(nextHandler)

	req := httptest.NewRequest("GET", "http://testing", nil)
	req.Header.Set(AuthorizationHeader, "Bearer") // Missing space and token
	recorder := httptest.NewRecorder()

	handlerToTest.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}
