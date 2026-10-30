package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// ScheduledRow is one ScheduledDownloads row with the fields the download
// controls act on. ItemKey is the notification tvdb id for the row's item.
type ScheduledRow struct {
	ID             int
	Media          tvdb.Media
	ItemKey        string
	ScheduleStatus string
	Attempts       int
	ReleaseTime    time.Time
	NextAttemptAt  *time.Time
}

// UpcomingItem is a pending scheduled row as the Activity page lists it:
// something Scout will do later, either at release or at the next retry.
type UpcomingItem struct {
	ID                int        `json:"id"`
	ItemKey           string     `json:"tvdb_id"`
	Title             string     `json:"media_title"`
	Category          string     `json:"category"`
	Season            int        `json:"season"`
	Episode           int        `json:"episode"`
	AbsoluteEpisode   int        `json:"absolute_episode,omitempty"`
	PosterUrl         string     `json:"poster_url,omitempty"`
	IsAnime           bool       `json:"is_anime"`
	ReleaseTime       time.Time  `json:"release_time"`
	NextAttemptAt     *time.Time `json:"next_attempt_at,omitempty"`
	DueAt             time.Time  `json:"due_at"`
	Attempts          int        `json:"attempts"`
	LastFailureCode   string     `json:"last_failure_code,omitempty"`
	LastFailureReason string     `json:"last_failure_reason,omitempty"`
}

// controllableStatuses are the rows a user can still act on.
var controllableStatuses = []string{StatusPending, StatusQueued, StatusFailed}

// lookupStatuses are the rows FindRowsByItem returns: the controllable ones
// plus completed rows, which keep the media payload of a download that was
// handed off and can be re-armed when the torrenter later loses it.
var lookupStatuses = []string{StatusPending, StatusQueued, StatusFailed, StatusCompleted}

// FindRowsByItem returns the pending, queued, failed and completed rows for an
// item, oldest first. Rows are matched in Go by NotificationKey, since the key
// lives inside the media JSON; the set is small, so this is cheaper than an
// index. Callers decide by ScheduleStatus which rows they may act on.
func (r *SchedulerRepo) FindRowsByItem(ctx context.Context, itemKey string) ([]ScheduledRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stmt := `SELECT id, media, release_time, schedule_status, attempts, next_attempt_at
			 FROM ScheduledDownloads
			 WHERE schedule_status IN ($1, $2, $3, $4)
			 ORDER BY id ASC`
	rows, err := r.db.QueryContext(ctx, stmt, lookupStatuses[0], lookupStatuses[1], lookupStatuses[2], lookupStatuses[3])
	if err != nil {
		return nil, fmt.Errorf("failed to query rows for item: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.ErrorContext(ctx, "Failed to close rows", "error", err)
		}
	}()

	var out []ScheduledRow
	for rows.Next() {
		var row ScheduledRow
		var mediaJSON []byte
		var next sql.NullTime
		if err := rows.Scan(&row.ID, &mediaJSON, &row.ReleaseTime, &row.ScheduleStatus, &row.Attempts, &next); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		if err := json.Unmarshal(mediaJSON, &row.Media); err != nil {
			r.logger.ErrorContext(ctx, "Failed to unmarshal media", "id", row.ID, "error", err)
			continue
		}
		row.ItemKey = NotificationKey(row.Media)
		if row.ItemKey != itemKey {
			continue
		}
		if next.Valid {
			t := next.Time
			row.NextAttemptAt = &t
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	return out, nil
}

// CancelRow marks a controllable row cancelled and clears its retry timer. A
// row in any other status is left alone.
func (r *SchedulerRepo) CancelRow(ctx context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads SET schedule_status = $1, next_attempt_at = NULL
		 WHERE id = $2 AND schedule_status IN ($3, $4, $5)`,
		StatusCancelled, id, controllableStatuses[0], controllableStatuses[1], controllableStatuses[2])
	if err != nil {
		return fmt.Errorf("failed to cancel row: %w", err)
	}
	return nil
}

// RearmRow makes a pending, failed or completed row due right now. resetAttempts gives a
// permanently failed row a fresh retry budget.
func (r *SchedulerRepo) RearmRow(ctx context.Context, id int, resetAttempts bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads
		 SET schedule_status = $1, next_attempt_at = $2,
		     attempts = CASE WHEN $3 THEN 0 ELSE attempts END
		 WHERE id = $4 AND schedule_status IN ($5, $6, $7)`,
		StatusPending, time.Now().Format(time.RFC3339), resetAttempts, id, StatusPending, StatusFailed, StatusCompleted)
	if err != nil {
		return fmt.Errorf("failed to re-arm row: %w", err)
	}
	return nil
}

// RearmCompletedRow puts a completed row back on the retry ladder after the
// torrenter lost the download it had started: pending again, one more attempt
// on the count, due at nextAttempt, with the failure recorded on the row.
func (r *SchedulerRepo) RearmCompletedRow(ctx context.Context, id int, code, reason string, nextAttempt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads
		 SET schedule_status = $1, attempts = attempts + 1, next_attempt_at = $2,
		     last_failure_code = $3, last_failure_reason = $4
		 WHERE id = $5 AND schedule_status = $6`,
		StatusPending, nextAttempt.Format(time.RFC3339), code, reason, id, StatusCompleted)
	if err != nil {
		return fmt.Errorf("failed to re-arm completed row: %w", err)
	}
	return nil
}

// FailCompletedRow marks a completed row terminally failed: the download it
// started was lost and the retry ladder is exhausted.
func (r *SchedulerRepo) FailCompletedRow(ctx context.Context, id int, code, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads
		 SET schedule_status = $1, attempts = attempts + 1, next_attempt_at = NULL,
		     last_failure_code = $2, last_failure_reason = $3
		 WHERE id = $4 AND schedule_status = $5`,
		StatusFailed, code, reason, id, StatusCompleted)
	if err != nil {
		return fmt.Errorf("failed to fail completed row: %w", err)
	}
	return nil
}

