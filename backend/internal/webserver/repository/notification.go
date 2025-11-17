package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/pkg/notifications"
	_ "github.com/lib/pq"
)

// NotificationRepo implements NotificationRepository using PostgreSQL
// Embeds the shared PostgresRepository for basic CRUD operations
type NotificationRepo struct {
	*notifications.PostgresRepository
	db     *sql.DB
	logger *slog.Logger
	mu     sync.Mutex
}

// NewNotificationRepo creates a new NotificationRepo instance
func NewNotificationRepo(logger *slog.Logger, connStr string) (*NotificationRepo, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &NotificationRepo{
		PostgresRepository: notifications.NewPostgresRepository(db, logger),
		db:                 db,
		logger:             logger,
	}, nil
}

// GetNotifications retrieves recent notifications with optional filters
func (r *NotificationRepo) GetNotifications(ctx context.Context, limit int, category string, status string) ([]notifications.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		SELECT id, tvdb_id, media_title, category, season, episode, absolute_episode,
			poster_url, is_anime, status, reason,
			is_read, auto_dismissed, trace_id, span_id, created_at, updated_at
		FROM Notifications
		WHERE auto_dismissed = false
	`
	args := []interface{}{}
	argCount := 1

	if category != "" {
		query += fmt.Sprintf(" AND category = $%d", argCount)
		args = append(args, category)
		argCount++
	}

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, status)
		argCount++
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to query notifications", "error", err)
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}
	defer rows.Close()

	var notifs []notifications.Notification
	for rows.Next() {
		var n notifications.Notification
		err := rows.Scan(
			&n.ID, &n.TvdbID, &n.MediaTitle, &n.Category, &n.Season, &n.Episode, &n.AbsoluteEpisode,
			&n.PosterURL, &n.IsAnime, &n.Status, &n.Reason,
			&n.IsRead, &n.AutoDismissed, &n.TraceID, &n.SpanID, &n.CreatedAt, &n.UpdatedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to scan notification row", "error", err)
			continue
		}
		notifs = append(notifs, n)
	}

	if err = rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error iterating notification rows", "error", err)
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	r.logger.InfoContext(ctx, "Retrieved notifications", "count", len(notifs))
	return notifs, nil
}

// GetGrouped retrieves notifications grouped by media
func (r *NotificationRepo) GetGrouped(ctx context.Context) ([]notifications.GroupedNotification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// First, get all unique media with their latest timestamp
	mediaQuery := `
		SELECT DISTINCT ON (tvdb_id) tvdb_id, media_title, category, poster_url, is_anime,
			MAX(created_at) as latest_timestamp
		FROM Notifications
		WHERE auto_dismissed = false
		GROUP BY tvdb_id, media_title, category, poster_url, is_anime
		ORDER BY tvdb_id, MAX(created_at) DESC
	`

	mediaRows, err := r.db.QueryContext(ctx, mediaQuery)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to query grouped media", "error", err)
		return nil, fmt.Errorf("failed to query grouped media: %w", err)
	}
	defer mediaRows.Close()

	var grouped []notifications.GroupedNotification
	for mediaRows.Next() {
		var g notifications.GroupedNotification
		err := mediaRows.Scan(&g.TvdbID, &g.MediaTitle, &g.Category, &g.PosterURL, &g.IsAnime, &g.LatestTimestamp)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to scan grouped media row", "error", err)
			continue
		}

		// Get all notifications for this media
		notifQuery := `
			SELECT id, tvdb_id, media_title, category, season, episode, absolute_episode,
				poster_url, is_anime, status, reason,
				is_read, auto_dismissed, trace_id, span_id, created_at, updated_at
			FROM Notifications
			WHERE tvdb_id = $1 AND auto_dismissed = false
			ORDER BY created_at DESC
		`

		notifRows, err := r.db.QueryContext(ctx, notifQuery, g.TvdbID)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to query notifications for media",
				"error", err, "tvdb_id", g.TvdbID)
			continue
		}

		var notifs []notifications.Notification
		for notifRows.Next() {
			var n notifications.Notification
			err := notifRows.Scan(
				&n.ID, &n.TvdbID, &n.MediaTitle, &n.Category, &n.Season, &n.Episode, &n.AbsoluteEpisode,
				&n.PosterURL, &n.IsAnime, &n.Status, &n.Reason,
				&n.IsRead, &n.AutoDismissed, &n.TraceID, &n.SpanID, &n.CreatedAt, &n.UpdatedAt,
			)
			if err != nil {
				r.logger.ErrorContext(ctx, "Failed to scan notification row", "error", err)
				continue
			}
			notifs = append(notifs, n)
		}
		notifRows.Close()

		g.Notifications = notifs
		grouped = append(grouped, g)
	}

	if err = mediaRows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error iterating grouped media rows", "error", err)
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	r.logger.InfoContext(ctx, "Retrieved grouped notifications", "count", len(grouped))
	return grouped, nil
}

// MarkAsRead marks a notification as read
func (r *NotificationRepo) MarkAsRead(ctx context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	stmt := `UPDATE Notifications SET is_read = true, updated_at = $1 WHERE id = $2`
	result, err := r.db.ExecContext(ctx, stmt, time.Now(), id)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to mark notification as read", "error", err, "id", id)
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("notification not found: id=%d", id)
	}

	r.logger.InfoContext(ctx, "Marked notification as read", "id", id)
	return nil
}

// Dismiss soft-deletes a notification (sets auto_dismissed = true)
func (r *NotificationRepo) Dismiss(ctx context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	stmt := `UPDATE Notifications SET auto_dismissed = true, updated_at = $1 WHERE id = $2`
	result, err := r.db.ExecContext(ctx, stmt, time.Now(), id)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to dismiss notification", "error", err, "id", id)
		return fmt.Errorf("failed to dismiss notification: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("notification not found: id=%d", id)
	}

	r.logger.InfoContext(ctx, "Dismissed notification", "id", id)
	return nil
}

// GetUnreadCount returns the count of unread notifications
func (r *NotificationRepo) GetUnreadCount(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stmt := `SELECT COUNT(*) FROM Notifications WHERE is_read = false AND auto_dismissed = false`
	var count int
	err := r.db.QueryRowContext(ctx, stmt).Scan(&count)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to get unread count", "error", err)
		return 0, fmt.Errorf("failed to get unread count: %w", err)
	}

	r.logger.InfoContext(ctx, "Retrieved unread count", "count", count)
	return count, nil
}

// CleanupOld deletes notifications older than retentionDays and auto-dismisses successful notifications
func (r *NotificationRepo) CleanupOld(ctx context.Context, retentionDays int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Auto-dismiss completed notifications older than 24 hours
	autoDismissStmt := `
		UPDATE Notifications
		SET auto_dismissed = true, updated_at = $1
		WHERE status = 'completed' AND created_at < $2 AND auto_dismissed = false
	`
	autoDismissTime := time.Now().Add(-24 * time.Hour)
	result, err := r.db.ExecContext(ctx, autoDismissStmt, time.Now(), autoDismissTime)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to auto-dismiss old successful notifications", "error", err)
		return fmt.Errorf("failed to auto-dismiss notifications: %w", err)
	}

	autoDismissed, _ := result.RowsAffected()
	r.logger.InfoContext(ctx, "Auto-dismissed old successful notifications", "count", autoDismissed)

	// Delete notifications older than retention period
	deleteStmt := `DELETE FROM Notifications WHERE created_at < $1`
	retentionTime := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	result, err = r.db.ExecContext(ctx, deleteStmt, retentionTime)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to delete old notifications", "error", err)
		return fmt.Errorf("failed to delete old notifications: %w", err)
	}

	deleted, _ := result.RowsAffected()
	r.logger.InfoContext(ctx, "Deleted old notifications",
		"count", deleted,
		"retention_days", retentionDays)

	return nil
}
