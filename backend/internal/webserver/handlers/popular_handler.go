package handlers

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/cache"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type PopularHandler struct {
	tvdbClient *clients.TVDBProxyClient
	cache      *cache.PopularCache
	logger     *slog.Logger
}

func NewPopularHandler(
	tvdbClient *clients.TVDBProxyClient,
	cache *cache.PopularCache,
	logger *slog.Logger,
) *PopularHandler {
	return &PopularHandler{
		tvdbClient: tvdbClient,
		cache:      cache,
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

	// Try cache first if limit matches default (20)
	if limit == 20 {
		shows, cacheHit := h.cache.GetBasicShows(ctx, genre)
		if cacheHit {
			h.logger.InfoContext(ctx, "Serving popular shows from cache",
				telemetry.WithTraceContext(ctx, "genre", genre, "count", len(shows), "cache_hit", true)...)
			c.JSON(200, shows)
			return
		}
		h.logger.InfoContext(ctx, "Cache miss for popular shows, falling back to API",
			telemetry.WithTraceContext(ctx, "genre", genre)...)
	}

	// Fall back to live API call
	shows, err := h.tvdbClient.GetPopularShows(ctx, genre, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch popular shows",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(502, gin.H{"error": "Failed to fetch popular shows from tvdb-proxy"})
		return
	}

	h.logger.InfoContext(ctx, "Returning popular shows from API",
		telemetry.WithTraceContext(ctx, "count", len(shows), "cache_hit", false)...)
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

	// Try cache first if limit matches default (20)
	if limit == 20 {
		movies, cacheHit := h.cache.GetBasicMovies(ctx, genre)
		if cacheHit {
			h.logger.InfoContext(ctx, "Serving popular movies from cache",
				telemetry.WithTraceContext(ctx, "genre", genre, "count", len(movies), "cache_hit", true)...)
			c.JSON(200, movies)
			return
		}
		h.logger.InfoContext(ctx, "Cache miss for popular movies, falling back to API",
			telemetry.WithTraceContext(ctx, "genre", genre)...)
	}

	// Fall back to live API call
	movies, err := h.tvdbClient.GetPopularMovies(ctx, genre, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch popular movies",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(502, gin.H{"error": "Failed to fetch popular movies from tvdb-proxy"})
		return
	}

	h.logger.InfoContext(ctx, "Returning popular movies from API",
		telemetry.WithTraceContext(ctx, "count", len(movies), "cache_hit", false)...)
	c.JSON(200, movies)
}

func (h *PopularHandler) GetGenres(c *gin.Context) {
	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Genres request", telemetry.WithTraceContext(ctx)...)

	// Try cache first
	genres, cacheHit := h.cache.GetGenres(ctx)
	if cacheHit {
		h.logger.InfoContext(ctx, "Serving genres from cache",
			telemetry.WithTraceContext(ctx, "count", len(genres), "cache_hit", true)...)
		c.JSON(200, genres)
		return
	}

	h.logger.InfoContext(ctx, "Cache miss for genres, falling back to API",
		telemetry.WithTraceContext(ctx)...)

	// Fall back to live API call
	genres, err := h.tvdbClient.GetGenres(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch genres",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(502, gin.H{"error": "Failed to fetch genres from tvdb-proxy"})
		return
	}

	h.logger.InfoContext(ctx, "Returning genres from API",
		telemetry.WithTraceContext(ctx, "count", len(genres), "cache_hit", false)...)
	c.JSON(200, genres)
}
