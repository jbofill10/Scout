package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	_ "github.com/lib/pq"
)

// Schedule status constants
const (
	StatusPending = "pending"
	StatusQueued  = "queued"
)

// ScheduledDownload represents a scheduled download with UI-relevant fields
type ScheduledDownload struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Season      int       `json:"season"`
	Episode     int       `json:"episode"`
	ReleaseTime time.Time `json:"releaseTime"`
	PosterUrl   string    `json:"posterUrl"`
	IsAnime     bool      `json:"isAnime"`
}

// SchedulerRepository defines DB operations for scheduled shows
type SchedulerRepository interface {
	Schedule(ctx context.Context, media tvdb.Media, releaseTime time.Time, scheduledTraceID, scheduledSpanID string) error
	GetDueMedia(ctx context.Context, windowEnd time.Time) ([]tvdb.Media, error)
	MarkCompleted(ctx context.Context, media tvdb.Media) error
	RequeueOrFail(ctx context.Context, media tvdb.Media, failureReason string, maxRetries int, baseDelay time.Duration) (bool, int, time.Time, error)
	InsertDownloadHistory(mediaTitle string, season, episode, absoluteEpisode int, status, reason, traceID, spanID string) error
	GetWeeklySchedule() ([]ScheduledDownload, error)
}

type SchedulerRepo struct {
	db     *sql.DB
	logger *slog.Logger
	mu     sync.Mutex
}

// ErrDuplicateScheduled is returned when attempting to schedule media that
// already exists in the ScheduledDownloads table (content_hash collision).
var ErrDuplicateScheduled = fmt.Errorf("media already scheduled")

func NewSchedulerRepo(logger *slog.Logger, connStr string) (*SchedulerRepo, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	repo := &SchedulerRepo{db: db, logger: logger}
	return repo, nil
}

func (r *SchedulerRepo) computeContentHash(media tvdb.Media, releaseTime time.Time) string {
	var scheduleHash string
	if media.Category == "movie" || len(media.Metadata.Episodes) == 0 {
		// For movies or media without episodes, use media ID + release date.
		scheduleHash = fmt.Sprintf("%s-%s", media.Id, releaseTime.Format("2006-01-02"))
	} else {
		// For TV shows, use episode identifiers.
		scheduleHash = fmt.Sprintf(
			"%d-%d-%d",
			media.Metadata.Episodes[0].SeasonNumber,
			media.Metadata.Episodes[0].Number,
			media.Metadata.Episodes[0].AbsoluteNumber,
		)
	}
	h := sha256.Sum256([]byte(scheduleHash))
	return hex.EncodeToString(h[:])
}

func releaseTimeForHash(media tvdb.Media, fallback time.Time) time.Time {
	if media.Category != "movie" {
		return fallback
	}
	if media.Metadata.FirstAired == "" {
		return fallback
	}
	releaseTime, err := time.Parse("2006-01-02", media.Metadata.FirstAired)
	if err != nil {
		return fallback
	}
	return releaseTime
}

func (r *SchedulerRepo) Schedule(ctx context.Context, media tvdb.Media, releaseTime time.Time, scheduledTraceID, scheduledSpanID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaJSON, err := json.Marshal(media)
	if err != nil {
		return fmt.Errorf("failed to marshal media: %w", err)
	}

	// Compute a deterministic content hash so we can dedupe scheduling.
	// For movies, keep hash stable across retries by preferring metadata first-air date.
	contentHash := r.computeContentHash(media, releaseTimeForHash(media, releaseTime))

	stmt := `INSERT INTO ScheduledDownloads (media, release_time, schedule_status, content_hash, scheduled_trace_id, scheduled_span_id)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (content_hash) DO NOTHING RETURNING id`

	r.logger.InfoContext(ctx, "Attempting to schedule media",
		slog.String("media", media.Name),
		slog.String("release_time", releaseTime.Format(time.RFC3339)),
		slog.String("content_hash", contentHash),
	)
	var insertedID int
	err = r.db.QueryRow(stmt, mediaJSON, releaseTime.Format(time.RFC3339), StatusPending, contentHash, scheduledTraceID, scheduledSpanID).Scan(&insertedID)
	if err != nil {
		if err == sql.ErrNoRows {
			// ON CONFLICT DO NOTHING caused no insert; treat as duplicate
			r.logger.InfoContext(ctx, "Show already scheduled, skipping",
				slog.String("media", media.Name),
				slog.String("release_time", releaseTime.Format(time.RFC3339)),
				slog.String("content_hash", contentHash),
			)
			return ErrDuplicateScheduled
		}
		return fmt.Errorf("failed to insert scheduled media: %w", err)
	}

	r.logger.InfoContext(ctx, "Successfully scheduled media",
		slog.String("media", media.Name),
		slog.String("release_time", releaseTime.Format(time.RFC3339)),
		slog.String("content_hash", contentHash),
	)

	return nil
}

