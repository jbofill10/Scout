package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type TVDBHandler struct {
	tvdbClient *clients.TVDBProxyClient
	logger     *slog.Logger
}

func NewTVDBHandler(tvdbClient *clients.TVDBProxyClient, logger *slog.Logger) *TVDBHandler {
	return &TVDBHandler{
		tvdbClient: tvdbClient,
		logger:     logger,
	}
}

// GetEpisodesBatch handles batch episode metadata requests from torrenter
// POST /tvdb/batch/episodes
func (h *TVDBHandler) GetEpisodesBatch(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []clients.EpisodesRequest
	if err := c.ShouldBindJSON(&requests); err != nil {
		h.logger.ErrorContext(ctx, "Failed to bind episodes batch request",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Handling episodes batch request",
		telemetry.WithTraceContext(ctx, "request_count", len(requests))...)

	// Call tvdb-proxy client to fetch episodes
	responses, err := h.tvdbClient.GetEpisodesBatch(ctx, requests)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch episodes batch",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch episodes: " + err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Episodes batch request completed successfully",
		telemetry.WithTraceContext(ctx, "response_count", len(responses))...)

	c.JSON(http.StatusOK, responses)
}
