package handlers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"shared/telemetry"
	"webserver/internal/interactors"
)

type SearchHandler struct {
	interactor *interactors.SearchInteractor
	logger     *slog.Logger
}

func NewSearchHandler(interactor *interactors.SearchInteractor, logger *slog.Logger) *SearchHandler {
	return &SearchHandler{
		interactor: interactor,
		logger:     logger,
	}
}

func (h *SearchHandler) HandleSearch(c *gin.Context) {
	query := c.Query("query")
	mediaType := c.Query("media_type")

	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Search request", telemetry.WithTraceContext(ctx, "query", query, "media_type", mediaType)...)

	searchResults, err := h.interactor.Search(ctx, mediaType, query)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to search via TVDB client", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(500, gin.H{"error": "Failed to search media"})
		return
	}

	h.logger.InfoContext(ctx, "Returning search results", telemetry.WithTraceContext(ctx, "count", len(searchResults))...)
	c.JSON(200, searchResults)
}
