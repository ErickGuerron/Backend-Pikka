// Package http expone /health y /ready del servicio.
package http

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// ReadinessCheck verifica una dependencia (por ejemplo, PostgreSQL).
type ReadinessCheck func(ctx context.Context) error

// NewHealthServer crea un Echo con /health (proceso vivo) y /ready (dependencias listas).
func NewHealthServer(ready ReadinessCheck) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/ready", func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})
	return e
}
