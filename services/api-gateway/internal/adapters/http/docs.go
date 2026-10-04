package http

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/ErickGuerron/Backend-Pikka/contracts/openapi"
)

// Versión fijada de Scalar para que la página no cambie sin un commit.
const scalarScript = "https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.72.4/dist/browser/standalone.js"

const docsPage = `<!doctype html>
<html lang="es">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Pikka API</title>
  </head>
  <body>
    <script id="api-reference" data-url="/openapi.yaml"></script>
    <script src="` + scalarScript + `"></script>
  </body>
</html>`

// La CSP global es "default-src 'none'"; la página de Scalar necesita cargar
// su script y estilos del CDN y llamar a la API desde el navegador.
const docsCSP = "default-src 'none'; " +
	"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
	"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://fonts.googleapis.com; " +
	"font-src 'self' data: https://cdn.jsdelivr.net https://fonts.gstatic.com https://fonts.scalar.com; " +
	"img-src 'self' data: https:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'"

// registerDocs publica el contrato OpenAPI y su referencia interactiva con Scalar.
func registerDocs(e *echo.Echo) {
	e.GET("/openapi.yaml", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/yaml; charset=utf-8", openapi.Spec)
	})
	e.GET("/docs", func(c echo.Context) error {
		c.Response().Header().Set("Content-Security-Policy", docsCSP)
		return c.HTML(http.StatusOK, docsPage)
	})
}
