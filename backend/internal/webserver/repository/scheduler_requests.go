package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/lib/pq"
)

// EnqueueNow records a request the torrenter is about to be asked for as a
// queued row, so a restart before the answer finds the row and dispatches it
// again instead of leaving the notification on "searching" for good. The row
// is keyed like a scheduled row for the same item, so an existing row in any
// status but queued is taken over: this request's payload and trace, a fresh
// retry ladder, due now. A row already queued belongs to a dispatch under way,
// and ok=false reports that nothing new was started.
func (r *SchedulerRepo) EnqueueNow(ctx context.Context, media tvdb.Media, traceID, spanID string) (DueItem, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaJSON, err := json.Marshal(media)
	if err != nil {
		return DueItem{}, false, fmt.Errorf("failed to marshal media: %w", err)
	}
	hashTime := mediaReleaseTime(media)
	contentHash := ComputeContentHash(media, hashTime)

	stmt := `INSERT INTO ScheduledDownloads
			 (media, release_time, schedule_status, content_hash, scheduled_trace_id, scheduled_span_id,
			  attempts, next_attempt_at, queued_at)
			 VALUES ($1, $2, $3, $4, $5, $6, 0, now(), now())
			 ON CONFLICT (content_hash) DO UPDATE SET
			     media = EXCLUDED.media, schedule_status = EXCLUDED.schedule_status,
			     scheduled_trace_id = EXCLUDED.scheduled_trace_id, scheduled_span_id = EXCLUDED.scheduled_span_id,
			     attempts = 0, next_attempt_at = now(), queued_at = now(),
			     last_failure_code = NULL, last_failure_reason = NULL
			 WHERE ScheduledDownloads.schedule_status <> EXCLUDED.schedule_status
			 RETURNING id, release_time`

	item := DueItem{Media: media, DueAt: time.Now()}
	err = r.db.QueryRowContext(ctx, stmt,
		mediaJSON, hashTime.Format(time.RFC3339), StatusQueued, contentHash, traceID, spanID,
	).Scan(&item.ID, &item.ReleaseTime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.logger.InfoContext(ctx, "Item is already being dispatched, not enqueued again",
				"media", media.Name, "content_hash", contentHash)
			return DueItem{}, false, nil
		}
		return DueItem{}, false, fmt.Errorf("failed to enqueue request: %w", err)
	}
	return item, true, nil
}

// TouchQueued refreshes queued_at on rows still waiting their turn in a
// dispatch that is making progress, so ResetStaleQueued does not take a long
// group for a crashed one and hand its tail to a second dispatch.
func (r *SchedulerRepo) TouchQueued(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads SET queued_at = now() WHERE schedule_status = $1 AND id = ANY($2)`,
		StatusQueued, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("failed to touch queued rows: %w", err)
	}
	return nil
}

// DeferRow returns a queued row to pending, due at next, without counting an
// attempt: the torrenter could not be reached, so the request was never tried.
func (r *SchedulerRepo) DeferRow(ctx context.Context, id int, code, reason string, next time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`UPDATE ScheduledDownloads
		 SET schedule_status = $1, next_attempt_at = $2, last_failure_code = $3, last_failure_reason = $4
		 WHERE id = $5 AND schedule_status = $6`,
		StatusPending, next.Format(time.RFC3339), code, reason, id, StatusQueued)
	if err != nil {
		return fmt.Errorf("failed to defer row: %w", err)
	}
	return nil
}
