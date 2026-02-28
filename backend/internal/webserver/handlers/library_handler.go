package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type LibraryHandler struct {
	torrenterClient *clients.TorrenterClient
	logger          *slog.Logger
}

func NewLibraryHandler(torrenterClient *clients.TorrenterClient, logger *slog.Logger) *LibraryHandler {
	return &LibraryHandler{
		torrenterClient: torrenterClient,
		logger:          logger,
	}
}

// GetShows proxies the request to torrenter to retrieve all library shows
// GET /api/library/shows
func (h *LibraryHandler) GetShows(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Proxying library shows request to torrenter")

	shows, err := h.torrenterClient.GetLibraryShows(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get library shows",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve library shows"})
		return
	}

	h.logger.InfoContext(ctx, "Successfully retrieved library shows",
		telemetry.WithTraceContext(ctx, "count", len(shows))...)
	c.JSON(http.StatusOK, shows)
}

// GetShowDetails proxies the request to torrenter to retrieve show episodes
// GET /api/library/shows/:tvdbId
func (h *LibraryHandler) GetShowDetails(c *gin.Context) {
	ctx := c.Request.Context()
	tvdbId := c.Param("tvdbId")

	if tvdbId == "" {
		h.logger.WarnContext(ctx, "Missing tvdbId parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "tvdbId parameter is required"})
		return
	}

	h.logger.InfoContext(ctx, "Proxying show episodes request to torrenter",
		telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId)...)

	episodes, err := h.torrenterClient.GetShowEpisodes(ctx, tvdbId)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get show episodes",
			telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve show episodes"})
		return
	}

	h.logger.InfoContext(ctx, "Successfully retrieved show episodes",
		telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId, "count", len(episodes))...)
	c.JSON(http.StatusOK, episodes)
}

// GetShowMetadataStatus proxies the request to torrenter to retrieve TVDB sync status
// GET /api/library/shows/:tvdbId/metadata-status
func (h *LibraryHandler) GetShowMetadataStatus(c *gin.Context) {
	ctx := c.Request.Context()
	tvdbId := c.Param("tvdbId")

	if tvdbId == "" {
		h.logger.WarnContext(ctx, "Missing tvdbId parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "tvdbId parameter is required"})
		return
	}

	h.logger.InfoContext(ctx, "Proxying metadata status request to torrenter",
		telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId)...)

	status, err := h.torrenterClient.GetShowMetadataStatus(ctx, tvdbId)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get metadata status",
			telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve metadata status"})
		return
	}

	h.logger.InfoContext(ctx, "Successfully retrieved metadata status",
		telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId)...)
	c.JSON(http.StatusOK, status)
}

// GetMovies proxies the request to torrenter to retrieve all library movies
// GET /api/library/movies
func (h *LibraryHandler) GetMovies(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Proxying library movies request to torrenter")

	movies, err := h.torrenterClient.GetLibraryMovies(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get library movies",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve library movies"})
		return
	}

	h.logger.InfoContext(ctx, "Successfully retrieved library movies",
		telemetry.WithTraceContext(ctx, "count", len(movies))...)
	c.JSON(http.StatusOK, movies)
}

// ProxyPlexThumb proxies Plex thumbnail requests to torrenter
// GET /api/plex/thumb?path=/library/metadata/123/thumb/456
func (h *LibraryHandler) ProxyPlexThumb(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path parameter is required"})
		return
	}

	// Forward to torrenter's Plex proxy
	url := "http://" + h.torrenterClient.Host + "/plex/thumb?path=" + path
	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request"})
		return
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to proxy thumbnail"})
		return
	}
	defer resp.Body.Close()

	// Copy response headers and body
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}
	c.Status(resp.StatusCode)

	// Stream the response body directly (not using c.Stream for static content)
	// Ignore errors - once headers are sent, we cannot send error response to client
	io.Copy(c.Writer, resp.Body) //nolint:errcheck
}
