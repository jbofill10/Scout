package handlers

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type NotificationHandler struct {
	repo   repository.NotificationRepository
	logger *slog.Logger
}

func NewNotificationHandler(repo repository.NotificationRepository, logger *slog.Logger) *NotificationHandler {
	return &NotificationHandler{
		repo:   repo,
		logger: logger,
	}
}

// GetNotifications retrieves recent notifications with optional filters
// Query params: limit (default 100), category (series/movie), status (scheduled/searching/downloading/completed/failed)
func (h *NotificationHandler) GetNotifications(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse query parameters
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}

	category := c.Query("category")
	status := c.Query("status")

	h.logger.InfoContext(ctx, "Fetching notifications",
		telemetry.WithTraceContext(ctx, "limit", limit, "category", category, "status", status)...)

	notifications, err := h.repo.GetNotifications(ctx, limit, category, status)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get notifications", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(500, gin.H{"error": "Failed to retrieve notifications"})
		return
	}

	h.logger.InfoContext(ctx, "Retrieved notifications", telemetry.WithTraceContext(ctx, "count", len(notifications))...)
	c.JSON(200, notifications)
}

// GetGroupedNotifications retrieves notifications grouped by media
func (h *NotificationHandler) GetGroupedNotifications(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Fetching grouped notifications", telemetry.WithTraceContext(ctx)...)

	grouped, err := h.repo.GetGrouped(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get grouped notifications", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(500, gin.H{"error": "Failed to retrieve grouped notifications"})
		return
	}

	h.logger.InfoContext(ctx, "Retrieved grouped notifications", telemetry.WithTraceContext(ctx, "count", len(grouped))...)
	c.JSON(200, grouped)
}

// GetUnreadCount returns the count of unread notifications
func (h *NotificationHandler) GetUnreadCount(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Fetching unread count", telemetry.WithTraceContext(ctx)...)

	count, err := h.repo.GetUnreadCount(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get unread count", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(500, gin.H{"error": "Failed to retrieve unread count"})
		return
	}

	h.logger.InfoContext(ctx, "Retrieved unread count", telemetry.WithTraceContext(ctx, "count", count)...)
	c.JSON(200, gin.H{"count": count})
}

// MarkAsRead marks a notification as read
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	ctx := c.Request.Context()

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.logger.WarnContext(ctx, "Invalid notification ID", telemetry.WithTraceContext(ctx, "id", idStr)...)
		c.JSON(400, gin.H{"error": "Invalid notification ID"})
		return
	}

	h.logger.InfoContext(ctx, "Marking notification as read", telemetry.WithTraceContext(ctx, "id", id)...)

	err = h.repo.MarkAsRead(ctx, id)
	if err != nil {
		if err.Error() == "notification not found: id="+idStr {
			h.logger.WarnContext(ctx, "Notification not found", telemetry.WithTraceContext(ctx, "id", id)...)
			c.JSON(404, gin.H{"error": "Notification not found"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to mark notification as read", telemetry.WithTraceContext(ctx, "error", err, "id", id)...)
		c.JSON(500, gin.H{"error": "Failed to mark notification as read"})
		return
	}

	h.logger.InfoContext(ctx, "Marked notification as read", telemetry.WithTraceContext(ctx, "id", id)...)
	c.JSON(200, gin.H{"message": "Notification marked as read"})
}

// DismissNotification soft-deletes a notification
func (h *NotificationHandler) DismissNotification(c *gin.Context) {
	ctx := c.Request.Context()

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.logger.WarnContext(ctx, "Invalid notification ID", telemetry.WithTraceContext(ctx, "id", idStr)...)
		c.JSON(400, gin.H{"error": "Invalid notification ID"})
		return
	}

	h.logger.InfoContext(ctx, "Dismissing notification", telemetry.WithTraceContext(ctx, "id", id)...)

	err = h.repo.Dismiss(ctx, id)
	if err != nil {
		if err.Error() == "notification not found: id="+idStr {
			h.logger.WarnContext(ctx, "Notification not found", telemetry.WithTraceContext(ctx, "id", id)...)
			c.JSON(404, gin.H{"error": "Notification not found"})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to dismiss notification", telemetry.WithTraceContext(ctx, "error", err, "id", id)...)
		c.JSON(500, gin.H{"error": "Failed to dismiss notification"})
		return
	}

	h.logger.InfoContext(ctx, "Dismissed notification", telemetry.WithTraceContext(ctx, "id", id)...)
	c.JSON(200, gin.H{"message": "Notification dismissed"})
}
