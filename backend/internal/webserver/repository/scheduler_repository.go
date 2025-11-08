package repository

import (
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
	Schedule(media tvdb.Media, releaseTime time.Time, scheduledTraceID, scheduledSpanID string) error
	GetDueMedia() ([]tvdb.Media, error)
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

func (r *SchedulerRepo) Schedule(media tvdb.Media, releaseTime time.Time, scheduledTraceID, scheduledSpanID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaJSON, err := json.Marshal(media)
	if err != nil {
		return fmt.Errorf("failed to marshal media: %w", err)
	}

	// Compute a deterministic content hash of the marshalled media JSON so
	// we can detect duplicate scheduled content efficiently.
	h := sha256.Sum256(mediaJSON)
	contentHash := hex.EncodeToString(h[:])

	stmt := `INSERT INTO ScheduledDownloads (media, release_time, schedule_status, content_hash, scheduled_trace_id, scheduled_span_id)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (content_hash) DO NOTHING RETURNING id`

	var insertedId int
	err = r.db.QueryRow(stmt, mediaJSON, releaseTime.Format(time.RFC3339), StatusPending, contentHash, scheduledTraceID, scheduledSpanID).Scan(&insertedId)
	if err != nil {
		if err == sql.ErrNoRows {
			// ON CONFLICT DO NOTHING caused no insert; treat as duplicate
			return ErrDuplicateScheduled
		}
		return fmt.Errorf("failed to insert scheduled media: %w", err)
	}
	return nil
}

func (r *SchedulerRepo) GetDueMedia() ([]tvdb.Media, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	stmt := `SELECT id, media FROM ScheduledDownloads
			 WHERE date(release_time) = $1 AND schedule_status = 'pending'
			 ORDER BY release_time ASC`
	rows, err := r.db.Query(stmt, today)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.Error("Failed to close rows", "error", err)
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
			r.logger.Error("Failed to unmarshal media", "id", id, "error", err)
			continue
		}
		results = append(results, media)
		idsToMark = append(idsToMark, id)
	}

	// Mark all retrieved items as queued
	for _, id := range idsToMark {
		_, _ = r.db.Exec(`UPDATE ScheduledDownloads SET schedule_status = 'queued' WHERE id = $1`, id)
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
