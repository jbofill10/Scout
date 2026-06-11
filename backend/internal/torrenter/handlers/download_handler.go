package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/torrenter/interactors"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
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

	results, err := h.interactor.InitiateDownload(ctx, &req)
	if err != nil {
		// Total infrastructure failure: processing could not even start.
		h.logger.ErrorContext(ctx, "Failed to initiate download", telemetry.WithTraceContext(ctx, "error", err, "media", req.Name)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Torrent download processed", telemetry.WithTraceContext(ctx, "media", req.Name, "result_count", len(results))...)
	c.JSON(http.StatusOK, dlstatus.DownloadResponse{Results: results})
}
