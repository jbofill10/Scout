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

	// Dismiss soft-deletes a notification (sets auto_dismissed = true)
	Dismiss(ctx context.Context, id int) error

	// GetUnreadCount returns the count of unread notifications
	GetUnreadCount(ctx context.Context) (int, error)

	// CleanupOld deletes notifications older than retentionDays and auto-dismisses successful notifications
	CleanupOld(ctx context.Context, retentionDays int) error
}
