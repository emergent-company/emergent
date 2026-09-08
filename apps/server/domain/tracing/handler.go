package tracing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/emergent-company/emergent.memory/internal/config"
)

// Handler proxies Tempo query API requests so clients never talk to Tempo directly.
type Handler struct {
	tempoBaseURL string
	client       *http.Client
}

// NewHandler creates a tracing handler. When tracing is disabled the handler
// still registers routes but returns 503 for all requests.
func NewHandler(cfg *config.Config) *Handler {
	tempoBase := ""
	if cfg.Otel.Enabled() {
		// Derive the internal Tempo query URL from the exporter endpoint:
		// replace the ingest port (4318) with the query port (3200), and use
		// the service hostname (tempo) that is reachable inside the Docker network.
		tempoBase = cfg.Otel.InternalTempoQueryURL()
	}
	return &Handler{
		tempoBaseURL: tempoBase,
		client:       &http.Client{},
	}
}

// Search proxies GET /api/search to Tempo with all query params forwarded.
// Corresponds to Tempo's trace search API.
//
// @Summary      Search traces
// @Description  Proxies Tempo's trace search API, returning recent traces matching optional filters. Returns 503 when tracing is not enabled.
// @Tags         tracing
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        limit         query string false "Maximum number of traces to return"
// @Param        service_name  query string false "Filter by service name"
// @Param        tags          query string false "Filter by tags (key=value, comma-separated)"
// @Param        min_duration  query string false "Minimum trace duration (e.g. '100ms', '1s')"
// @Param        start         query string false "Start time for the search window (RFC3339)"
// @Param        end           query string false "End time for the search window (RFC3339)"
// @Param        project_id    query string false "Scope search to a project by ID (sets a TraceQL project filter)"
// @Success      200 {object} map[string]any "Trace search results (Tempo passthrough)"
// @Failure      401 {object} map[string]any "Unauthorized"
// @Failure      403 {object} map[string]any "Insufficient permissions"
// @Failure      503 {object} map[string]any "Tracing not enabled"
// @Router       /api/traces [get]
// @Router       /api/traces/search [get]
func (h *Handler) Search(c echo.Context) error {
	if h.tempoBaseURL == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "tracing not enabled")
	}
	params := c.QueryParams()
	if projectID := c.QueryParam("project_id"); projectID != "" && params.Get("q") == "" {
		// Project scoping only applies when no explicit TraceQL query is given;
		// when both are present q wins, so do not merge.
		params.Set("q", fmt.Sprintf(`{ .memory.project.id = "%s" || .emergent.project.id = "%s" }`, projectID, projectID))
	}
	return h.proxy(c, "/api/search", params)
}

// GetTrace proxies GET /api/traces/:id to Tempo.
//
// @Summary      Get trace by ID
// @Description  Proxies Tempo's trace retrieval API, returning the full span tree for a trace. Returns 503 when tracing is not enabled.
// @Tags         tracing
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id path string true "Trace ID"
// @Param        format        query string false "Response format: 'structured' returns a normalized span list instead of raw OTLP"
// @Success      200 {object} map[string]any "Full span tree (Tempo passthrough)"
// @Failure      401 {object} map[string]any "Unauthorized"
// @Failure      403 {object} map[string]any "Insufficient permissions"
// @Failure      503 {object} map[string]any "Tracing not enabled"
// @Router       /api/traces/{id} [get]
func (h *Handler) GetTrace(c echo.Context) error {
	if h.tempoBaseURL == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "tracing not enabled")
	}

	if c.QueryParam("format") == "structured" {
		path := "/api/traces/" + c.Param("id")
		resp, err := h.tempoGet(c, path)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read tempo response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return echo.NewHTTPError(resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var otlpResp otlpTraceResponse
		if err := json.Unmarshal(body, &otlpResp); err != nil {
			return fmt.Errorf("decode tempo response: %w", err)
		}
		return c.JSON(http.StatusOK, toStructuredTrace(&otlpResp))
	}

	return h.proxy(c, "/api/traces/"+c.Param("id"), nil)
}

// tempoGet performs a GET against Tempo and returns the raw response. It
// mirrors proxy's request-building and error handling. The caller must close
// the returned response body.
func (h *Handler) tempoGet(c echo.Context, path string) (*http.Response, error) {
	target := h.tempoBaseURL + path

	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build tempo request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("tempo unreachable: %s", err))
	}
	return resp, nil
}

// proxy forwards the request to Tempo and streams the response back.
func (h *Handler) proxy(c echo.Context, path string, params url.Values) error {
	target := path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}

	resp, err := h.tempoGet(c, target)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.Response().Header().Set(echo.HeaderContentType, resp.Header.Get(echo.HeaderContentType))
	c.Response().WriteHeader(resp.StatusCode)
	_, err = io.Copy(c.Response(), resp.Body)
	return err
}
