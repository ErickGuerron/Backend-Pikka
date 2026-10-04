package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
)

const identityKey = "identity"

// TokenVerifier valida un token de acceso.
type TokenVerifier interface {
	Verify(token string) (jwt.Identity, error)
}

// Authenticate exige un Bearer token válido y guarda la identidad en el contexto.
func Authenticate(v TokenVerifier) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get(echo.HeaderAuthorization)
			scheme, token, ok := strings.Cut(header, " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing bearer token")
			}
			id, err := v.Verify(token)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
			}
			c.Set(identityKey, id)
			return next(c)
		}
	}
}

func identityFrom(c echo.Context) (jwt.Identity, bool) {
	id, ok := c.Get(identityKey).(jwt.Identity)
	return id, ok
}

// callMeta arma la metadata que se propaga a los servicios internos.
func callMeta(c echo.Context) grpcclient.CallMeta {
	m := grpcclient.CallMeta{RequestID: c.Response().Header().Get(echo.HeaderXRequestID)}
	if id, ok := identityFrom(c); ok {
		m.UserID, m.Role = id.UserID, id.Role
	}
	return m
}

// RequestLogger registra cada petición en JSON sin cuerpo ni cabecera Authorization.
func RequestLogger(log *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			if err != nil {
				c.Error(err)
			}
			attrs := []any{
				"method", c.Request().Method,
				"path", c.Path(),
				"status", c.Response().Status,
				"durationMs", time.Since(start).Milliseconds(),
				"traceId", c.Response().Header().Get(echo.HeaderXRequestID),
				"ip", c.RealIP(),
			}
			if id, ok := identityFrom(c); ok {
				attrs = append(attrs, "userId", id.UserID)
			}
			if detail, ok := c.Get("error").(string); ok {
				attrs = append(attrs, "error", detail)
			}
			level := slog.LevelInfo
			if c.Response().Status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			log.Log(c.Request().Context(), level, "http request", attrs...)
			return nil
		}
	}
}

// ContextTimeout limita el tiempo total de cada petición.
func ContextTimeout(d time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx, cancel := context.WithTimeout(c.Request().Context(), d)
			defer cancel()
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
