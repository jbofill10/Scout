package interactors

import (
	"context"
	"log/slog"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/internal/torrenter/service"
	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"go.opentelemetry.io/otel/trace"
)

type DownloadInteractor struct {
	qbitt   service.TorrentService
	mp      service.MediaProcessor
	repo    service.Repository
	library service.LibrarySyncer
	logger  *slog.Logger
}

// NewDownloadInteractor wires the download pipeline. library may be nil, in
// which case the database mirror of Plex only catches up on its own schedule.
func NewDownloadInteractor(
	qbitt service.TorrentService,
	mp service.MediaProcessor,
	repo service.Repository,
	library service.LibrarySyncer,
	logger *slog.Logger,
) *DownloadInteractor {
	return &DownloadInteractor{
		qbitt:   qbitt,
		mp:      mp,
		repo:    repo,
		library: library,
		logger:  logger,
	}
}

// InitiateDownload orchestrates the torrent download process and returns the
// per-episode outcomes so the handler can surface a structured response.
func (i *DownloadInteractor) InitiateDownload(ctx context.Context, req *tvdb.Media) ([]dlstatus.EpisodeResult, error) {
	// Use a buffered channel sized for potential multiple episodes
	// TV shows can have multiple episodes, each spawning a monitoring goroutine
	dlComplete := make(chan models.TorrentCompleteEvent, 10)

	// Start the completion handler before initiating downloads
	// so it's ready to receive events
	go i.handleDownloadCompletion(ctx, dlComplete)

	// HandleDownload owns closing dlComplete: either directly on its early-return
	// paths, or via the background goroutine that waits on the monitoring workers.
	results, err := i.qbitt.HandleDownload(ctx, req, dlComplete)
	if err != nil {
		// On error HandleDownload has not taken ownership of the channel;
		// close it so the completion goroutine exits.
		close(dlComplete)
		return results, err
	}

	return results, nil
}

// Cancel stops everything in flight for an item and marks its notification
// cancelled. The response says what was found; nothing found is not an error.
func (i *DownloadInteractor) Cancel(ctx context.Context, itemID string) (controls.CancelResponse, error) {
	return i.qbitt.CancelItem(ctx, itemID)
}

// ListReleases returns every candidate release the indexers have for an item,
// resolving the media from the request or from the item's in-flight torrent.
func (i *DownloadInteractor) ListReleases(ctx context.Context, req controls.ReleasesRequest) (controls.ReleasesResponse, error) {
	media, err := i.qbitt.ResolveItemMedia(ctx, req.ItemID, req.Media)
	if err != nil {
		return controls.ReleasesResponse{}, err
	}
	return i.qbitt.ListReleases(ctx, req.ItemID, media)
}

// ManualDownload downloads the release the user chose for an item. The
// completion handler runs exactly as for an automatic download, so the file is
// linked into Plex and subtitles are fetched when it lands.
func (i *DownloadInteractor) ManualDownload(ctx context.Context, req controls.ManualDownloadRequest) (dlstatus.EpisodeResult, error) {
	media, err := i.qbitt.ResolveItemMedia(ctx, req.ItemID, req.Media)
	if err != nil {
		return dlstatus.EpisodeResult{}, err
	}

	dlComplete := make(chan models.TorrentCompleteEvent, 1)
	go i.handleDownloadCompletion(ctx, dlComplete)

	// DownloadRelease owns closing dlComplete on every path.
	return i.qbitt.DownloadRelease(ctx, req.ItemID, media, req.Release, dlComplete)
}

// RunCompletionHandler consumes completion events for downloads started
// outside InitiateDownload (upgrades), with the same post-processing.
func (i *DownloadInteractor) RunCompletionHandler(ctx context.Context, dlComplete <-chan models.TorrentCompleteEvent) {
	i.handleDownloadCompletion(ctx, dlComplete)
}

