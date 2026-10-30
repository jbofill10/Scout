package repository

import (
	"context"

	"github.com/jbofill10/scout/backend/pkg/notifications"
)

// NotificationRepository defines DB operations for download notifications
// Embeds the core notifications.Repository interface and extends it with webserver-specific methods
type NotificationRepository interface {
	// Core methods from shared notifications.Repository interface
	notifications.Repository

	// Webserver-specific methods for frontend support
	// GetNotifications retrieves recent notifications with optional filters
	GetNotifications(ctx context.Context, limit int, category string, status string) ([]notifications.Notification, error)

	// GetGrouped retrieves notifications grouped by media
	GetGrouped(ctx context.Context) ([]notifications.GroupedNotification, error)

	// MarkAsRead marks a notification as read
	MarkAsRead(ctx context.Context, id int) error

	// ExistingTvdbIDs reports which of the given item keys already have a
	// notification (any status), so a follow check can skip them
	ExistingTvdbIDs(ctx context.Context, tvdbIDs []string) (map[string]bool, error)

	// PendingDeliveries returns terminal notifications whose status has not been
	// delivered to outbound targets yet, oldest change first
	PendingDeliveries(ctx context.Context, limit int) ([]notifications.Notification, error)
	// MarkNotified records that status was delivered; a no-op if the status moved on
	MarkNotified(ctx context.Context, id int, status string) error

	// DismissFinished soft-deletes every visible notification that is done: completed
	// ones, and failed ones whose tvdb id is not in keepTvdbIDs (the ids the scheduler
	// still has a pending retry for). In-flight statuses are never touched.
	DismissFinished(ctx context.Context, keepTvdbIDs []string) (int64, error)

	// Dismiss soft-deletes a notification (sets auto_dismissed = true)
	Dismiss(ctx context.Context, id int) error

	// GetUnreadCount returns the count of unread notifications
	GetUnreadCount(ctx context.Context) (int, error)

	// CleanupOld deletes notifications older than retentionDays and auto-dismisses successful notifications
	CleanupOld(ctx context.Context, retentionDays int) error
}
