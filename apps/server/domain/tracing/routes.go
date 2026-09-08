package tracing

import (
	"github.com/labstack/echo/v4"

	"github.com/emergent-company/emergent.memory/pkg/auth"
)

// RegisterRoutes registers the Tempo proxy routes under /api/traces.
// All routes require authentication — Tempo is never exposed publicly.
func RegisterRoutes(e *echo.Echo, h *Handler, authMiddleware *auth.Middleware) {
	g := e.Group("/api/traces")
	g.Use(authMiddleware.RequireAuth())
	// Trace data exposes internal spans (including LLM prompts/attributes), so
	// restrict to admin read scope rather than any authenticated user.
	g.Use(authMiddleware.RequireScopes("admin:read"))
	g.GET("", h.Search)
	g.GET("/search", h.Search) // also accept /api/traces/search for explicitness
	g.GET("/:id", h.GetTrace)
}
