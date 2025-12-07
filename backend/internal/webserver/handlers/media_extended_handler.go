package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type MediaExtendedHandler struct {
	tvdbClient *clients.TVDBProxyClient
	logger     *slog.Logger
}

func NewMediaExtendedHandler(tvdbClient *clients.TVDBProxyClient, logger *slog.Logger) *MediaExtendedHandler {
	return &MediaExtendedHandler{
		tvdbClient: tvdbClient,
		logger:     logger,
	}
}

func (h *MediaExtendedHandler) GetBatchExtended(c *gin.Context) {
	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	var requests []clients.ExtendedRequest
	if err := c.BindJSON(&requests); err != nil {
		h.logger.ErrorContext(ctx, "Invalid request body",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	h.logger.InfoContext(ctx, "Batch extended info request",
		telemetry.WithTraceContext(ctx, "count", len(requests))...)

	results, err := h.tvdbClient.GetExtendedBatch(ctx, requests)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch batch extended info",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch extended info from tvdb-proxy"})
		return
	}

	h.logger.InfoContext(ctx, "Returning batch extended info",
		telemetry.WithTraceContext(ctx, "count", len(results))...)
	c.JSON(http.StatusOK, results)
}
