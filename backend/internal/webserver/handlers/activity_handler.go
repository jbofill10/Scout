package handlers

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/interactors"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// defaultActivityLimit bounds how many recent items the activity view pulls.
const defaultActivityLimit = 200

type ActivityHandler struct {
	interactor *interactors.ActivityInteractor
	logger     *slog.Logger
}

func NewActivityHandler(interactor *interactors.ActivityInteractor, logger *slog.Logger) *ActivityHandler {
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