// ResumeInFlightDownloads restarts completion monitors for torrents that were
// still downloading when the process last stopped. Without this a restart
// silently orphans them: qBittorrent finishes the download, but nothing is left
// listening to link it into Plex, and the notification sits on "downloading"
// forever. Torrents that finished during the downtime are caught on the resumed
// monitor's first poll.
func (i *DownloadInteractor) ResumeInFlightDownloads(ctx context.Context) error {
	dlComplete := make(chan models.TorrentCompleteEvent, 10)

	// Start the completion handler before resuming, so it's ready to receive.
	// ResumeMonitors owns closing dlComplete, including when there is nothing
	// to resume, so this goroutine always exits.
	go i.handleDownloadCompletion(ctx, dlComplete)

	resumed, err := i.qbitt.ResumeMonitors(ctx, dlComplete)
	if err != nil {
		return err
	}

	if resumed > 0 {
		i.logger.InfoContext(ctx, "Resumed in-flight torrent monitors", "count", resumed)
	}
	return nil
}

// handleDownloadCompletion processes the completed torrents
// This runs in a goroutine and handles multiple completion events until the channel is closed
func (i *DownloadInteractor) handleDownloadCompletion(ctx context.Context, dlComplete <-chan models.TorrentCompleteEvent) {
	// Process all events until the channel is closed
	for event := range dlComplete {
		// Create a new background context with the span context from the event
		// This allows us to continue the trace from the search strategy that initiated the download
		eventCtx := trace.ContextWithSpanContext(context.Background(), event.SpanContext)

		i.logger.InfoContext(eventCtx, "Processing completed torrent", "hash", event.Hash)

		if err := i.mp.ProcessDownloadedTorrent(eventCtx, &event); err != nil {
			i.logger.ErrorContext(eventCtx, "Error processing downloaded torrent", "error", err, "hash", event.Hash)
			if err2 := i.repo.UpdateDownloadHistoryStatus(eventCtx, event.Hash, "failure", err.Error()); err2 != nil {
				i.logger.ErrorContext(eventCtx, "Failed to update download history", "error", err2)
			}
			// Update notification to failed
			i.updateNotificationFailed(eventCtx, &event, "Processing error: "+err.Error())
			if event.Req != nil && event.Req.Upgrade != nil {
				if uerr := i.repo.SetUpgradeAttemptStatus(eventCtx, event.Req.EpisodeTvdbID, "failed", err.Error()); uerr != nil {
					i.logger.ErrorContext(eventCtx, "Failed to record upgrade failure", "error", uerr)
				}
			}
			// The torrent is accounted for even though processing failed: the
			// outcome is recorded, and retrying it on every restart would only
			// replay the same failure.
			i.clearActiveTorrent(eventCtx, event.Hash)
			continue
		}

		// The file is in the library folder; have the database mirror catch up
		// so the item reads as downloaded without waiting for a restart.
		if i.library != nil {
			i.library.RequestSync()
		}

		if err := i.repo.UpdateDownloadHistoryStatus(eventCtx, event.Hash, "success", ""); err != nil {
			i.logger.ErrorContext(eventCtx, "Failed to update download history", "error", err, "hash", event.Hash)
		}

		// Update notification to completed
		if event.Req != nil && event.Req.Pack != nil {
			i.settlePackNotifications(eventCtx, &event)
		} else {
			i.updateNotificationCompleted(eventCtx, &event)
		}
		if event.Req != nil && event.Req.Upgrade != nil {
			if uerr := i.repo.SetUpgradeAttemptStatus(eventCtx, event.Req.EpisodeTvdbID, "completed", ""); uerr != nil {
				i.logger.ErrorContext(eventCtx, "Failed to record upgrade completion", "error", uerr)
			}
		}

		// The torrent is fully accounted for; stop tracking it as in-flight.
		i.clearActiveTorrent(eventCtx, event.Hash)

		// Remove UUID tracking tag to prevent tag bloat
		if err := i.qbitt.RemoveUUIDTag(eventCtx, event.Hash, event.UUID); err != nil {
			i.logger.WarnContext(eventCtx, "Failed to remove UUID tag (non-fatal)", "error", err, "hash", event.Hash, "uuid", event.UUID)
		}
	}

	i.logger.InfoContext(ctx, "Download completion handler exiting")
}

// clearActiveTorrent stops tracking a torrent as in-flight once its completion
// event has been handled, either way. Best-effort: a leftover row only costs one
// wasted resume attempt on the next boot.
func (i *DownloadInteractor) clearActiveTorrent(ctx context.Context, hash string) {
	if err := i.repo.DeleteActiveTorrent(ctx, hash); err != nil {
		i.logger.ErrorContext(ctx, "Failed to clear in-flight torrent record", "error", err, "hash", hash)
	}
}