func (r *SchedulerRepo) GetDueMedia(ctx context.Context, windowEnd time.Time) ([]tvdb.Media, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin due-media transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Include stale queued items so jobs are recoverable after crashes/restarts.
	stmt := `SELECT id, media FROM ScheduledDownloads
			 WHERE (
			         schedule_status = $1
			         OR (
			            schedule_status = $2
			            AND (last_attempt_at IS NULL OR last_attempt_at <= NOW() - INTERVAL '15 minutes')
			         )
			       )
			   AND COALESCE(next_attempt_at, release_time) <= $3
			 ORDER BY COALESCE(next_attempt_at, release_time) ASC
			 FOR UPDATE SKIP LOCKED`
	rows, err := tx.QueryContext(ctx, stmt, StatusPending, StatusQueued, windowEnd.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.ErrorContext(ctx, "Failed to close rows", "error", err)
		}
	}()

	var results []tvdb.Media
	var idsToMark []int
	for rows.Next() {
		var id int
		var mediaJSON []byte
		if err := rows.Scan(&id, &mediaJSON); err != nil {
			return nil, err
		}
		var media tvdb.Media
		if err := json.Unmarshal(mediaJSON, &media); err != nil {
			r.logger.ErrorContext(ctx, "Failed to unmarshal media", "id", id, "error", err)
			continue
		}
		results = append(results, media)
		idsToMark = append(idsToMark, id)
	}

	// Mark all retrieved items as queued and stamp attempt time.
	for _, id := range idsToMark {
		if _, err := tx.ExecContext(ctx, `UPDATE ScheduledDownloads SET schedule_status = $1, last_attempt_at = NOW() WHERE id = $2`, StatusQueued, id); err != nil {
			return nil, fmt.Errorf("failed to mark scheduled media queued: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit due-media transaction: %w", err)
	}

	return results, nil
}

func (r *SchedulerRepo) MarkCompleted(ctx context.Context, media tvdb.Media) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	contentHash := r.computeContentHash(media, releaseTimeForHash(media, time.Now().UTC()))
	_, err := r.db.ExecContext(ctx, `DELETE FROM ScheduledDownloads WHERE content_hash = $1`, contentHash)
	if err != nil {
		return fmt.Errorf("failed to mark scheduled media complete: %w", err)
	}
	return nil
}

func computeBackoffDelay(baseDelay time.Duration, attempt int) time.Duration {
	if baseDelay <= 0 {
		baseDelay = 5 * time.Minute
	}
	// Exponential backoff with cap.
	delay := baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= 6*time.Hour {
			return 6 * time.Hour
		}
	}
	return delay
}

