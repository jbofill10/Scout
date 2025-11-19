package handlers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
)

type StatusHandler struct {
	torrenterClient *clients.TorrenterClient
	logger          *slog.Logger
}

func NewStatusHandler(torrenterClient *clients.TorrenterClient, logger *slog.Logger) *StatusHandler {
	return &StatusHandler{
		torrenterClient: torrenterClient,
		logger:          logger,
	}
}

func (h *StatusHandler) GetBatchStatus(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []tvdb.StatusRequest
	if err := c.ShouldBindJSON(&requests); err != nil {
		h.logger.ErrorContext(ctx, "Bad request", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(400, gin.H{"error": "Bad Request"})
		return
	}

	// Validate that request is not empty
	if len(requests) == 0 {
		h.logger.ErrorContext(ctx, "Empty request", telemetry.WithTraceContext(ctx)...)
		c.JSON(400, gin.H{"error": "Request cannot be empty"})
		return
	}

	response, err := h.torrenterClient.GetStatusBatch(ctx, requests)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get batch status",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(500, gin.H{"error": "Failed to get batch status"})
		return
	}

	h.logger.InfoContext(ctx, "Successfully retrieved batch status",
		telemetry.WithTraceContext(ctx, "request_count", len(requests))...)
	c.JSON(200, response)
}
