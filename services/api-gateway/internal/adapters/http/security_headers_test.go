package http_test

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
)

// Cabeceras de seguridad de la API (sección 26.5). Complementan TestSecurityHeaders,
// que solo cubre nosniff y X-Frame-Options.

func TestAPIResponsesSendReferrerPolicyAndCSP(t *testing.T) {
	rec := do(newRouter(&fakeAuth{}, true), http.MethodGet, "/health", "", nil)

	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
}

// Echo solo envía HSTS sobre conexiones TLS, así que la prueba usa una petición TLS.
func TestAPIResponsesSendHSTSOverTLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()

	newRouter(&fakeAuth{}, true).ServeHTTP(rec, req)

	assert.Contains(t, rec.Header().Get("Strict-Transport-Security"), "max-age=")
}

func TestRateLimitAnswersTooManyRequests(t *testing.T) {
	// Límite bajo a propósito: newRouter usa 1000 por segundo y nunca llegaría a 429.
	opts := httpapi.Options{
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RateLimitPerSecond: 1, RateLimitBurst: 5, LoginRatePerMinute: 6000,
		RequestTimeout: time.Second, BodyLimit: "1K",
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ready := func(context.Context) bool { return true }
	h := httpapi.NewRouter(log, opts, jwt.NewVerifier(secret, "lastmile-auth"), &fakeAuth{}, nil, ready)

	var codes []int
	for i := 0; i < 20; i++ {
		codes = append(codes, do(h, http.MethodGet, "/api/v1/users", "", nil).Code)
	}
	assert.Contains(t, codes, http.StatusTooManyRequests, "la API debe limitar por IP")
}

// ZAP 90004: toda respuesta, incluidas las de error, lleva Cross-Origin-Resource-Policy.
func TestAPIResponsesSendCrossOriginResourcePolicy(t *testing.T) {
	h := newRouterWithSignup(&fakeAuth{}, httpapi.Options{EnableDocs: true})

	for _, tc := range []struct {
		name, path string
		status     int
	}{
		{"health", "/health", http.StatusOK},
		{"ready", "/ready", http.StatusOK},
		{"openapi", "/openapi.yaml", http.StatusOK},
		{"api route", "/api/v1/users", http.StatusUnauthorized},
		{"unauthorized", "/api/v1/auth/me", http.StatusUnauthorized},
		{"not found", "/api/v1/no-such-route", http.StatusNotFound},
	} {
		rec := do(h, http.MethodGet, tc.path, "", nil)

		assert.Equal(t, tc.status, rec.Code, tc.name)
		assert.Equal(t, "same-origin", rec.Header().Get("Cross-Origin-Resource-Policy"), tc.name)
	}
}
