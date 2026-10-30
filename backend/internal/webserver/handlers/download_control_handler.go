package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// downloadControls is what the handler needs from the control interactor.
type downloadControls interface {
	Cancel(ctx context.Context, itemKey string) (interactors.CancelOutcome, error)
	RetryNow(ctx context.Context, itemKey string) (interactors.RetryOutcome, error)
	ListReleases(ctx context.Context, itemKey string) (controls.ReleasesResponse, error)
	Grab(ctx context.Context, itemKey string, sel controls.ReleaseSelection) (dlstatus.EpisodeResult, error)
	Upcoming(ctx context.Context) ([]repository.UpcomingItem, error)
}

// DownloadControlHandler serves the per-item download controls and the
// upcoming list. Every item route takes the notification tvdb id as :id.
type DownloadControlHandler struct {
	interactor downloadControls
	logger     *slog.Logger
}

func NewDownloadControlHandler(interactor downloadControls, logger *slog.Logger) *DownloadControlHandler {
	return &DownloadControlHandler{interactor: interactor, logger: logger}
}

// Cancel: 200 with what was stopped, 404 when nothing exists for the item.
func (h *DownloadControlHandler) Cancel(c *gin.Context) {
	ctx := c.Request.Context()
	itemKey := c.Param("id")

	out, err := h.interactor.Cancel(ctx, itemKey)
	if err != nil {
		if errors.Is(err, interactors.ErrNothingToControl) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Nothing to cancel for this item"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to cancel item", telemetry.WithTraceContext(ctx, "error", err, "tvdb_id", itemKey)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel"})
		return
	}
	c.JSON(http.StatusOK, out)
}

// RetryNow: 200 with the rows re-armed, 404 without a row, 409 mid-dispatch.
func (h *DownloadControlHandler) RetryNow(c *gin.Context) {
	ctx := c.Request.Context()
	itemKey := c.Param("id")

	out, err := h.interactor.RetryNow(ctx, itemKey)
	if err != nil {
		switch {
		case errors.Is(err, interactors.ErrNothingToControl):
			c.JSON(http.StatusNotFound, gin.H{"error": "Nothing to retry for this item"})
		case errors.Is(err, interactors.ErrDispatchInProgress):
			c.JSON(http.StatusConflict, gin.H{"error": "This item is being dispatched right now"})
		default:
			h.logger.ErrorContext(ctx, "Failed to retry item", telemetry.WithTraceContext(ctx, "error", err, "tvdb_id", itemKey)...)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retry"})
		}
		return
	}
	c.JSON(http.StatusOK, out)
}

// ListReleases: 200 with candidates, 404 when neither service knows the item,
// 502 when the torrenter could not be reached.
func (h *DownloadControlHandler) ListReleases(c *gin.Context) {
	ctx := c.Request.Context()
	itemKey := c.Param("id")

	resp, err := h.interactor.ListReleases(ctx, itemKey)
	if err != nil {
		if errors.Is(err, interactors.ErrNothingToControl) {
			c.JSON(http.StatusNotFound, gin.H{"error": "No media known for this item"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to list releases", telemetry.WithTraceContext(ctx, "error", err, "tvdb_id", itemKey)...)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Could not search for releases"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Grab: 200 with the torrenter's per-item result, 400 on a malformed
// selection, 404 when neither service knows the item.
func (h *DownloadControlHandler) Grab(c *gin.Context) {
	ctx := c.Request.Context()
	itemKey := c.Param("id")

	var sel controls.ReleaseSelection
	if err := c.ShouldBindJSON(&sel); err != nil || !sel.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A release needs a title and a guid or download_url"})
		return
	}

	result, err := h.interactor.Grab(ctx, itemKey, sel)
	if err != nil {
		if errors.Is(err, interactors.ErrNothingToControl) {
			c.JSON(http.StatusNotFound, gin.H{"error": "No media known for this item"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to grab release",
			telemetry.WithTraceContext(ctx, "error", err, "tvdb_id", itemKey, "torrent_title", sel.Title)...)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Could not start the download"})
		return
	}
	h.logger.InfoContext(ctx, "Manual grab processed",
		telemetry.WithTraceContext(ctx, "tvdb_id", itemKey, "outcome", result.Outcome, "code", string(result.Code))...)
	c.JSON(http.StatusOK, result)
}

// Upcoming: every pending scheduled row, soonest first.
func (h *DownloadControlHandler) Upcoming(c *gin.Context) {
	ctx := c.Request.Context()
	items, err := h.interactor.Upcoming(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to list upcoming downloads", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch upcoming downloads"})
		return
	}
	if items == nil {
		items = []repository.UpcomingItem{}
	}
	c.JSON(http.StatusOK, items)
}
