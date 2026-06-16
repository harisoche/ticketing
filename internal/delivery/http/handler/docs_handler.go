package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"ticketing-api/docs"
)

type DocsHandler struct{}

func NewDocsHandler() *DocsHandler { return &DocsHandler{} }

// Spec serves the raw OpenAPI YAML embedded at build time.
func (h *DocsHandler) Spec(c echo.Context) error {
	return c.Blob(http.StatusOK, "application/yaml; charset=utf-8", docs.OpenAPISpec)
}

// UI serves a self-contained HTML page that loads Swagger UI from a CDN and
// renders the spec served by Spec.
func (h *DocsHandler) UI(c echo.Context) error {
	return c.HTML(http.StatusOK, swaggerHTML)
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <title>Ticketing API — Swagger UI</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
    <style>body { margin: 0; }</style>
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
      window.ui = SwaggerUIBundle({
        url: "/docs/openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis],
      });
    </script>
  </body>
</html>`
