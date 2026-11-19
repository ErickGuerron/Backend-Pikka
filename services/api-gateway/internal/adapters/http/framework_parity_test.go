package http_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// Pruebas de caracterizacion: fijan el comportamiento HTTP transversal que hoy entrega httpx.
// Deben seguir pasando, sin cambios, despues de migrar el gateway a net/http.

func TestParity_CORSPreflightFromAllowedOrigin(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodOptions, "/api/v1/auth/login", "", map[string]string{
		"Origin":                         "http://localhost:5173",
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "content-type,authorization",
	})
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Equal(t, "600", rec.Header().Get("Access-Control-Max-Age"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key")
}

func TestParity_CORSRejectsUnknownOrigin(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodGet, "/health", "", map[string]string{"Origin": "https://evil.example"})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestParity_CORSExposesRequestID(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodGet, "/health", "", map[string]string{"Origin": "http://localhost:5173"})
	// echo canonicalizes the header name in this list ("X-Request-Id"); compare case-insensitively.
	assert.Contains(t, strings.ToLower(rec.Header().Get("Access-Control-Expose-Headers")), "x-request-id")
}

func TestParity_RequestIDReusedWhenSafe(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodGet, "/health", "", map[string]string{"X-Request-ID": "abc-123.ok_9"})
	assert.Equal(t, "abc-123.ok_9", rec.Header().Get("X-Request-ID"))
}

func TestParity_RequestIDReplacedWhenUnsafe(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	for _, bad := range []string{"has space", "bad/slash", strings.Repeat("a", 65)} {
		rec := do(h, http.MethodGet, "/health", "", map[string]string{"X-Request-ID": bad})
		got := rec.Header().Get("X-Request-ID")
		assert.NotEmpty(t, got, "input %q", bad)
		assert.NotEqual(t, bad, got, "unsafe id %q must not be echoed", bad)
	}
}

func TestParity_RequestIDGeneratedWhenAbsent(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodGet, "/health", "", nil)
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestParity_UnknownAPIRouteIs404JSON(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodGet, "/api/v1/no-such-route", "", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"requestId"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body.Error.Code)
	assert.NotEmpty(t, body.Error.RequestID)
}

// Contrato actual: un metodo no registrado en una ruta existente responde 404, no 405.
// net/http ServeMux responde 405; la migracion debe conservar 404 o cambiarlo de forma explicita.
func TestParity_WrongMethodIs404(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodGet, "/api/v1/auth/login", "", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestParity_OversizedBodyIs413(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	big := `{"email":"` + strings.Repeat("a", 2048) + `"}`
	rec := do(h, http.MethodPost, "/api/v1/auth/login", big, map[string]string{"Content-Type": "application/json"})
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

// panicAuth provoca un panic dentro de un handler para verificar la recuperacion.
type panicAuth struct{ fakeAuth }

func (panicAuth) Login(context.Context, grpcclient.CallMeta, *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	panic("boom")
}

func TestParity_PanicBecomesJSON500(t *testing.T) {
	h := newRouter(&panicAuth{}, true)
	rec := do(h, http.MethodPost, "/api/v1/auth/login", `{"email":"a@b.co","password":"x"}`, map[string]string{"Content-Type": "application/json"})
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	assert.Contains(t, rec.Body.String(), `"error"`)
}

func TestParity_SecurityHeadersPresent(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	rec := do(h, http.MethodGet, "/health", "", nil)
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
	// HSTS solo se emite sobre TLS (comportamiento de echo Secure).
	assert.Empty(t, rec.Header().Get("Strict-Transport-Security"))
}

func TestParity_HSTSOnlyOverTLS(t *testing.T) {
	h := newRouter(&fakeAuth{}, true)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, "max-age=31536000; includeSubdomains", rec.Header().Get("Strict-Transport-Security"))
}
