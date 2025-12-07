package handlers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type EnrichedSearchHandler struct {
	interactor *interactors.EnrichedSearchInteractor
	logger     *slog.Logger
}

func NewEnrichedSearchHandler(interactor *interactors.EnrichedSearchInteractor, logger *slog.Logger) *EnrichedSearchHandler {
	return &EnrichedSearchHandler{
		interactor: interactor,
		logger:     logger,
	}
}

// HandleEnrichedSearch handles GET /api/search/enriched?query=...&media_type=...
// Returns search results enriched with download status information
func (h *EnrichedSearchHandler) HandleEnrichedSearch(c *gin.Context) {
	query := c.Query("query")
	mediaType := c.Query("media_type")

	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	h.logger.InfoContext(
		ctx,
		"Enriched search request",
		telemetry.WithTraceContext(ctx, "query", query, "media_type", mediaType)...,
	)

	// Validate required parameters
	if query == "" {
		h.logger.WarnContext(ctx, "Missing query parameter", telemetry.WithTraceContext(ctx)...)
		c.JSON(400, gin.H{"error": "query parameter is required"})
		return
	}

	if mediaType == "" {
		h.logger.WarnContext(ctx, "Missing media_type parameter", telemetry.WithTraceContext(ctx)...)
		c.JSON(400, gin.H{"error": "media_type parameter is required"})
		return
	}

	// Call interactor to orchestrate enriched search
	enrichedResults, err := h.interactor.EnrichedSearch(ctx, mediaType, query)
	if err != nil {
		h.logger.ErrorContext(
			ctx,
			"Enriched search failed",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		c.JSON(500, gin.H{"error": "Failed to perform enriched search"})
		return
	}

	h.logger.InfoContext(
		ctx,
		"Returning enriched search results",
		telemetry.WithTraceContext(ctx, "count", len(enrichedResults))...,
	)
	c.JSON(200, enrichedResults)
}
