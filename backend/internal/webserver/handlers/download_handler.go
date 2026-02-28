package handlers

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
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

	if req.Category != "series" {
		h.logger.ErrorContext(ctx, "Invalid media type", telemetry.WithTraceContext(ctx, "category", req.Category)...)
		c.JSON(400, gin.H{"error": "Invalid media type: expected 'series'"})
		return
	}
	if strings.TrimSpace(req.Id) == "" || strings.TrimSpace(req.Name) == "" {
		h.logger.ErrorContext(ctx, "Missing required fields", telemetry.WithTraceContext(ctx, "id", req.Id, "name", req.Name)...)
		c.JSON(400, gin.H{"error": "Missing required fields: id and mediaName are required"})
		return
	}
	if len(req.Metadata.Episodes) == 0 {
		h.logger.ErrorContext(ctx, "Missing required episodes", telemetry.WithTraceContext(ctx)...)
		c.JSON(400, gin.H{"error": "Missing required fields: metadata.episodes must contain at least one episode"})
		return
	}

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
	if strings.TrimSpace(req.Id) == "" || strings.TrimSpace(req.Name) == "" {
		h.logger.ErrorContext(ctx, "Missing required fields", telemetry.WithTraceContext(ctx, "id", req.Id, "name", req.Name)...)
		c.JSON(400, gin.H{"error": "Missing required fields: id and mediaName are required"})
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