// updateNotificationCompleted updates notification to completed stage (non-blocking)
func (i *DownloadInteractor) updateNotificationCompleted(ctx context.Context, event *models.TorrentCompleteEvent) {
	if event.Req == nil {
		i.logger.WarnContext(ctx, "Cannot update notification: strategy is nil")
		return
	}

	ss := event.Req

	// Get notification by episode tvdb id (episode ID for series, movie ID for movies)
	notification, err := i.repo.GetNotification(ctx, ss.EpisodeTvdbID)

	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to get notification for completion update",
			"error", err,
			"tvdb_id", ss.EpisodeTvdbID,
			"season", ss.Season,
			"episode", ss.Episode,
			"is_movie", ss.IsMovie)
		return
	}

	if notification == nil {
		i.logger.WarnContext(ctx, "No notification found for completion (webserver should have created it)",
			"tvdb_id", ss.EpisodeTvdbID,
			"season", ss.Season,
			"episode", ss.Episode,
			"is_movie", ss.IsMovie)
		return
	}

	// Update notification fields
	notification.Status = notifications.StatusCompleted
	notification.Reason = ""
	notification.FailureCode = ""

	err = i.repo.UpdateNotification(ctx, notification)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to update notification to completed status",
			"error", err,
			"notification_id", notification.ID)
		return
	}

	i.logger.InfoContext(ctx, "Updated notification to completed status",
		"notification_id", notification.ID,
		"status", "completed")
}

// updateNotificationFailed updates notification to failed stage (non-blocking)
func (i *DownloadInteractor) updateNotificationFailed(ctx context.Context, event *models.TorrentCompleteEvent, reason string) {
	if event.Req == nil {
		i.logger.WarnContext(ctx, "Cannot update notification: strategy is nil")
		return
	}

	ss := event.Req

	// Get notification by episode tvdb id (episode ID for series, movie ID for movies)
	notification, err := i.repo.GetNotification(ctx, ss.EpisodeTvdbID)

	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to get notification for failure update",
			"error", err,
			"tvdb_id", ss.EpisodeTvdbID,
			"season", ss.Season,
			"episode", ss.Episode,
			"is_movie", ss.IsMovie)
		return
	}

	if notification == nil {
		i.logger.WarnContext(ctx, "No notification found for failure (webserver should have created it)",
			"tvdb_id", ss.EpisodeTvdbID,
			"season", ss.Season,
			"episode", ss.Episode,
			"is_movie", ss.IsMovie)
		return
	}

	// Update notification fields
	notification.Status = notifications.StatusFailed
	notification.Reason = reason
	// The files are on disk and need a person; the webserver must not retry.
	notification.FailureCode = string(dlstatus.CodeProcessingError)

	err = i.repo.UpdateNotification(ctx, notification)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to update notification to failed status",
			"error", err,
			"notification_id", notification.ID)
		return
	}

	i.logger.InfoContext(ctx, "Updated notification to failed status",
		"notification_id", notification.ID,
		"status", "failed",
		"reason", reason)
}

// settlePackNotifications marks each episode of a season pack by what the
// processor found in the torrent: linked ones complete, the rest fail with
// the reason, so a pack that was short an episode is visible per episode.
func (i *DownloadInteractor) settlePackNotifications(ctx context.Context, event *models.TorrentCompleteEvent) {
	for _, id := range event.Req.Pack.EpisodeIDs() {
		outcome := event.PackResults[id]
		n, err := i.repo.GetNotification(ctx, id)
		if err != nil || n == nil {
			continue
		}
		if outcome == "linked" {
			n.Status = notifications.StatusCompleted
			n.Reason = ""
			n.FailureCode = ""
		} else {
			n.Status = notifications.StatusFailed
			if outcome == "" {
				outcome = "not in the season pack"
			}
			n.Reason = outcome
			// Transient: the webserver retries the episode on its own.
			n.FailureCode = string(dlstatus.CodeNoTorrentFound)
		}
		if err := i.repo.UpdateNotification(ctx, n); err != nil {
			i.logger.ErrorContext(ctx, "Failed to settle pack episode notification", "error", err, "tvdb_id", id)
		}
	}
}
