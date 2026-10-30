package service

import (
	"context"

	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// updateNotificationStatus updates a notification with the given status and reason
// This is a generalized function used by all notification update operations to reduce code duplication
func (q *QbittHandler) updateNotificationStatus(ctx context.Context, tvdbId, reason string, status notifications.NotificationStatus) {
	q.setNotification(ctx, tvdbId, status, reason, "")
}

// failNotification marks an item failed with the machine-readable code behind
// the reason. The webserver reads the code to decide whether a failure the
// torrenter reported on its own (a lost monitor, a dead swarm, a torrent that
// vanished from qBittorrent) is transient and deserves a retry.
func (q *QbittHandler) failNotification(ctx context.Context, tvdbId string, code dlstatus.FailureCode, reason string) {
	q.setNotification(ctx, tvdbId, notifications.StatusFailed, reason, string(code))
}

func (q *QbittHandler) setNotification(ctx context.Context, tvdbId string, status notifications.NotificationStatus, reason, failureCode string) {
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
	// A code only means something on a failed row; every other transition
	// clears whatever an earlier failure left behind.
	notification.FailureCode = failureCode

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
