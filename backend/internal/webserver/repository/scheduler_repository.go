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
	StatusPending   = "pending"
	StatusQueued    = "queued"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// DueItem is a scheduled download row that is due (or retry-due) for dispatch.
type DueItem struct {
	ID          int
	Media       tvdb.Media
	ReleaseTime time.Time
	DueAt       time.Time // COALESCE(next_attempt_at, release_time)
	Attempts    int
}

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
	ScheduleRetry(ctx context.Context, media tvdb.Media, firstAttemptAt time.Time, code, reason, scheduledTraceID, scheduledSpanID string) error
	GetDueMedia(ctx context.Context, windowEnd time.Time) ([]DueItem, error)
	MarkQueued(ctx context.Context, id int) error
	MarkCompleted(ctx context.Context, id int) error
	RecordFailure(ctx context.Context, id int, code, reason string, nextAttempt time.Time) error
	MarkPermanentlyFailed(ctx context.Context, id int, code, reason string) error
	ResetStaleQueued(ctx context.Context, olderThan time.Duration) (int64, error)
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

// ComputeContentHash returns a deterministic SHA-256 content hash for media
// deduplication. The hash incorporates the media ID to prevent collisions
// between different shows that share the same season/episode numbers.
func ComputeContentHash(media tvdb.Media, releaseTime time.Time) string {
	var scheduleHash string
	if media.Category == "movie" || len(media.Metadata.Episodes) == 0 {
		// For movies or media without episodes, use media ID + release date
		scheduleHash = fmt.Sprintf("%s-%s", media.Id, releaseTime.Format("2006-01-02"))
	} else {
		// For TV shows, use media ID + episode identifiers
		ep := media.Metadata.Episodes[0]
		scheduleHash = fmt.Sprintf("%s-%d-%d-%d",
			media.Id,
			ep.SeasonNumber,
			ep.Number,
			ep.AbsoluteNumber,
		)
	}
	h := sha256.Sum256([]byte(scheduleHash))
	return hex.EncodeToString(h[:])
}

// Ping checks the database connectivity with the given context.
func (r *SchedulerRepo) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func NewSchedulerRepo(logger *slog.Logger, connStr string) (*SchedulerRepo, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	repo := &SchedulerRepo{db: db, logger: logger}
	return repo, nil
}

func (r *SchedulerRepo) Schedule(ctx context.Context, media tvdb.Media, releaseTime time.Time, scheduledTraceID, scheduledSpanID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaJSON, err := json.Marshal(media)
	if err != nil {
		return fmt.Errorf("failed to marshal media: %w", err)
	}

	// Compute a deterministic content hash for deduplication.
	contentHash := ComputeContentHash(media, releaseTime)

	stmt := `INSERT INTO ScheduledDownloads (media, release_time, schedule_status, content_hash, scheduled_trace_id, scheduled_span_id)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (content_hash) DO NOTHING RETURNING id`

	r.logger.InfoContext(ctx, "Attempting to schedule media",
		slog.String("media", media.Name),
		slog.String("release_time", releaseTime.Format(time.RFC3339)),
		slog.String("content_hash", contentHash),
	)
	err = r.db.QueryRow(stmt, mediaJSON, releaseTime.Format(time.RFC3339), StatusPending, contentHash, scheduledTraceID, scheduledSpanID).Scan(&contentHash)
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

// GetDueMedia returns pending rows whose effective due time (next_attempt_at
// when set, otherwise release_time) is at or before windowEnd. It no longer
// mutates row status; callers transition rows explicitly via MarkQueued /
// MarkCompleted / RecordFailure / MarkPermanentlyFailed.
func (r *SchedulerRepo) GetDueMedia(ctx context.Context, windowEnd time.Time) ([]DueItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stmt := `SELECT id, media, release_time, attempts, COALESCE(next_attempt_at, release_time) AS due_at
			 FROM ScheduledDownloads
			 WHERE schedule_status = $1 AND COALESCE(next_attempt_at, release_time) <= $2
			 ORDER BY COALESCE(next_attempt_at, release_time) ASC`
	rows, err := r.db.QueryContext(ctx, stmt, StatusPending, windowEnd.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.ErrorContext(ctx, "Failed to close rows", "error", err)
		}
	}()

	var results []DueItem
	for rows.Next() {
		var id int
		var mediaJSON []byte
		var releaseTime time.Time
		var attempts int
		var dueAt time.Time
		if err := rows.Scan(&id, &mediaJSON, &releaseTime, &attempts, &dueAt); err != nil {
			return nil, err
		}
		var media tvdb.Media
		if err := json.Unmarshal(mediaJSON, &media); err != nil {
			r.logger.ErrorContext(ctx, "Failed to unmarshal media", "id", id, "error", err)
			continue
		}
		results = append(results, DueItem{
			ID:          id,
			Media:       media,
			ReleaseTime: releaseTime,
			DueAt:       dueAt,
			Attempts:    attempts,
		})
	}

	return results, nil
}

