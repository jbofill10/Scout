package interactors

import (
	"context"
	"fmt"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/internal/webserver/retry"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	status "github.com/jbofill10/scout/backend/pkg/status"
)

// asyncReconcileInterval is how often failures the torrenter reported on its
// own are picked up for a retry. Minutes matter little here: the retry itself
// is an hour or more out.
const asyncReconcileInterval = 2 * time.Minute

// asyncReconcileBatch bounds one pass. Failed notifications are few.
const asyncReconcileBatch = 200

// downloading reports whether a torrenter response accepted at least one item
// into a download, which is the moment a row must exist to retry from.
func downloading(resp dlstatus.DownloadResponse) bool {
	for _, r := range resp.Results {
		if r.Outcome == dlstatus.OutcomeDownloading {
			return true
		}
	}
	return false
}

// recordDispatched keeps a completed row for a download the torrenter just
// started from a direct request. Without it a loss reported later (monitor
// timeout, dead swarm, torrent removed) has nothing to retry from.
func (i *DownloadInteractor) recordDispatched(ctx context.Context, media tvdb.Media, traceID, spanID string) {
	if err := i.repo.RecordDispatched(ctx, media, traceID, spanID); err != nil {
		i.logger.ErrorContext(ctx, "Failed to record dispatched item", "error", err, "media", media.Name)
	}
}

func (i *DownloadInteractor) runAsyncFailureReconciler(ctx context.Context) {
	// Let the services settle after a boot before the first pass.
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}
	ticker := time.NewTicker(asyncReconcileInterval)
	defer ticker.Stop()
	for {
		if n, err := i.ReconcileAsyncFailures(ctx); err != nil {
			i.logger.ErrorContext(ctx, "Async failure reconciliation failed", "error", err)
		} else if n > 0 {
			i.logger.InfoContext(ctx, "Re-armed items the torrenter lost after the hand-off", "items", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ReconcileAsyncFailures puts items back on the retry ladder when the
// torrenter failed them after the request had been accepted: a download that
// stalled past the monitor timeout, a dead swarm with no replacement, a
// torrent removed from qBittorrent. Those failures arrive through the
// notification (status failed with a transient failure code), not through a
// dispatch response, so nothing else schedules the retry.
//
// For each such notification the newest completed row for the item is
// re-armed with the usual backoff, or marked failed when the ladder is
// exhausted. A movie with no row at all gets a retry row built from the
// notification; an episode with no row cannot be rebuilt and is left as is.
// It returns how many items were acted on.
func (i *DownloadInteractor) ReconcileAsyncFailures(ctx context.Context) (int, error) {
	failed, err := i.notificationRepo.GetNotifications(ctx, asyncReconcileBatch, "", string(notifications.StatusFailed))
	if err != nil {
		return 0, fmt.Errorf("failed notifications: %w", err)
	}
	acted := 0
	for _, n := range failed {
		code := dlstatus.FailureCode(n.FailureCode)
		if code == "" || code.Category() != dlstatus.CategoryTransient {
			continue
		}
		if ctx.Err() != nil {
			return acted, ctx.Err()
		}
		if i.reconcileOne(ctx, n, code) {
			acted++
		}
	}
	return acted, nil
}

func (i *DownloadInteractor) reconcileOne(ctx context.Context, n notifications.Notification, code dlstatus.FailureCode) bool {
	rows, err := i.repo.FindRowsByItem(ctx, n.TvdbID)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to look up rows for a lost download", "error", err, "tvdb_id", n.TvdbID)
		return false
	}
	var last *repository.ScheduledRow
	for idx := range rows {
		switch rows[idx].ScheduleStatus {
		case repository.StatusPending, repository.StatusQueued:
			return false // already on its way back
		case repository.StatusCompleted:
			last = &rows[idx]
		}
	}
	now := time.Now()

	if last == nil {
		if n.Category != "movie" {
			i.warnOnce(ctx, n)
			return false
		}
		// A movie needs nothing beyond its id and name to be requested again.
		media := tvdb.Media{Id: n.TvdbID, Name: n.MediaTitle, Category: "movie", ImageUrl: n.PosterURL, Anime: n.IsAnime}
		next := retry.FirstAttempt(now)
		if err := i.repo.ScheduleRetry(ctx, media, next, string(code), n.Reason, n.TraceID, n.SpanID); err != nil {
			i.logger.ErrorContext(ctx, "Failed to schedule a retry for a lost movie download", "error", err, "tvdb_id", n.TvdbID)
			return false
		}
		i.announceRetry(ctx, n.TvdbID, code, 1, next)
		return true
	}

	next, ok := retry.NextAttempt(now, last.Attempts)
	if !ok {
		reason := fmt.Sprintf("Gave up after %d attempts: %s", retry.MaxAttempts, n.Reason)
		if err := i.repo.FailCompletedRow(ctx, last.ID, string(dlstatus.CodeMaxRetries), reason); err != nil {
			i.logger.ErrorContext(ctx, "Failed to mark a lost download permanently failed", "error", err, "id", last.ID)
			return false
		}
		i.setNotification(ctx, n.TvdbID, notifications.StatusFailed, reason, string(dlstatus.CodeMaxRetries))
		if histErr := i.repo.InsertDownloadHistory(n.MediaTitle, intOf(n.Season), intOf(n.Episode), 0,
			status.Failure, "[Download Failed]: "+reason, n.TraceID, n.SpanID); histErr != nil {
			i.logger.ErrorContext(ctx, "Failed to insert download history", "error", histErr)
		}
		i.logger.WarnContext(ctx, "Lost download exhausted its retries", "tvdb_id", n.TvdbID, "media", n.MediaTitle, "code", code)
		return true
	}
	if err := i.repo.RearmCompletedRow(ctx, last.ID, string(code), n.Reason, next); err != nil {
		i.logger.ErrorContext(ctx, "Failed to re-arm a lost download", "error", err, "id", last.ID)
		return false
	}
	i.announceRetry(ctx, n.TvdbID, code, last.Attempts+1, next)
	return true
}

// announceRetry moves the notification back to searching with the same
// wording the scheduled path uses, and wakes the scheduler so the row's
// timer is armed without waiting for the next poll.
func (i *DownloadInteractor) announceRetry(ctx context.Context, tvdbID string, code dlstatus.FailureCode, attempt int, next time.Time) {
	reason := fmt.Sprintf("%s (attempt %d of %d, next try %s)", code.HumanReason(), attempt, retry.MaxAttempts, next.Format("15:04"))
	i.updateNotificationStatus(ctx, tvdbID, notifications.StatusSearching, reason)
	if i.scheduler != nil {
		i.scheduler.Kick()
	}
	i.logger.InfoContext(ctx, "Lost download will be retried", "tvdb_id", tvdbID, "code", code, "next_attempt", next.Format(time.RFC3339))
}

// warnOnce logs an episode that cannot be retried because no row holds its
// request, once per process rather than on every pass.
func (i *DownloadInteractor) warnOnce(ctx context.Context, n notifications.Notification) {
	if _, seen := i.unrecoverable.LoadOrStore(n.TvdbID, struct{}{}); seen {
		return
	}
	i.logger.WarnContext(ctx, "Lost download cannot be retried automatically: no request row holds the episode",
		"tvdb_id", n.TvdbID, "media", n.MediaTitle, "reason", n.Reason)
}

func intOf(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
