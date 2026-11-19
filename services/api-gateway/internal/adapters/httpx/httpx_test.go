package httpx_test

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/httpx"
)

func send(e *httpx.Engine, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func text(body string) httpx.HandlerFunc {
	return func(c *httpx.Context) error { return c.Blob(http.StatusOK, "text/plain", []byte(body)) }
}

func TestRouter_StaticSegmentBeatsParam(t *testing.T) {
	e := httpx.New()
	e.GET("/api/v1/customers/:id", text("param"))
	e.GET("/api/v1/customers/recidivists", text("static"))
	assert.Equal(t, "static", send(e, http.MethodGet, "/api/v1/customers/recidivists", nil).Body.String())
	assert.Equal(t, "param", send(e, http.MethodGet, "/api/v1/customers/42", nil).Body.String())
}

func TestRouter_ParamIsExposed(t *testing.T) {
	e := httpx.New()
	e.GET("/orders/:id/lines/:line", func(c *httpx.Context) error {
		return c.Blob(http.StatusOK, "text/plain", []byte(c.Param("id")+"|"+c.Param("line")+"|"+c.Path()))
	})
	rec := send(e, http.MethodGet, "/orders/o-9/lines/3", nil)
	assert.Equal(t, "o-9|3|/orders/:id/lines/:line", rec.Body.String())
}

func TestRouter_MethodMismatchIs404(t *testing.T) {
	e := httpx.New()
	e.POST("/login", text("ok"))
	assert.Equal(t, http.StatusNotFound, send(e, http.MethodGet, "/login", nil).Code)
}

func TestRouter_EmptyParamSegmentDoesNotMatch(t *testing.T) {
	e := httpx.New()
	e.GET("/items/:id", text("ok"))
	assert.Equal(t, http.StatusNotFound, send(e, http.MethodGet, "/items/", nil).Code)
}

func TestRouter_GroupMiddlewareOrderAndScope(t *testing.T) {
	var trace []string
	mark := func(name string) httpx.MiddlewareFunc {
		return func(next httpx.HandlerFunc) httpx.HandlerFunc {
			return func(c *httpx.Context) error {
				trace = append(trace, name)
				return next(c)
			}
		}
	}
	e := httpx.New()
	api := e.Group("/api/v1", mark("api"))
	authed := api.Group("", mark("auth"))
	authed.GET("/me", text("me"), mark("route"))
	api.GET("/public", text("public"))

	send(e, http.MethodGet, "/api/v1/me", nil)
	assert.Equal(t, []string{"api", "auth", "route"}, trace)

	trace = nil
	send(e, http.MethodGet, "/api/v1/public", nil)
	assert.Equal(t, []string{"api"}, trace, "auth middleware must not leak into sibling routes")
}

func TestRouter_GlobalMiddlewareRunsForUnmatched(t *testing.T) {
	e := httpx.New()
	e.Use(httpx.Secure(httpx.SecureConfig{ContentTypeNosniff: "nosniff"}))
	rec := send(e, http.MethodGet, "/nothing", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
}

func TestRouter_HandlerErrorGoesToErrorHandler(t *testing.T) {
	e := httpx.New()
	e.HTTPErrorHandler = func(err error, c *httpx.Context) {
		var he *httpx.HTTPError
		if errors.As(err, &he) {
			_ = c.NoContent(he.Code)
		}
	}
	e.GET("/x", func(*httpx.Context) error { return httpx.NewHTTPError(http.StatusTeapot) })
	assert.Equal(t, http.StatusTeapot, send(e, http.MethodGet, "/x", nil).Code)
}

func TestRecover_PanicBecomesError(t *testing.T) {
	e := httpx.New()
	var got error
	e.HTTPErrorHandler = func(err error, c *httpx.Context) {
		got = err
		_ = c.NoContent(http.StatusInternalServerError)
	}
	e.Use(httpx.Recover())
	e.GET("/boom", func(*httpx.Context) error { panic("kaboom") })
	rec := send(e, http.MethodGet, "/boom", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Error(t, got)
	assert.Contains(t, got.Error(), "kaboom")
}

func TestCORS_PreflightAllowedAndDenied(t *testing.T) {
	e := httpx.New()
	e.Use(httpx.CORS(httpx.CORSConfig{AllowOrigins: []string{"http://ok"}, AllowMethods: []string{"GET", "POST"}, MaxAge: 600}))
	e.POST("/x", text("ok"))

	allowed := send(e, http.MethodOptions, "/x", map[string]string{"Origin": "http://ok", "Access-Control-Request-Method": "POST"})
	assert.Equal(t, http.StatusNoContent, allowed.Code)
	assert.Equal(t, "http://ok", allowed.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET,POST", allowed.Header().Get("Access-Control-Allow-Methods"))

	denied := send(e, http.MethodOptions, "/x", map[string]string{"Origin": "http://evil", "Access-Control-Request-Method": "POST"})
	assert.Equal(t, http.StatusNoContent, denied.Code)
	assert.Empty(t, denied.Header().Get("Access-Control-Allow-Origin"))
}

func TestBodyLimit_ParsesSuffixes(t *testing.T) {
	e := httpx.New()
	e.POST("/b", text("ok"), httpx.BodyLimit("1K"))
	req := httptest.NewRequest(http.MethodPost, "/b", strings.NewReader(strings.Repeat("x", 1025)))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/b", strings.NewReader(strings.Repeat("x", 1024)))
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBodyLimit_InvalidPanics(t *testing.T) {
	assert.Panics(t, func() { httpx.BodyLimit("lots") })
}

func TestRealIP_DirectIgnoresHeaders(t *testing.T) {
	e := httpx.New()
	e.GET("/ip", func(c *httpx.Context) error { return c.Blob(http.StatusOK, "text/plain", []byte(c.RealIP())) })
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.7:5555"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, "203.0.113.7", rec.Body.String())
}

func TestRealIP_XFFSkipsTrustedProxies(t *testing.T) {
	e := httpx.New()
	e.IPExtractor = httpx.ExtractIPFromXFFHeader()
	e.GET("/ip", func(c *httpx.Context) error { return c.Blob(http.StatusOK, "text/plain", []byte(c.RealIP())) })
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.0.0.1:1000"
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.0.0.2")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, "198.51.100.9", rec.Body.String())
}

func TestRealIP_XFFMalformedFallsBackToPeer(t *testing.T) {
	e := httpx.New()
	e.IPExtractor = httpx.ExtractIPFromXFFHeader()
	e.GET("/ip", func(c *httpx.Context) error { return c.Blob(http.StatusOK, "text/plain", []byte(c.RealIP())) })
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.0.0.1:1000"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, "10.0.0.1", rec.Body.String())
}

func TestRateLimiter_DeniesAfterBurst(t *testing.T) {
	e := httpx.New()
	// expiresIn must exceed the test duration: idle buckets are discarded, and a zero value would
	// reset the budget on every request.
	store := httpx.NewMemoryRateLimiter(rate.Limit(0.0001), 2, time.Minute)
	e.GET("/r", text("ok"), httpx.RateLimiter(store))
	assert.Equal(t, http.StatusOK, send(e, http.MethodGet, "/r", nil).Code)
	assert.Equal(t, http.StatusOK, send(e, http.MethodGet, "/r", nil).Code)
	assert.Equal(t, http.StatusTooManyRequests, send(e, http.MethodGet, "/r", nil).Code)
}

func TestSecure_HSTSOnlyOverTLS(t *testing.T) {
	e := httpx.New()
	e.Use(httpx.Secure(httpx.SecureConfig{HSTSMaxAge: 60}))
	e.GET("/s", text("ok"))
	assert.Empty(t, send(e, http.MethodGet, "/s", nil).Header().Get("Strict-Transport-Security"))

	req := httptest.NewRequest(http.MethodGet, "/s", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, "max-age=60; includeSubdomains", rec.Header().Get("Strict-Transport-Security"))
}