// RecordDispatched keeps a completed row for an item the torrenter accepted
// straight from a request, so the media payload exists to retry from when the
// download is lost later. A row for the item (canonical or retry) already
// tracking it is left alone.
func (r *SchedulerRepo) RecordDispatched(ctx context.Context, media tvdb.Media, traceID, spanID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaJSON, err := json.Marshal(media)
	if err != nil {
		return fmt.Errorf("failed to marshal media: %w", err)
	}
	hashTime := mediaReleaseTime(media)
	contentHash := ComputeContentHash(media, hashTime)

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO ScheduledDownloads
		 (media, release_time, schedule_status, content_hash, scheduled_trace_id, scheduled_span_id, attempts, queued_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 0, now())
		 ON CONFLICT (content_hash) DO NOTHING`,
		mediaJSON, hashTime.Format(time.RFC3339), StatusCompleted, contentHash, traceID, spanID)
	if err != nil {
		return fmt.Errorf("failed to record dispatched item: %w", err)
	}
	return nil
}

// GetUpcoming returns every pending row, soonest first by its effective due
// time (next retry when one is set, otherwise release).
func (r *SchedulerRepo) GetUpcoming(ctx context.Context) ([]UpcomingItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stmt := `SELECT id, media, release_time, attempts, next_attempt_at, last_failure_code, last_failure_reason,
			        COALESCE(next_attempt_at, release_time) AS due_at
			 FROM ScheduledDownloads
			 WHERE schedule_status = $1
			 ORDER BY COALESCE(next_attempt_at, release_time) ASC, id ASC`
	rows, err := r.db.QueryContext(ctx, stmt, StatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to query upcoming downloads: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.ErrorContext(ctx, "Failed to close rows", "error", err)
		}
	}()

	items := []UpcomingItem{}
	for rows.Next() {
		var item UpcomingItem
		var mediaJSON []byte
		var next sql.NullTime
		var code, reason sql.NullString
		if err := rows.Scan(&item.ID, &mediaJSON, &item.ReleaseTime, &item.Attempts, &next, &code, &reason, &item.DueAt); err != nil {
			return nil, fmt.Errorf("failed to scan upcoming row: %w", err)
		}
		var media tvdb.Media
		if err := json.Unmarshal(mediaJSON, &media); err != nil {
			r.logger.ErrorContext(ctx, "Failed to unmarshal media", "id", item.ID, "error", err)
			continue
		}
		item.ItemKey = NotificationKey(media)
		item.Title = media.Name
		item.Category = media.Category
		item.PosterUrl = media.ImageUrl
		item.IsAnime = media.Anime
		if len(media.Metadata.Episodes) > 0 {
			ep := media.Metadata.Episodes[0]
			item.Season = ep.SeasonNumber
			item.Episode = ep.Number
			item.AbsoluteEpisode = ep.AbsoluteNumber
		}
		if next.Valid {
			t := next.Time
			item.NextAttemptAt = &t
		}
		item.LastFailureCode = code.String
		item.LastFailureReason = reason.String
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating upcoming rows: %w", err)
	}
	return items, nil
}

// TrackedItemKeys returns the notification key of every scheduled row, in any
// status. Cancelled and completed rows count: a follow check must not bring
// back an episode the user stopped or already has.
func (r *SchedulerRepo) TrackedItemKeys(ctx context.Context) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.db.QueryContext(ctx, `SELECT media FROM ScheduledDownloads`)
	if err != nil {
		return nil, fmt.Errorf("failed to query tracked items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			r.logger.ErrorContext(ctx, "Failed to close rows", "error", err)
		}
	}()

	keys := map[string]bool{}
	for rows.Next() {
		var mediaJSON []byte
		if err := rows.Scan(&mediaJSON); err != nil {
			return nil, fmt.Errorf("failed to scan tracked item: %w", err)
		}
		var media tvdb.Media
		if err := json.Unmarshal(mediaJSON, &media); err != nil {
			continue
		}
		keys[NotificationKey(media)] = true
	}
	return keys, rows.Err()
}
