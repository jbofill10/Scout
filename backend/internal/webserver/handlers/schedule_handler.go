package handlers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
)

type ScheduleHandler struct {
	repo   repository.SchedulerRepository
	logger *slog.Logger
}

func NewScheduleHandler(repo repository.SchedulerRepository, logger *slog.Logger) *ScheduleHandler {
	return &ScheduleHandler{
		repo:   repo,
		logger: logger,
	}
}

func (h *ScheduleHandler) GetWeeklySchedule(c *gin.Context) {
	// Extract context for trace propagation and logging
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Weekly schedule request", telemetry.WithTraceContext(ctx)...)

	schedule, err := h.repo.GetWeeklySchedule()
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch weekly schedule",
			telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(500, gin.H{"error": "Failed to fetch weekly schedule"})
		return
	}

	// Return empty array if no scheduled downloads
	if schedule == nil {
		schedule = []repository.ScheduledDownload{}
	}

	h.logger.InfoContext(ctx, "Returning weekly schedule",
		telemetry.WithTraceContext(ctx, "count", len(schedule))...)
	c.JSON(200, schedule)
}
