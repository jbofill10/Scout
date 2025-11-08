package handlers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
)

type DownloadHandler struct {
	interactor *interactors.DownloadInteractor
	logger     *slog.Logger
}

func NewDownloadHandler(interactor *interactors.DownloadInteractor, logger *slog.Logger) *DownloadHandler {
	return &DownloadHandler{
		interactor: interactor,
		logger:     logger,
	}
}

func (h *DownloadHandler) DownloadShow(c *gin.Context) {
	var req tvdb.Media
	if err := c.ShouldBindJSON(&req); err != nil {
		// Extract context for trace propagation and logging
		ctx := c.Request.Context()
		h.logger.ErrorContext(ctx, "Bad request", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(400, gin.H{"error": "Bad Request"})
		return
	}

	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	err := h.interactor.DownloadShow(ctx, req)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to download show",
			telemetry.WithTraceContext(ctx, "error", err, "show", req.Name)...)
		c.JSON(500, gin.H{"error": "Failed to download show"})
		return
	}

	h.logger.InfoContext(ctx, "Successfully processed show",
		telemetry.WithTraceContext(ctx, "show", req.Name)...)
	c.JSON(200, gin.H{})
}

func (h *DownloadHandler) DownloadMovie(c *gin.Context) {
	var req tvdb.Media
	if err := c.ShouldBindJSON(&req); err != nil {
		// Extract context for trace propagation and logging
		ctx := c.Request.Context()
		h.logger.ErrorContext(ctx, "Bad request", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(400, gin.H{"error": "Bad Request"})
		return
	}

	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	// Validate that this is actually a movie
	if req.Category != "movie" {
		h.logger.ErrorContext(ctx, "Invalid media type", telemetry.WithTraceContext(ctx, "category", req.Category)...)
		c.JSON(400, gin.H{"error": "Invalid media type: expected 'movie'"})
		return
	}

	err := h.interactor.DownloadMovie(ctx, req)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to download movie",
			telemetry.WithTraceContext(ctx, "error", err, "movie", req.Name)...)
		c.JSON(500, gin.H{"error": "Failed to download movie"})
		return
	}

	h.logger.InfoContext(ctx, "Successfully processed movie",
		telemetry.WithTraceContext(ctx, "movie", req.Name)...)
	c.JSON(200, gin.H{})
}
