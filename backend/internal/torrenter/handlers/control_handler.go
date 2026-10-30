package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jbofill10/scout/backend/internal/torrenter/service"
	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// controlService is what the handler needs from the download interactor,
// narrowed so tests can stand in for it.
type controlService interface {
	Cancel(ctx context.Context, itemID string) (controls.CancelResponse, error)
	ListReleases(ctx context.Context, req controls.ReleasesRequest) (controls.ReleasesResponse, error)
	ManualDownload(ctx context.Context, req controls.ManualDownloadRequest) (dlstatus.EpisodeResult, error)
}

// ControlHandler serves the download control endpoints: cancel, list
// releases, manual download.
type ControlHandler struct {
	interactor controlService
	logger     *slog.Logger
}

func NewControlHandler(interactor controlService, logger *slog.Logger) *ControlHandler {
	return &ControlHandler{interactor: interactor, logger: logger}
}

// Cancel stops everything in flight for an item. 200 with what was found.
func (h *ControlHandler) Cancel(c *gin.Context) {
	ctx := c.Request.Context()
	var req controls.CancelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "item_id is required"})
		return
	}

	resp, err := h.interactor.Cancel(ctx, req.ItemID)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to cancel item", telemetry.WithTraceContext(ctx, "error", err, "tvdb_id", req.ItemID)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// ListReleases returns the candidate releases for an item. 404 when the
// torrenter has no media for it and none was posted.
func (h *ControlHandler) ListReleases(c *gin.Context) {
	ctx := c.Request.Context()
	var req controls.ReleasesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "item_id is required"})
		return
	}

	resp, err := h.interactor.ListReleases(ctx, req)
	if err != nil {
		if errors.Is(err, service.ErrUnknownItem) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no media known for this item"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to list releases", telemetry.WithTraceContext(ctx, "error", err, "tvdb_id", req.ItemID)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// ManualDownload downloads the release the user chose. The result is the same
// per-item shape the automatic path returns.
func (h *ControlHandler) ManualDownload(c *gin.Context) {
	ctx := c.Request.Context()
	var req controls.ManualDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "item_id is required"})
		return
	}
	if !req.Release.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "release needs a title and a guid or download_url"})
		return
	}

	result, err := h.interactor.ManualDownload(ctx, req)
	if err != nil {
		if errors.Is(err, service.ErrUnknownItem) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no media known for this item"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to download chosen release",
			telemetry.WithTraceContext(ctx, "error", err, "tvdb_id", req.ItemID, "torrent_title", req.Release.Title)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Manual download processed",
		telemetry.WithTraceContext(ctx, "tvdb_id", req.ItemID, "outcome", result.Outcome, "code", string(result.Code))...)
	c.JSON(http.StatusOK, result)
}
