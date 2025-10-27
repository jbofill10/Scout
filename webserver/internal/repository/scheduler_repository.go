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

	tvdb "shared/media"

	_ "github.com/lib/pq"
)

// Schedule status constants
const (
	StatusPending = "pending"
	StatusQueued  = "queued"
)

// SchedulerRepository defines DB operations for scheduled shows
type SchedulerRepository interface {
	Schedule(media tvdb.Media, releaseTime time.Time) error
	GetDueMedia() ([]tvdb.Media, error)
	InsertDownloadHistory(mediaTitle string, season, episode, absoluteEpisode int, status, reason string) error
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

func (r *SchedulerRepo) Schedule(media tvdb.Media, releaseTime time.Time) error {
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

	stmt := `INSERT INTO ScheduledDownloads (media, release_time, schedule_status, content_hash)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (content_hash) DO NOTHING RETURNING id`

	var insertedId int
	err = r.db.QueryRow(stmt, mediaJSON, releaseTime.Format(time.RFC3339), StatusPending, contentHash).Scan(&insertedId)
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

func (r *SchedulerRepo) InsertDownloadHistory(mediaTitle string, season, episode, absoluteEpisode int, status, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.Exec(`
		INSERT INTO ShowDownloadHistory (mediaTitle, season, episode, absoluteEpisode, status, reason)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, mediaTitle, season, episode, absoluteEpisode, status, reason)
	if err != nil {
		return fmt.Errorf("failed to insert download history: %w", err)
	}
	return nil
}
