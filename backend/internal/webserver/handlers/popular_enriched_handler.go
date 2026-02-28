package handlers

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/cache"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type PopularEnrichedHandler struct {
	interactor *interactors.PopularEnrichedInteractor
	cache      *cache.PopularCache
	logger     *slog.Logger
}

func NewPopularEnrichedHandler(
	interactor *interactors.PopularEnrichedInteractor,
	cache *cache.PopularCache,
	logger *slog.Logger,
) *PopularEnrichedHandler {
	return &PopularEnrichedHandler{
		interactor: interactor,
		cache:      cache,
		logger:     logger,
	}
}

// HandleEnrichedPopularShows handles GET /api/popular/shows/enriched?genre=...&limit=...
// Returns popular shows enriched with episode metadata, extended info, and download status
func (h *PopularEnrichedHandler) HandleEnrichedPopularShows(c *gin.Context) {
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

	h.logger.InfoContext(
		ctx,
		"Enriched popular shows request",
		telemetry.WithTraceContext(ctx, "genre", genre, "limit", limit)...,
	)

	// Try cache first if limit matches default (20)
	if limit == 20 {
		shows, cacheHit := h.cache.GetEnrichedShows(ctx, genre)
		if cacheHit {
			h.logger.InfoContext(ctx, "Serving enriched popular shows from cache",
				telemetry.WithTraceContext(ctx, "genre", genre, "count", len(shows), "cache_hit", true)...)
			c.JSON(200, shows)
			return
		}
		h.logger.InfoContext(ctx, "Cache miss for enriched popular shows, falling back to interactor",
			telemetry.WithTraceContext(ctx, "genre", genre)...)
	}

	// Fall back to live enrichment via interactor
	enrichedResults, err := h.interactor.EnrichedPopularShows(ctx, genre, limit)
	if err != nil {
		h.logger.ErrorContext(
			ctx,
			"Enriched popular shows failed",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		c.JSON(500, gin.H{"error": "Failed to fetch enriched popular shows"})
		return
	}

	h.logger.InfoContext(
		ctx,
		"Returning enriched popular shows from interactor",
		telemetry.WithTraceContext(ctx, "count", len(enrichedResults), "cache_hit", false)...,
	)
	c.JSON(200, enrichedResults)
}

// HandleEnrichedPopularMovies handles GET /api/popular/movies/enriched?genre=...&limit=...
// Returns popular movies enriched with extended info and download status
func (h *PopularEnrichedHandler) HandleEnrichedPopularMovies(c *gin.Context) {
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

	h.logger.InfoContext(
		ctx,
		"Enriched popular movies request",
		telemetry.WithTraceContext(ctx, "genre", genre, "limit", limit)...,
	)

	// Try cache first if limit matches default (20)
	if limit == 20 {
		movies, cacheHit := h.cache.GetEnrichedMovies(ctx, genre)
		if cacheHit {
			h.logger.InfoContext(ctx, "Serving enriched popular movies from cache",
				telemetry.WithTraceContext(ctx, "genre", genre, "count", len(movies), "cache_hit", true)...)
			c.JSON(200, movies)
			return
		}
		h.logger.InfoContext(ctx, "Cache miss for enriched popular movies, falling back to interactor",
			telemetry.WithTraceContext(ctx, "genre", genre)...)
	}

	// Fall back to live enrichment via interactor
	enrichedResults, err := h.interactor.EnrichedPopularMovies(ctx, genre, limit)
	if err != nil {
		h.logger.ErrorContext(
			ctx,
			"Enriched popular movies failed",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		c.JSON(500, gin.H{"error": "Failed to fetch enriched popular movies"})
		return
	}

	h.logger.InfoContext(
		ctx,
		"Returning enriched popular movies from interactor",
		telemetry.WithTraceContext(ctx, "count", len(enrichedResults), "cache_hit", false)...,
	)
	c.JSON(200, enrichedResults)
}
