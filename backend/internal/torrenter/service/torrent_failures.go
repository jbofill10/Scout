package service

import (
	"context"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// recordStrategyFailure inserts a download-history failure row from a search strategy alone
// (used by the monitor, which has no *tvdb.Media). Nil-safe for movies (EpisodeMeta == nil).
func (q *QbittHandler) recordStrategyFailure(ctx context.Context, ss *models.SearchStrategy, reason string) {
	if ss == nil {
		return
	}
	absoluteNumber := 0
	if ss.EpisodeMeta != nil {
		absoluteNumber = ss.EpisodeMeta.AbsoluteNumber
	}
	if err := q.repo.InsertDownloadHistory(ctx, ss.MediaName, ss.Season, ss.Episode, absoluteNumber, "", "failure", reason); err != nil {
		q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
	}
}

// recordDownloadFailure inserts a download-history failure row, preserving the prior behavior of
// always recording failures for observability.
func (q *QbittHandler) recordDownloadFailure(ctx context.Context, req *tvdb.Media, ss *models.SearchStrategy, reason string) {
	absoluteNumber := 0
	if ss.EpisodeMeta != nil {
		absoluteNumber = ss.EpisodeMeta.AbsoluteNumber
	}
	if err := q.repo.InsertDownloadHistory(ctx, req.Name, ss.Season, ss.Episode, absoluteNumber, "", "failure", reason); err != nil {
		q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
	}
}

// failedResult builds a failed EpisodeResult for a search strategy and failure code.
func (q *QbittHandler) failedResult(ss *models.SearchStrategy, code dlstatus.FailureCode) dlstatus.EpisodeResult {
	return dlstatus.EpisodeResult{
		TvdbID:  ss.EpisodeTvdbID,
		Season:  ss.Season,
		Episode: ss.Episode,
		Outcome: dlstatus.OutcomeFailed,
		Code:    code,
		Reason:  code.HumanReason(),
	}
}

// applyFailureNotification updates the episode notification for a failure. Permanent failures move the
// notification to "failed"; transient failures keep it in "searching" with the human-readable reason,
// because the webserver's retry engine (Phase 2) owns the final failure decision.
func (q *QbittHandler) applyFailureNotification(ctx context.Context, tvdbID string, code dlstatus.FailureCode) {
	if code.Category() == dlstatus.CategoryPermanent {
		q.updateNotificationStatus(ctx, tvdbID, code.HumanReason(), notifications.StatusFailed)
		return
	}
	q.updateNotificationStatus(ctx, tvdbID, code.HumanReason(), notifications.StatusSearching)
}

// recordFailedTorrent remembers a torrent that was given up on for an item, so
// later searches for it skip the release.
func (q *QbittHandler) recordFailedTorrent(ctx context.Context, match *models.TorrentMatch, infoHash, reason string) {
	ss := match.Strategy
	if ss == nil {
		return
	}
	ft := &models.FailedTorrent{
		TvdbID:   ss.TvdbId,
		Season:   ss.Season,
		Episode:  ss.Episode,
		InfoHash: infoHash,
		Title:    match.Torrent.Title,
		Reason:   reason,
	}
	if err := q.repo.InsertFailedTorrent(ctx, ft); err != nil {
		q.logger.ErrorContext(ctx, "Failed to record failed torrent",
			telemetry.WithTraceContext(ctx, "error", err.Error(), "torrent_title", match.Torrent.Title)...)
	}
}

// giveUpOnTorrent removes a download that will not finish: out of qBittorrent
// with its files, into the failed list, and off the in-flight table.
func (q *QbittHandler) giveUpOnTorrent(ctx context.Context, match *models.TorrentMatch, infoHash, reason string) {
	if err := q.c.DeleteTorrentsCtx(ctx, []string{infoHash}, true); err != nil {
		q.logger.ErrorContext(ctx, "Failed to remove dead torrent from qBittorrent",
			telemetry.WithTraceContext(ctx, "error", err.Error(), "torrent_hash", infoHash)...)
	}
	q.recordStrategyFailure(ctx, match.Strategy, reason)
	q.recordFailedTorrent(ctx, match, infoHash, reason)
	q.clearActiveTorrent(ctx, infoHash)
}
