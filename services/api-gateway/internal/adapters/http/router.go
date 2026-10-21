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
	CORSAllowedOrigins  []string
	RateLimitPerSecond  float64
	RateLimitBurst      int
	LoginRatePerMinute  float64
	SignupRatePerMinute float64
	RequestTimeout      time.Duration
	BodyLimit           string
	TrustProxy          bool
	EnableHSTS          bool
	// EnableDocs publica /docs (Scalar) y /openapi.yaml.
	EnableDocs bool
	// Customers, si no es nil, recibe CreateCustomer tras un signup exitoso.
	Customers CustomerClient
	// CustomerQueries, si no es nil, habilita GET /customers y GET /customers/:id (ADMIN).
	CustomerQueries CustomerQueryClient
	// Orders, si no es nil, habilita POST /orders y GET /orders/mine para clientes.
	Orders OrderClient
	// Catalog, si no es nil, habilita /products (ADR 0009). Los roles los aplica el middleware y el servicio.
	Catalog CatalogClient
	// Drivers, si no es nil, habilita las rutas de repartidor (rol DRIVER). Hoy responden 501.
	Drivers DriverClient
	// Tracking, si no es nil, habilita PUT /drivers/me/location (rol DRIVER). Requiere Drivers.
	Tracking TrackingClient
	// Routing, if not nil, enables POST /routes/plan (ADMIN and OPERATOR).
	Routing RoutingClient
	// TrackingPollInterval es cada cuánto el WebSocket de seguimiento consulta el snapshot. Cero usa 5 s.
	TrackingPollInterval time.Duration
	// TrackingTicker crea el reloj de sondeo de cada socket. Nil usa time.NewTicker; los tests lo inyectan.
	TrackingTicker func(interval time.Duration) (<-chan time.Time, func())
}

