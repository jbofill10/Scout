package notifications

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// PostgresRepository provides a shared PostgreSQL implementation of the Repository interface
// Both webserver and torrenter can embed this to get standard notification CRUD operations
type PostgresRepository struct {
	db     *sql.DB
	logger *slog.Logger
	mu     sync.Mutex
}

// NewPostgresRepository creates a new shared PostgreSQL notification repository
func NewPostgresRepository(db *sql.DB, logger *slog.Logger) *PostgresRepository {
	return &PostgresRepository{
		db:     db,
		logger: logger,
	}
}

// CreateNotification creates a new notification in the database
func (r *PostgresRepository) CreateNotification(ctx context.Context, notification *Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Extract trace_id and span_id from context if not already set
	if notification.TraceID == "" || notification.SpanID == "" {
		traceID, spanID := telemetry.GetTraceSpanIDs(ctx)
		notification.TraceID = traceID
		notification.SpanID = spanID
	}

	stmt := `
		INSERT INTO Notifications (
			tvdb_id, media_title, category, season, episode, absolute_episode,
			poster_url, is_anime, status, reason,
			is_read, auto_dismissed, trace_id, span_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, stmt,
		notification.TvdbID,
		notification.MediaTitle,
		notification.Category,
		notification.Season,
		notification.Episode,
		notification.AbsoluteEpisode,
		notification.PosterURL,
		notification.IsAnime,
		notification.Status,
		notification.Reason,
		notification.IsRead,
		notification.AutoDismissed,
		notification.TraceID,
		notification.SpanID,
		time.Now(),
		time.Now(),
	).Scan(&notification.ID, &notification.CreatedAt, &notification.UpdatedAt)

	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to create notification",
			"error", err,
			"tvdb_id", notification.TvdbID,
			"media_title", notification.MediaTitle)
		return fmt.Errorf("failed to insert notification: %w", err)
	}

	r.logger.InfoContext(ctx, "Created notification",
		"id", notification.ID,
		"tvdb_id", notification.TvdbID,
		"media_title", notification.MediaTitle,
		"status", notification.Status)

	return nil
}

// GetNotification retrieves a notification by its TVDB ID
// For series: tvdbID is the episode ID from TVDB
// For movies: tvdbID is the movie ID from TVDB
func (r *PostgresRepository) GetNotification(ctx context.Context, tvdbID string) (*Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		SELECT id, tvdb_id, media_title, category, season, episode, absolute_episode,
			poster_url, is_anime, status, reason,
			is_read, auto_dismissed, trace_id, span_id, created_at, updated_at
		FROM Notifications
		WHERE tvdb_id = $1 AND auto_dismissed = false
		ORDER BY created_at DESC
		LIMIT 1
	`

	var n Notification
	err := r.db.QueryRowContext(ctx, query, tvdbID).Scan(
		&n.ID, &n.TvdbID, &n.MediaTitle, &n.Category, &n.Season, &n.Episode, &n.AbsoluteEpisode,
		&n.PosterURL, &n.IsAnime, &n.Status, &n.Reason,
		&n.IsRead, &n.AutoDismissed, &n.TraceID, &n.SpanID, &n.CreatedAt, &n.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found, return nil without error
	}
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to get notification",
			"error", err,
			"tvdb_id", tvdbID)
		return nil, fmt.Errorf("failed to query notification: %w", err)
	}

	r.logger.InfoContext(ctx, "Retrieved notification by tvdb_id",
		"tvdb_id", tvdbID,
		"id", n.ID,
		"media_title", n.MediaTitle)

	return &n, nil
}

// UpdateNotification updates an existing notification in the database
func (r *PostgresRepository) UpdateNotification(ctx context.Context, notification *Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	stmt := `
		UPDATE Notifications
		SET status = $1, reason = $2,
		    is_read = $3, auto_dismissed = $4, updated_at = $5
		WHERE id = $6
	`

	result, err := r.db.ExecContext(ctx, stmt,
		notification.Status,
		notification.Reason,
		notification.IsRead,
		notification.AutoDismissed,
		time.Now(),
		notification.ID,
	)

	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to update notification",
			"error", err,
			"id", notification.ID)
		return fmt.Errorf("failed to update notification: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("notification not found: id=%d", notification.ID)
	}

	r.logger.InfoContext(ctx, "Updated notification",
		"id", notification.ID,
		"status", notification.Status)

	return nil
}
