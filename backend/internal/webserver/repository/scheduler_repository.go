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

func (r *SchedulerRepo) GetDueMedia(ctx context.Context, windowEnd time.Time) ([]tvdb.Media, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stmt := `SELECT id, media FROM ScheduledDownloads
			 WHERE schedule_status = $1 AND release_time <= $2
			 ORDER BY release_time ASC`
	rows, err := r.db.QueryContext(ctx, stmt, StatusPending, windowEnd.Format(time.RFC3339))
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

	// Mark all retrieved items as queued
	for _, id := range idsToMark {
		_, _ = r.db.ExecContext(ctx, `UPDATE ScheduledDownloads SET schedule_status = $1 WHERE id = $2`, StatusQueued, id)
	}

	return results, nil
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
