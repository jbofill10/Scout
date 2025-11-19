package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/torrenter/interactors"
	media "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type StatusHandler struct {
	interactor *interactors.StatusInteractor
	logger     *slog.Logger
}

func NewStatusHandler(interactor *interactors.StatusInteractor, logger *slog.Logger) *StatusHandler {
	return &StatusHandler{
		interactor: interactor,
		logger:     logger,
	}
}

// BatchStatus handles batch status requests for multiple media items
func (h *StatusHandler) BatchStatus(c *gin.Context) {
	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	var requests []media.StatusRequest
	if err := c.ShouldBindJSON(&requests); err != nil {
		h.logger.ErrorContext(ctx, "Failed to bind batch status request", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
		return
	}

	// Validate requests
	if len(requests) == 0 {
		h.logger.WarnContext(ctx, "Empty batch status request received")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Request body cannot be empty"})
		return
	}

	h.logger.InfoContext(ctx, "Received batch status request", telemetry.WithTraceContext(ctx, "count", len(requests))...)

	response, err := h.interactor.GetBatchStatus(ctx, requests)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get batch status", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve batch status: " + err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Batch status request completed successfully",
		telemetry.WithTraceContext(ctx, "shows_count", len(response.Shows), "movies_count", len(response.Movies))...)
	c.JSON(http.StatusOK, response)
}
