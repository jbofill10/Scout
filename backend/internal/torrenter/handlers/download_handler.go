package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/torrenter/interactors"
	"github.com/jbofill10/scout/backend/internal/torrenter/models"
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

func (h *DownloadHandler) DownloadTorrent(c *gin.Context) {
	var req tvdb.Media
	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	if err := c.ShouldBind(&req); err != nil {
		h.logger.ErrorContext(ctx, "Failed to bind request", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.interactor.InitiateDownload(ctx, &req); err != nil {
		if errors.Is(err, models.ErrNoTorrentFound) {
			h.logger.WarnContext(ctx, "No torrent found for media", telemetry.WithTraceContext(ctx, "media", req.Name)...)
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to initiate download", telemetry.WithTraceContext(ctx, "error", err, "media", req.Name)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Torrent download initiated", telemetry.WithTraceContext(ctx, "media", req.Name)...)
	c.JSON(http.StatusOK, gin.H{"message": "Torrent download initiated successfully"})
}
