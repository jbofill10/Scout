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

// FindRowsByItem returns the pending, queued and failed rows for an item. Rows
// are matched in Go by NotificationKey, since the key lives inside the media
// JSON; the controllable set is small, so this is cheaper than an index.
func (r *SchedulerRepo) FindRowsByItem(ctx context.Context, itemKey string) ([]ScheduledRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stmt := `SELECT id, media, release_time, schedule_status, attempts, next_attempt_at
			 FROM ScheduledDownloads
			 WHERE schedule_status IN ($1, $2, $3)
			 ORDER BY id ASC`
	rows, err := r.db.QueryContext(ctx, stmt, controllableStatuses[0], controllableStatuses[1], controllableStatuses[2])
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

// RearmRow makes a pending or failed row due right now. resetAttempts gives a
// permanently failed row a fresh retry budget.
func (r *SchedulerRepo) RearmRow(ctx context.Context, id int, resetAttempts bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads
		 SET schedule_status = $1, next_attempt_at = $2,
		     attempts = CASE WHEN $3 THEN 0 ELSE attempts END
		 WHERE id = $4 AND schedule_status IN ($5, $6)`,
		StatusPending, time.Now().Format(time.RFC3339), resetAttempts, id, StatusPending, StatusFailed)
	if err != nil {
		return fmt.Errorf("failed to re-arm row: %w", err)
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