func (r *SchedulerRepo) RequeueOrFail(
	ctx context.Context,
	media tvdb.Media,
	failureReason string,
	maxRetries int,
	baseDelay time.Duration,
) (bool, int, time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	contentHash := r.computeContentHash(media, releaseTimeForHash(media, time.Now().UTC()))

	var currentRetryCount int
	var rowMaxRetries int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT retry_count, max_retries FROM ScheduledDownloads WHERE content_hash = $1`,
		contentHash,
	).Scan(&currentRetryCount, &rowMaxRetries)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, 0, time.Time{}, nil
		}
		return false, 0, time.Time{}, fmt.Errorf("failed to load scheduled retry state: %w", err)
	}

	if rowMaxRetries <= 0 {
		rowMaxRetries = maxRetries
	}
	if rowMaxRetries <= 0 {
		rowMaxRetries = 5
	}

	nextAttemptNumber := currentRetryCount + 1
	if nextAttemptNumber > rowMaxRetries {
		_, delErr := r.db.ExecContext(ctx, `DELETE FROM ScheduledDownloads WHERE content_hash = $1`, contentHash)
		if delErr != nil {
			return false, nextAttemptNumber, time.Time{}, fmt.Errorf("failed to cleanup exhausted scheduled media: %w", delErr)
		}
		return false, nextAttemptNumber, time.Time{}, nil
	}

	delay := computeBackoffDelay(baseDelay, nextAttemptNumber)
	nextAttemptAt := time.Now().UTC().Add(delay)

	_, err = r.db.ExecContext(
		ctx,
		`UPDATE ScheduledDownloads
		 SET schedule_status = $1,
		     retry_count = $2,
		     max_retries = $3,
		     last_error = $4,
		     next_attempt_at = $5,
		     last_attempt_at = NOW()
		 WHERE content_hash = $6`,
		StatusPending,
		nextAttemptNumber,
		rowMaxRetries,
		failureReason,
		nextAttemptAt.Format(time.RFC3339),
		contentHash,
	)
	if err != nil {
		return false, nextAttemptNumber, time.Time{}, fmt.Errorf("failed to requeue scheduled media: %w", err)
	}

	return true, nextAttemptNumber, nextAttemptAt, nil
}

func (r *SchedulerRepo) InsertDownloadHistory(mediaTitle string, season, episode, absoluteEpisode int, status, reason, traceID, spanID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.Exec(`
		INSERT INTO ShowDownloadHistory (mediaTitle, season, episode, absoluteEpisode, status, reason, trace_id, span_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, mediaTitle, season, episode, absoluteEpisode, status, reason, traceID, spanID)
	if err != nil {
		return fmt.Errorf("failed to insert download history: %w", err)
	}
	return nil
}

func (r *SchedulerRepo) GetWeeklySchedule() ([]ScheduledDownload, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	oneWeekFromNow := now.Add(7 * 24 * time.Hour)

	stmt := `SELECT id, media, release_time FROM ScheduledDownloads
			 WHERE schedule_status = $1 AND release_time BETWEEN $2 AND $3
			 ORDER BY release_time ASC`

	rows, err := r.db.Query(stmt, StatusPending, now.Format(time.RFC3339), oneWeekFromNow.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to query weekly schedule: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.Error("Failed to close rows", "error", err)
		}
	}()

	var results []ScheduledDownload
	for rows.Next() {
		var id int
		var mediaJSON []byte
		var releaseTimeStr string

		if err := rows.Scan(&id, &mediaJSON, &releaseTimeStr); err != nil {
			r.logger.Error("Failed to scan row", "error", err)
			continue
		}

		var media tvdb.Media
		if err := json.Unmarshal(mediaJSON, &media); err != nil {
			r.logger.Error("Failed to unmarshal media", "id", id, "error", err)
			continue
		}

		releaseTime, err := time.Parse(time.RFC3339, releaseTimeStr)
		if err != nil {
			r.logger.Error("Failed to parse release time", "id", id, "error", err)
			continue
		}

		// Extract season and episode from the first episode in metadata
		season := 0
		episode := 0
		if len(media.Metadata.Episodes) > 0 {
			season = media.Metadata.Episodes[0].SeasonNumber
			episode = media.Metadata.Episodes[0].Number
		}

		results = append(results, ScheduledDownload{
			ID:          id,
			Title:       media.Name,
			Season:      season,
			Episode:     episode,
			ReleaseTime: releaseTime,
			PosterUrl:   media.ImageUrl,
			IsAnime:     media.Anime,
		})
	}

	return results, nil
}