// Readiness informa si una dependencia está lista.
type Readiness func(ctx context.Context) bool

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// NewRouter arma el Echo con middlewares, rutas y manejo de errores.
func NewRouter(log *slog.Logger, opts Options, verifier TokenVerifier, auth AuthClient, zones ZonesClient, ready Readiness) *echo.Echo {
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
	h := &authHandlers{auth: auth, customers: opts.Customers}

	loginLimit := rateLimiter(rate.Limit(opts.LoginRatePerMinute/60), max(1, int(opts.LoginRatePerMinute/2)))
	api.POST("/auth/login", h.login, loginLimit)
	// signup tiene bucket dedicado: comparte la política (mismo valor por defecto)
	// sin compartir el almacenamiento, para que el abuso de uno no vacíe el otro.
	signupPerMinute := opts.SignupRatePerMinute
	if signupPerMinute <= 0 {
		signupPerMinute = opts.LoginRatePerMinute
	}
	signupLimit := rateLimiter(rate.Limit(signupPerMinute/60), max(1, int(signupPerMinute/2)))
	api.POST("/auth/signup", h.signup, signupLimit)

	authed := api.Group("", Authenticate(verifier))
	authed.GET("/auth/me", h.me)
	authed.POST("/users", h.createUser)
	authed.GET("/users/:id", h.getUser)

	// Subgrupo adminOnly: 403 antes de tocar el handler ni el servicio interno
	// para cualquier rol distinto de ADMIN (ADR 0008 §3).
	adminOnly := authed.Group("", AdminOnly())
	adminOnly.GET("/users", h.listUsers)
	adminOnly.PATCH("/users/:id", h.updateUser)
	adminOnly.DELETE("/users/:id", h.deactivateUser)
	if opts.CustomerQueries != nil {
		ch := &customerHandlers{customers: opts.CustomerQueries}
		adminOnly.GET("/customers", ch.list)
		adminOnly.GET("/customers/:id", ch.get)
		adminOnly.PATCH("/customers/:id/status", ch.setStatus)
	}

	// Route plan generation: ADMIN and OPERATOR (ADR 0008). The Routing Service validates role and date.
	if opts.Routing != nil {
		rp := &routePlanHandlers{routing: opts.Routing}
		routeStaff := authed.Group("", StaffOnly())
		routeStaff.POST("/routes/plan", rp.generate)
	}

	// Pedidos del cliente (ADR 0007). La autorización por rol la aplica Order Service.
	if opts.Orders != nil {
		o := &ordersHandlers{orders: opts.Orders}
		authed.POST("/orders", o.create, CustomerOnly())
		authed.GET("/orders", o.list, StaffOnly())
		authed.GET("/orders/mine", o.listMine)
		authed.GET("/orders/exceptions", o.listExceptions)
		authed.POST("/orders/:id/review", o.review)
		authed.POST("/orders/:id/cancel", o.cancel)
		authed.GET("/notifications/mine", o.notifications)
	}

	// Catálogo de productos (ADR 0009). Cualquier usuario autenticado lee lo activo; el personal
	// administra. El servicio decide qué ve cada rol.
	if opts.Catalog != nil {
		p := &catalogHandlers{catalog: opts.Catalog}
		// Ruta pública para la landing: fuera del grupo authed, sin JWT. Solo lectura de productos activos.
		api.GET("/catalog/products", p.listPublic)
		staff := authed.Group("", StaffOnly())
		authed.GET("/products", p.list)
		authed.GET("/products/:id", p.get)
		staff.POST("/products", p.create)
		staff.PATCH("/products/:id/active", p.setActive)
		staff.POST("/products/:id/stock-adjustments", p.adjustStock)
		staff.GET("/products/:id/stock-movements", p.movements)
	}

	// Rutas del repartidor (ADR 0007). Solo rol DRIVER; el backend de ruta y parada aún no existe.
	if opts.Drivers != nil {
		driverAdmin := &driverAdminHandlers{auth: auth, drivers: opts.Drivers, log: log}
		adminOnly.POST("/drivers", driverAdmin.create)
		authed.GET("/drivers", driverAdmin.list, StaffOnly())
		driverOnly := authed.Group("", DriverOnly())
		if opts.Orders != nil {
			routes := &driverRouteHandlers{drivers: opts.Drivers, orders: opts.Orders}
			driverOnly.GET("/drivers/me/route", routes.get)
		} else {
			driverOnly.GET("/drivers/me/route", driverRouteNotImplemented)
		}
		if opts.Orders != nil {
			stops := &driverStopHandlers{drivers: opts.Drivers, orders: opts.Orders}
			driverOnly.PATCH("/drivers/me/routes/:routeId/stops/:stopId/status", stops.advance)
		} else {
			driverOnly.PATCH("/drivers/me/routes/:routeId/stops/:stopId/status", driverRouteNotImplemented)
		}
		driverOnly.POST("/drivers/me/routes/:routeId/stops/:stopId/result", driverRouteNotImplemented)
		if opts.Tracking != nil {
			loc := &driverLocationHandlers{drivers: opts.Drivers, tracking: opts.Tracking}
			driverOnly.PUT("/drivers/me/location", loc.report)
		}
	}

	// Seguimiento del pedido para el cliente. Necesita pedidos (ruta y paradas), repartidores (ruta ->
	// repartidor) y tracking (ETA). HTTP exige CUSTOMER por rol; el WebSocket valida el token en su
	// subprotocolo, porque el navegador no envía Authorization en un socket.
	if opts.Orders != nil && opts.Drivers != nil && opts.Tracking != nil {
		interval := opts.TrackingPollInterval
		if interval <= 0 {
			interval = 5 * time.Second
		}
		ticker := opts.TrackingTicker
		if ticker == nil {
			ticker = defaultTicker
		}
		ct := &customerTrackingHandlers{
			orders: opts.Orders, drivers: opts.Drivers, tracking: opts.Tracking,
			verifier: verifier, origins: opts.CORSAllowedOrigins,
			interval: interval, ticker: ticker, now: time.Now, log: log,
		}
		customerOnly := authed.Group("", CustomerOnly())
		customerOnly.GET("/orders/:id/tracking", ct.snapshot)
		api.GET("/orders/:id/tracking/ws", ct.ws)
	}

	z := &zonesHandlers{zones: zones}
	authed.POST("/zones", z.create)
	authed.GET("/zones", z.list)
	authed.PUT("/zones/:id", z.update)
	authed.PATCH("/zones/:id/status", z.setStatus)
	authed.DELETE("/zones/:id", z.remove)
	authed.GET("/zones/classify", z.classify)
	authed.GET("/zones/:id", z.get)
	// Cobertura: cualquier rol autenticado ve las zonas activas (ADR 0008). /zones sigue siendo de staff.
	authed.GET("/coverage/zones", z.coverage)

	// Cada Group con middleware registra su propio "ruta no encontrada" envuelto en ese middleware y
	// la última registrada gana: el grupo del repartidor atrapaba toda ruta inexistente y respondía
	// 403 "driver role required" a un ADMIN. Este registro final devuelve 404 para cualquiera.
	notFound := func(echo.Context) error { return echo.ErrNotFound }
	api.RouteNotFound("", notFound)
	api.RouteNotFound("/*", notFound)

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