// MarkQueued marks a row as queued (dispatched to the torrenter) and records
// the time, so ResetStaleQueued can later recover rows whose dispatch crashed.
func (r *SchedulerRepo) MarkQueued(ctx context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads SET schedule_status = $1, queued_at = now() WHERE id = $2`,
		StatusQueued, id)
	if err != nil {
		return fmt.Errorf("failed to mark row queued: %w", err)
	}
	return nil
}

// MarkCompleted marks a row as terminally completed.
func (r *SchedulerRepo) MarkCompleted(ctx context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads SET schedule_status = $1 WHERE id = $2`,
		StatusCompleted, id)
	if err != nil {
		return fmt.Errorf("failed to mark row completed: %w", err)
	}
	return nil
}

// RecordFailure records a transient failure and reschedules the row for a
// future retry attempt by returning it to pending with a next_attempt_at.
func (r *SchedulerRepo) RecordFailure(ctx context.Context, id int, code, reason string, nextAttempt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads
		 SET schedule_status = $1, attempts = attempts + 1, next_attempt_at = $2,
		     last_failure_code = $3, last_failure_reason = $4
		 WHERE id = $5`,
		StatusPending, nextAttempt.Format(time.RFC3339), code, reason, id)
	if err != nil {
		return fmt.Errorf("failed to record failure: %w", err)
	}
	return nil
}

// MarkPermanentlyFailed marks a row as terminally failed (no further retries).
func (r *SchedulerRepo) MarkPermanentlyFailed(ctx context.Context, id int, code, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads
		 SET schedule_status = $1, attempts = attempts + 1,
		     last_failure_code = $2, last_failure_reason = $3
		 WHERE id = $4`,
		StatusFailed, code, reason, id)
	if err != nil {
		return fmt.Errorf("failed to mark row permanently failed: %w", err)
	}
	return nil
}

// ResetStaleQueued returns rows stuck in 'queued' longer than olderThan back to
// 'pending' so they can be re-dispatched (recovers from crashes mid-dispatch).
// It returns the number of rows reset.
func (r *SchedulerRepo) ResetStaleQueued(ctx context.Context, olderThan time.Duration) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	result, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads SET schedule_status = $1
		 WHERE schedule_status = $2 AND queued_at < $3`,
		StatusPending, StatusQueued, cutoff.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("failed to reset stale queued rows: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected > 0 {
		r.logger.InfoContext(ctx, "Reset stale queued downloads", "count", affected)
	}
	return affected, nil
}

// ScheduleRetry inserts a new pending row seeded with a first retry attempt for
// an immediate-download failure that has no pre-existing scheduled row. The
// content hash is computed from the media's natural release time so the row
// dedups against the canonical scheduled row, and ON CONFLICT DO NOTHING treats
// an already-tracked retry as success.
func (r *SchedulerRepo) ScheduleRetry(ctx context.Context, media tvdb.Media, firstAttemptAt time.Time, code, reason, scheduledTraceID, scheduledSpanID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaJSON, err := json.Marshal(media)
	if err != nil {
		return fmt.Errorf("failed to marshal media: %w", err)
	}

	// Use the media's natural release time for the hash so retries dedup against
	// any canonical scheduled row a future poll would create.
	hashTime := mediaReleaseTime(media)
	contentHash := ComputeContentHash(media, hashTime)

	stmt := `INSERT INTO ScheduledDownloads
			 (media, release_time, schedule_status, content_hash, scheduled_trace_id, scheduled_span_id,
			  attempts, next_attempt_at, last_failure_code, last_failure_reason)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 ON CONFLICT (content_hash) DO NOTHING RETURNING id`

	var insertedID int
	err = r.db.QueryRowContext(ctx, stmt,
		mediaJSON, hashTime.Format(time.RFC3339), StatusPending, contentHash,
		scheduledTraceID, scheduledSpanID,
		1, firstAttemptAt.Format(time.RFC3339), code, reason,
	).Scan(&insertedID)
	if err != nil {
		if err == sql.ErrNoRows {
			// Row already tracked (canonical or retry) — fine.
			r.logger.InfoContext(ctx, "Retry already tracked, skipping",
				slog.String("media", media.Name),
				slog.String("content_hash", contentHash),
			)
			return nil
		}
		return fmt.Errorf("failed to insert retry row: %w", err)
	}

	r.logger.InfoContext(ctx, "Scheduled retry for failed download",
		slog.String("media", media.Name),
		slog.String("code", code),
		slog.String("next_attempt", firstAttemptAt.Format(time.RFC3339)),
	)
	return nil
}

// mediaReleaseTime returns the natural release time of media for content
// hashing. For shows this is the first episode's aired date; for movies the
// FirstAired date. Parsing failures fall back to the zero time (ComputeContentHash
// ignores the time for shows anyway and only formats the date for movies).
func mediaReleaseTime(media tvdb.Media) time.Time {
	if len(media.Metadata.Episodes) > 0 {
		if t, err := time.Parse("2006-01-02", media.Metadata.Episodes[0].Aired); err == nil {
			return t
		}
		return time.Time{}
	}
	if t, err := time.Parse("2006-01-02", media.Metadata.FirstAired); err == nil {
		return t
	}
	return time.Time{}
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
