package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// inflightWork is one thing the torrenter is doing for an item that a user may
// want to stop: a running search (infoHash empty) or a monitored download.
type inflightWork struct {
	cancel   context.CancelFunc
	infoHash string
	title    string
	// cancelled is set by CancelItem before cancel is called, so the owner of
	// the work can tell a user cancel from an ordinary context error.
	cancelled atomic.Bool
}

// inflightRegistry tracks cancellable work per item id (the notification tvdb
// id: episode id for series, media id for movies). A nil registry is inert, so
// handlers built by hand in tests need not set one up.
type inflightRegistry struct {
	mu   sync.Mutex
	work map[string]map[*inflightWork]struct{}
}

func newInflightRegistry() *inflightRegistry {
	return &inflightRegistry{work: map[string]map[*inflightWork]struct{}{}}
}

// add registers w under itemID and returns the function that removes it again.
func (r *inflightRegistry) add(itemID string, w *inflightWork) func() {
	if r == nil {
		return func() {}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	set := r.work[itemID]
	if set == nil {
		set = map[*inflightWork]struct{}{}
		r.work[itemID] = set
	}
	set[w] = struct{}{}
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if set, ok := r.work[itemID]; ok {
			delete(set, w)
			if len(set) == 0 {
				delete(r.work, itemID)
			}
		}
	}
}

// take removes and returns everything registered for itemID.
func (r *inflightRegistry) take(itemID string) []*inflightWork {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	set := r.work[itemID]
	delete(r.work, itemID)
	out := make([]*inflightWork, 0, len(set))
	for w := range set {
		out = append(out, w)
	}
	return out
}

// sleepCtx waits for d or until ctx is done, reporting whether the full wait
// elapsed. The monitor loop uses it so a cancel does not have to wait out a
// poll interval.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// CancelItem stops everything in flight for an item: a running search is
// aborted (its result comes back with code cancelled), a monitored torrent is
// stopped and deleted from qBittorrent together with its files, leftover
// ActiveTorrents rows for the item are removed, and the item's notification is
// marked cancelled. It is not an error for there to be nothing to cancel; the
// response says what was found.
func (q *QbittHandler) CancelItem(ctx context.Context, itemID string) (controls.CancelResponse, error) {
	resp := controls.CancelResponse{ItemID: itemID}
	q.cancelInflight(ctx, itemID, &resp)

	// The notification is the webserver's; it may legitimately not exist.
	n, err := q.repo.GetNotification(ctx, itemID)
	if err != nil {
		q.logger.WarnContext(ctx, "Failed to load notification for cancel",
			telemetry.WithTraceContext(ctx, "tvdb_id", itemID, "error", err.Error())...)
	} else if n != nil {
		n.Status = notifications.StatusCancelled
		n.Reason = "Cancelled"
		if err := q.repo.UpdateNotification(ctx, n); err != nil {
			q.logger.ErrorContext(ctx, "Failed to mark notification cancelled",
				telemetry.WithTraceContext(ctx, "tvdb_id", itemID, "error", err.Error())...)
		} else {
			resp.NotificationUpdated = true
		}
	}

	q.logger.InfoContext(ctx, "Cancelled item",
		telemetry.WithTraceContext(ctx, "tvdb_id", itemID, "search_cancelled", resp.SearchCancelled,
			"torrents_removed", resp.TorrentsRemoved, "notification_updated", resp.NotificationUpdated)...)
	return resp, nil
}

// cancelInflight aborts the item's search or monitor and removes its torrents,
// filling in resp. It leaves the notification alone, so a manual grab can
// replace a download without the item flashing "cancelled" in between.
func (q *QbittHandler) cancelInflight(ctx context.Context, itemID string, resp *controls.CancelResponse) {
	removed := map[string]bool{}
	for _, w := range q.inflight.take(itemID) {
		w.cancelled.Store(true)
		w.cancel()
		if w.infoHash == "" {
			resp.SearchCancelled = true
			continue
		}
		if !removed[w.infoHash] {
			removed[w.infoHash] = true
			q.removeTorrent(ctx, w.infoHash, w.title)
			resp.TorrentsRemoved++
		}
	}

	// Rows with no live monitor: the resume after a restart has not reached
	// them yet, or the insert outlived a crashed monitor.
	active, err := q.repo.GetActiveTorrents(ctx)
	if err != nil {
		q.logger.WarnContext(ctx, "Failed to list in-flight torrents for cancel",
			telemetry.WithTraceContext(ctx, "tvdb_id", itemID, "error", err.Error())...)
		return
	}
	for _, at := range active {
		if at.Strategy == nil || at.Strategy.EpisodeTvdbID != itemID || removed[at.InfoHash] {
			continue
		}
		removed[at.InfoHash] = true
		q.removeTorrent(ctx, at.InfoHash, at.TorrentTitle)
		resp.TorrentsRemoved++
	}
}

// removeTorrent deletes a torrent and its files from qBittorrent and forgets
// it as in-flight. Both steps are best-effort: a torrent that is already gone
// from qBittorrent is the outcome we want anyway.
func (q *QbittHandler) removeTorrent(ctx context.Context, infoHash, title string) {
	if err := q.c.DeleteTorrentsCtx(ctx, []string{infoHash}, true); err != nil {
		q.logger.WarnContext(ctx, "Failed to delete cancelled torrent from qBittorrent",
			telemetry.WithTraceContext(ctx, "torrent_hash", infoHash, "torrent_title", title, "error", err.Error())...)
	}
	q.clearActiveTorrent(ctx, infoHash)
	q.logger.InfoContext(ctx, "Removed cancelled torrent",
		telemetry.WithTraceContext(ctx, "torrent_hash", infoHash, "torrent_title", title)...)
}
