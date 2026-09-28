package handlers

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// defaultActivityLimit bounds how many recent items the activity view pulls.
const defaultActivityLimit = 200

// activityService is what the handler needs from the activity interactor,
// narrowed to an interface so tests can stand in for it.
type activityService interface {
	GetActivity(ctx context.Context, limit int) (interactors.ActivitySnapshot, error)
	ClearFinished(ctx context.Context) (int64, error)
}

type ActivityHandler struct {
	interactor activityService
	logger     *slog.Logger
}

func NewActivityHandler(interactor activityService, logger *slog.Logger) *ActivityHandler {
	return &ActivityHandler{
		interactor: interactor,
		logger:     logger,
	}
}

// GetActivity returns in-flight and recently finished downloads with their
// current stage, failure reason and retry schedule.
// Query params: limit (default 200)
func (h *ActivityHandler) GetActivity(c *gin.Context) {
	ctx := c.Request.Context()

	limit := defaultActivityLimit
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	snapshot, err := h.interactor.GetActivity(ctx, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get activity", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(500, gin.H{"error": "Failed to retrieve activity"})
		return
	}

	h.logger.InfoContext(ctx, "Retrieved activity",
		telemetry.WithTraceContext(ctx, "active", len(snapshot.Active), "recent", len(snapshot.Recent))...)
	c.JSON(200, snapshot)
}

// ClearFinishedNotifications dismisses completed notifications and failures the
// scheduler has given up on. In-flight items and pending retries are kept, so
// the bell's "Clear finished" never hides work that is still happening.
func (h *ActivityHandler) ClearFinishedNotifications(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Clearing finished notifications", telemetry.WithTraceContext(ctx)...)

	count, err := h.interactor.ClearFinished(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to clear finished notifications", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(500, gin.H{"error": "Failed to clear notifications"})
		return
	}

	h.logger.InfoContext(ctx, "Cleared finished notifications", telemetry.WithTraceContext(ctx, "count", count)...)
	c.JSON(200, gin.H{"message": "Finished notifications cleared", "count": count})
}
