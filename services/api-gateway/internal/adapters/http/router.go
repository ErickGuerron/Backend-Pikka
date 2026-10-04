// Package http expone la API pública REST/JSON del sistema bajo /api/v1.
package http

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// Options configura las políticas HTTP transversales.
type Options struct {
	CORSAllowedOrigins []string
	RateLimitPerSecond float64
	RateLimitBurst     int
	LoginRatePerMinute float64
	RequestTimeout     time.Duration
	BodyLimit          string
	TrustProxy         bool
	EnableHSTS         bool
	// EnableDocs publica /docs (Scalar) y /openapi.yaml.
	EnableDocs bool
}

// Readiness informa si una dependencia está lista.
type Readiness func(ctx context.Context) bool

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// NewRouter arma el Echo con middlewares, rutas y manejo de errores.
func NewRouter(log *slog.Logger, opts Options, verifier TokenVerifier, auth AuthClient, ready Readiness) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = ErrorHandler
	if opts.TrustProxy {
		e.IPExtractor = echo.ExtractIPFromXFFHeader()
	} else {
		e.IPExtractor = echo.ExtractIPDirect()
	}

	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		Generator: uuid.NewString,
		Skipper: func(c echo.Context) bool {
			// Reutiliza el id entrante solo si es seguro registrarlo en logs.
			if rid := c.Request().Header.Get(echo.HeaderXRequestID); safeRequestID.MatchString(rid) {
				c.Response().Header().Set(echo.HeaderXRequestID, rid)
				return true
			}
			c.Request().Header.Del(echo.HeaderXRequestID)
			return false
		},
	}))
	e.Use(RequestLogger(log))
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{DisableStackAll: true}))
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "0",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            hstsMaxAge(opts.EnableHSTS),
		ContentSecurityPolicy: "default-src 'none'; frame-ancestors 'none'",
		ReferrerPolicy:        "no-referrer",
	}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:  opts.CORSAllowedOrigins,
		AllowMethods:  []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:  []string{echo.HeaderAuthorization, echo.HeaderContentType, echo.HeaderXRequestID, "Idempotency-Key"},
		ExposeHeaders: []string{echo.HeaderXRequestID},
		MaxAge:        600,
	}))
	e.Use(middleware.BodyLimit(opts.BodyLimit))
	e.Use(ContextTimeout(opts.RequestTimeout))

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/ready", func(c echo.Context) error {
		if !ready(c.Request().Context()) {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})

	if opts.EnableDocs {
		registerDocs(e)
	}

	api := e.Group("/api/v1", rateLimiter(rate.Limit(opts.RateLimitPerSecond), opts.RateLimitBurst))
	h := &authHandlers{auth: auth}

	loginLimit := rateLimiter(rate.Limit(opts.LoginRatePerMinute/60), max(1, int(opts.LoginRatePerMinute/2)))
	api.POST("/auth/login", h.login, loginLimit)

	authed := api.Group("", Authenticate(verifier))
	authed.GET("/auth/me", h.me)
	authed.POST("/users", h.createUser)
	authed.GET("/users/:id", h.getUser)

	return e
}

func hstsMaxAge(enabled bool) int {
	if enabled {
		return 31536000
	}
	return 0
}

// rateLimiter limita por IP en memoria. Con varias réplicas del Gateway se
// moverá a Redis (uso permitido por la sección 27 de la base técnica).
func rateLimiter(r rate.Limit, burst int) echo.MiddlewareFunc {
	store := middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate: r, Burst: burst, ExpiresIn: 3 * time.Minute,
	})
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: store,
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		DenyHandler: func(_ echo.Context, _ string, _ error) error {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many requests")
		},
		ErrorHandler: func(_ echo.Context, _ error) error {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many requests")
		},
	})
}
