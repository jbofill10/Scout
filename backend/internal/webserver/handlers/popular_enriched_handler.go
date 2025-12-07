package handlers

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type PopularEnrichedHandler struct {
	interactor *interactors.PopularEnrichedInteractor
	logger     *slog.Logger
}

func NewPopularEnrichedHandler(
	interactor *interactors.PopularEnrichedInteractor,
	logger *slog.Logger,
) *PopularEnrichedHandler {
	return &PopularEnrichedHandler{
		interactor: interactor,
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

	// Call interactor to orchestrate enrichment
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
		"Returning enriched popular shows",
		telemetry.WithTraceContext(ctx, "count", len(enrichedResults))...,
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

	// Call interactor to orchestrate enrichment
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
		"Returning enriched popular movies",
		telemetry.WithTraceContext(ctx, "count", len(enrichedResults))...,
	)
	c.JSON(200, enrichedResults)
}
