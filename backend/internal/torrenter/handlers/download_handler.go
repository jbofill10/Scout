package handlers

import (
	"context"
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
	// The search and the torrent add outlive the caller's connection: the
	// webserver restarting (or giving up) mid-request must not cancel work
	// that is about to hand qBittorrent a torrent, or the torrent arrives
	// untracked. The trace stays attached; only cancellation is dropped.
	ctx := workContext(c)

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

// workContext is the request's context without its cancellation: the trace
// and values carry on, but a client that disconnects no longer aborts work the
// torrenter has started on its behalf. Deadlines set by the torrenter itself
// (the search deadline) and user cancels (the in-flight registry) still apply.
func workContext(c *gin.Context) context.Context {
	return context.WithoutCancel(c.Request.Context())
}
