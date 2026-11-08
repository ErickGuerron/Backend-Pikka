package http

import "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/httpx"

// crossOriginResourcePolicy añade Cross-Origin-Resource-Policy a toda respuesta (ZAP 90004).
// same-origin: ZAP pide este valor (same-site se considera menos seguro). Solo afecta a cargas no-CORS
// (img, script...) de otro origen; el front consume la API por fetch con CORS y el gateway no sirve archivos.
func crossOriginResourcePolicy(next httpx.HandlerFunc) httpx.HandlerFunc {
	return func(c *httpx.Context) error {
		c.Response().Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		return next(c)
	}
}
