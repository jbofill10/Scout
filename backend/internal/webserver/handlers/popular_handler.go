package handlers

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
)

type PopularHandler struct {
	tvdbClient *clients.TVDBProxyClient
	logger     *slog.Logger
}

func NewPopularHandler(tvdbClient *clients.TVDBProxyClient, logger *slog.Logger) *PopularHandler {
	return &PopularHandler{
		tvdbClient: tvdbClient,
		logger:     logger,
	}
}

func (h *PopularHandler) GetPopularShows(c *gin.Context) {
	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	genre := c.Query("genre")
	limitStr := c.Query("limit")

	// Parse limit with default value
	limit := 20
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			h.logger.ErrorContext(ctx, "Invalid limit parameter",
				telemetry.WithTraceContext(ctx, "error", err, "limit", limitStr)...)
			c.JSON(400, gin.H{"error": "Invalid limit parameter"})
			return
		}
		limit = parsedLimit
	}

	h.logger.InfoContext(ctx, "Popular shows request",
		telemetry.WithTraceContext(ctx, "genre", genre, "limit", limit)...)

	shows, err := h.tvdbClient.GetPopularShows(ctx, genre, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch popular shows",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(502, gin.H{"error": "Failed to fetch popular shows from tvdb-proxy"})
		return
	}

	h.logger.InfoContext(ctx, "Returning popular shows",
		telemetry.WithTraceContext(ctx, "count", len(shows))...)
	c.JSON(200, shows)
}

func (h *PopularHandler) GetPopularMovies(c *gin.Context) {
	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	genre := c.Query("genre")
	limitStr := c.Query("limit")

	// Parse limit with default value
	limit := 20
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			h.logger.ErrorContext(ctx, "Invalid limit parameter",
				telemetry.WithTraceContext(ctx, "error", err, "limit", limitStr)...)
			c.JSON(400, gin.H{"error": "Invalid limit parameter"})
			return
		}
		limit = parsedLimit
	}

	h.logger.InfoContext(ctx, "Popular movies request",
		telemetry.WithTraceContext(ctx, "genre", genre, "limit", limit)...)

	movies, err := h.tvdbClient.GetPopularMovies(ctx, genre, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch popular movies",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(502, gin.H{"error": "Failed to fetch popular movies from tvdb-proxy"})
		return
	}

	h.logger.InfoContext(ctx, "Returning popular movies",
		telemetry.WithTraceContext(ctx, "count", len(movies))...)
	c.JSON(200, movies)
}

func (h *PopularHandler) GetGenres(c *gin.Context) {
	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Genres request", telemetry.WithTraceContext(ctx)...)

	genres, err := h.tvdbClient.GetGenres(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch genres",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(502, gin.H{"error": "Failed to fetch genres from tvdb-proxy"})
		return
	}

	h.logger.InfoContext(ctx, "Returning genres",
		telemetry.WithTraceContext(ctx, "count", len(genres))...)
	c.JSON(200, genres)
}
