package service

import (
	"context"

	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// updateNotificationStatus updates a notification with the given status and reason
// This is a generalized function used by all notification update operations to reduce code duplication
func (q *QbittHandler) updateNotificationStatus(ctx context.Context, tvdbId, reason string, status notifications.NotificationStatus) {
	notification, err := q.repo.GetNotification(ctx, tvdbId)

	if err != nil {
		q.logger.ErrorContext(ctx, "Failed to find notification for update",
			telemetry.WithTraceContext(ctx,
				"error", err.Error(),
				"tvdb_id", tvdbId,
				"status", string(status))...)
		return
	}

	if notification == nil {
		q.logger.WarnContext(ctx, "No notification found (webserver should have created it)",
			telemetry.WithTraceContext(ctx,
				"tvdb_id", tvdbId,
				"status", string(status))...)
		return
	}

	// Update notification fields
	notification.Status = status
	notification.Reason = reason

	err = q.repo.UpdateNotification(ctx, notification)
	if err != nil {
		q.logger.ErrorContext(ctx, "Failed to update notification",
			telemetry.WithTraceContext(ctx,
				"error", err.Error(),
				"notification_id", notification.ID,
				"status", string(status))...)
		return
	}

	// Build log attributes and emit the success log
	logAttrs := []any{
		"notification_id", notification.ID,
		"status", string(status),
	}
	if reason != "" {
		logAttrs = append(logAttrs, "reason", reason)
	}
	q.logger.InfoContext(ctx, "Updated notification status", telemetry.WithTraceContext(ctx, logAttrs...)...)
}
