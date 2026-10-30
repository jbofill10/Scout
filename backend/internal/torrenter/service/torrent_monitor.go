package service

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/internal/torrenter/searchgate"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golift.io/starr/prowlarr"
)

// ResumeMonitors restarts completion monitors for every torrent still recorded as
// in-flight, and owns closing done once they have all finished. Torrents that
// completed while the process was down are picked up on the resumed monitor's
// first poll. Returns the number of monitors resumed.
func (q *QbittHandler) ResumeMonitors(ctx context.Context, done chan<- models.TorrentCompleteEvent) (int, error) {
	active, err := q.repo.GetActiveTorrents(ctx)
	if err != nil {
		close(done)
		return 0, fmt.Errorf("failed to load in-flight torrents: %w", err)
	}

	if len(active) == 0 {
		q.logger.InfoContext(ctx, "No in-flight torrents to resume")
		close(done)
		return 0, nil
	}

	var wg sync.WaitGroup
	resumed := 0
	for _, at := range active {
		if at.Strategy == nil {
			q.logger.WarnContext(ctx, "Skipping in-flight torrent with no strategy", "torrent_hash", at.InfoHash)
			q.clearActiveTorrent(ctx, at.InfoHash)
			continue
		}

		q.logger.InfoContext(ctx, "Resuming torrent monitor",
			"torrent_title", at.TorrentTitle,
			"torrent_hash", at.InfoHash,
			"started_at", at.StartedAt.Format(time.RFC3339))

		// Only Title and InfoHash are read off the torrent by the monitor; the
		// full Prowlarr result is not worth persisting to rebuild here.
		match := &models.TorrentMatch{
			Strategy: at.Strategy,
			Torrent:  &prowlarr.Search{Title: at.TorrentTitle, InfoHash: at.InfoHash},
		}

		wg.Add(1)
		resumed++
		go q.monitorTorrentCompletion(context.Background(), match, at.Request, at.InfoHash, at.TrackingUUID, at.StartedAt, done, &wg)
	}

	if resumed == 0 {
		close(done)
		return 0, nil
	}

	go func() {
		wg.Wait()
		close(done)
		q.logger.InfoContext(ctx, "All resumed torrent monitors finished, channel closed")
	}()

	return resumed, nil
}

const (
	defaultMonitorTimeout = 6 * time.Hour
	// monitorFastPollWindow is how long we poll frequently before backing off.
	monitorFastPollWindow = 10 * time.Minute
	monitorFastInterval   = 10 * time.Second
	monitorSlowInterval   = 60 * time.Second
	// maxMissedPolls is how many consecutive polls may come back empty before we
	// conclude the torrent was removed from qBittorrent and stop watching. At the
	// 10s fast cadence this is ~5 minutes, wide enough that a qBittorrent slow to
	// surface a freshly added torrent is not mistaken for a deleted one. Query
	// failures are handled separately by retries, so an empty result here means
	// qBittorrent answered and the torrent genuinely was not in the category.
	maxMissedPolls = 30
)

// monitorPoll is the poll cadence in use; tests shorten it.
var monitorPoll = nextPollInterval

// nextPollInterval returns the poll cadence given elapsed time since monitoring began:
// 10s for the first 10 minutes, then 60s thereafter.
func nextPollInterval(elapsed time.Duration) time.Duration {
	if elapsed < monitorFastPollWindow {
		return monitorFastInterval
	}
	return monitorSlowInterval
}

// effectiveStart resolves the instant a monitor's deadline is measured from.
// A resumed monitor supplies its original start so it inherits the deadline it
// was born with; a zero value means "starting now" and must not be read as the
// zero time, which would expire the monitor on its first pass.
func effectiveStart(startedAt time.Time) time.Time {
	if startedAt.IsZero() {
		return time.Now()
	}
	return startedAt
}

// monitorTimeout reads the overall monitor deadline from MONITOR_TIMEOUT (a Go
// duration like "6h"), falling back to 6h when unset or unparseable.
func monitorTimeout() time.Duration {
	if v := os.Getenv("MONITOR_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultMonitorTimeout
}

// monitorTorrentCompletion watches a torrent until it completes and sends the completion event.
//
// A torrent is given up on when it makes no progress for MONITOR_TIMEOUT, not
// after a fixed wall-clock budget: a 60 GB remux that is still moving is a
// slow download, not a lost one. The stall window is measured from when this
// monitor started (now, also for a monitor resumed after a restart); startedAt
// is when the download was first watched and only drives the poll cadence.
func (q *QbittHandler) monitorTorrentCompletion(
	ctx context.Context,
	match *models.TorrentMatch,
	request *models.SearchRequest,
	infoHash string,
	trackingUUID string,
	startedAt time.Time,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	// The winning strategy span represents the (now-complete) search. End it here so the
	// search trace doesn't stay open for the lifetime of this long-lived goroutine.
	searchSpan := trace.SpanFromContext(ctx)
	searchSpanCtx := trace.SpanContextFromContext(ctx)
	searchSpan.End()

	// Start a self-contained monitor span as a new root, linked back to the search span,
	// rather than parenting it under the already-ended search/episode spans. The monitor
	// can run for up to MONITOR_TIMEOUT (default 6h), so its spans must not dangle off an
	// ended parent. Use a background context so it's independent of the HTTP request.
	monitorCtx, monitorSpan := torrentTracer.Start(context.Background(), "monitorTorrent",
		trace.WithNewRoot(),
		trace.WithLinks(trace.Link{SpanContext: searchSpanCtx}),
		trace.WithAttributes(
			attribute.String("torrent_title", match.Torrent.Title),
			attribute.String("torrent_hash", infoHash),
		),
	)
	defer monitorSpan.End()

	// Registered so a user can stop the download; the entry also tells
	// CancelItem which torrent to delete.
	monitorCtx, stopMonitor := context.WithCancel(monitorCtx)
	defer stopMonitor()
	work := &inflightWork{cancel: stopMonitor, infoHash: infoHash, title: match.Torrent.Title}
	unregister := q.inflight.add(match.Strategy.EpisodeTvdbID, work)
	defer unregister()

	start := effectiveStart(startedAt)
	timeout := monitorTimeout()
	q.logger.InfoContext(monitorCtx, "Starting torrent monitor",
		telemetry.WithTraceContext(monitorCtx,
			"torrent_title", match.Torrent.Title,
			"torrent_hash", infoHash,
			"started_at", start.Format(time.RFC3339),
			"timeout", timeout.String())...)

	ranRecheck := false
	missedPolls := 0
	// Progress bookkeeping for dead-swarm detection: a torrent that has made no
	// progress for deadSwarmGrace while connected to no seeds is not going to
	// finish, and is replaced rather than watched until the deadline.
	lastProgress := -1.0
	lastProgressAt := time.Now()
	grace := deadSwarmGrace()
	for {
		elapsed := time.Since(start)

		if !sleepCtx(monitorCtx, monitorPoll(elapsed)) {
			q.logger.InfoContext(monitorCtx, "Torrent monitor stopped: download cancelled",
				telemetry.WithTraceContext(monitorCtx, "torrent_title", match.Torrent.Title, "torrent_hash", infoHash)...)
			monitorSpan.SetStatus(codes.Ok, "cancelled")
			return
		}
		q.logger.InfoContext(monitorCtx, "Monitoring torrent status",
			telemetry.WithTraceContext(monitorCtx,
				"torrent_title", match.Torrent.Title,
				"torrent_hash", infoHash)...)

		filter := qbittorrent.TorrentFilterOptions{
			Category: scoutTag,
			Hashes:   []string{infoHash},
		}

		torrents, err := q.queryQbittorrentWithRetries(monitorCtx, filter, 6, 10*time.Second)
		if err != nil {
			if work.cancelled.Load() {
				monitorSpan.SetStatus(codes.Ok, "cancelled")
				return
			}
			q.logger.WarnContext(monitorCtx, "Failed to monitor torrent after retries, marking failed",
				telemetry.WithTraceContext(monitorCtx,
					"torrent_title", match.Torrent.Title,
					"torrent_hash", infoHash,
					"error", err.Error())...)
			q.failNotification(monitorCtx, match.Strategy.EpisodeTvdbID, dlstatus.CodeMonitorLost, dlstatus.CodeMonitorLost.HumanReason())
			q.recordStrategyFailure(monitorCtx, match.Strategy, "monitor lost: qbittorrent query exhausted")
			q.clearActiveTorrent(monitorCtx, infoHash)
			monitorSpan.SetStatus(codes.Error, "max retries reached")
			return
		}

		if len(torrents) == 0 {
			// A torrent removed from qBittorrent is never coming back, so don't
			// watch an empty result set until the deadline. This matters most for
			// resumed monitors, where the torrent may have been deleted while
			// torrenter was down.
			missedPolls++
			if missedPolls >= maxMissedPolls {
				q.logger.WarnContext(monitorCtx, "Torrent gone from qBittorrent, giving up",
					telemetry.WithTraceContext(monitorCtx,
						"torrent_title", match.Torrent.Title,
						"torrent_hash", infoHash,
						"missed_polls", missedPolls)...)
				q.failNotification(monitorCtx, match.Strategy.EpisodeTvdbID, dlstatus.CodeMonitorLost, dlstatus.CodeMonitorLost.HumanReason())
				q.recordStrategyFailure(monitorCtx, match.Strategy, "monitor lost: torrent no longer in qbittorrent")
				q.recordFailedTorrent(monitorCtx, match, infoHash, "removed from qbittorrent")
				q.clearActiveTorrent(monitorCtx, infoHash)
				monitorSpan.SetStatus(codes.Error, "torrent missing from qbittorrent")
				return
			}
			q.logger.InfoContext(monitorCtx, "Torrent not found, continuing watch",
				"torrent", match.Torrent.Title, "missed_polls", missedPolls)
			continue
		}

		missedPolls = 0
		torrent := torrents[0]
		if torrent.Progress > lastProgress {
			lastProgress = torrent.Progress
			lastProgressAt = time.Now()
		}
		if q.didTorrentComplete(&torrent) {
			if !ranRecheck {
				ranRecheck = true
				if err := q.c.RecheckCtx(monitorCtx, []string{torrent.Hash}); err != nil {
					q.logger.WarnContext(monitorCtx, "Failed to recheck torrent", "hash", torrent.Hash, "error", err)
				}
				continue
			}

			q.logger.InfoContext(monitorCtx, "Torrent completed", "title", match.Torrent.Title)
			monitorSpan.SetStatus(codes.Ok, "torrent downloaded successfully")
			// The ActiveTorrents row is deliberately left in place here: the
			// torrent is not accounted for until the completion event has been
			// processed, and the consumer clears the row once it has been.
			done <- models.TorrentCompleteEvent{
				SavePath: torrent.SavePath,
				Req:      match.Strategy,
				// The monitored hash, not match.Torrent.InfoHash: Prowlarr leaves
				// InfoHash empty for .torrent files and it is resolved from
				// qBittorrent after the add, so the match still carries "".
				Hash:        infoHash,
				UUID:        trackingUUID,
				SpanContext: trace.SpanContextFromContext(monitorCtx),
			}
			break
		} else {
			stalledFor := time.Since(lastProgressAt)
			if isDeadSwarm(stalledFor, torrent.NumSeeds, torrent.DlSpeed, grace) {
				q.logger.WarnContext(monitorCtx, "Torrent has no seeds and has made no progress, giving up on it",
					telemetry.WithTraceContext(monitorCtx,
						"torrent_title", match.Torrent.Title,
						"torrent_hash", infoHash,
						"progress", torrent.Progress*100,
						"stalled_for", stalledFor.Round(time.Second).String(),
						"leechers", torrent.NumLeechs)...)
				reason := fmt.Sprintf("dead swarm: no seeds and no progress for %s", stalledFor.Round(time.Minute))
				q.giveUpOnTorrent(monitorCtx, match, infoHash, reason)
				q.replaceDeadTorrent(monitorCtx, request, match.Strategy, done, wg, "Chosen torrent had no seeds")
				monitorSpan.SetStatus(codes.Error, "dead swarm")
				return
			}
			// Seeds or not, a download that has not moved in MONITOR_TIMEOUT is
			// not going to finish; swap it for another copy like a dead swarm.
			if stalledFor >= timeout {
				q.logger.WarnContext(monitorCtx, "Torrent has made no progress for too long, giving up on it",
					telemetry.WithTraceContext(monitorCtx,
						"torrent_title", match.Torrent.Title,
						"torrent_hash", infoHash,
						"progress", torrent.Progress*100,
						"state", string(torrent.State),
						"stalled_for", stalledFor.Round(time.Second).String())...)
				reason := fmt.Sprintf("no progress for %s", stalledFor.Round(time.Minute))
				q.giveUpOnTorrent(monitorCtx, match, infoHash, reason)
				q.replaceDeadTorrent(monitorCtx, request, match.Strategy, done, wg,
					"The download made no progress for "+timeout.String())
				monitorSpan.SetStatus(codes.Error, "stalled")
				return
			}
			q.logger.InfoContext(monitorCtx, "Torrent downloading",
				telemetry.WithTraceContext(monitorCtx,
					"title", match.Torrent.Title,
					"torrent_hash", infoHash,
					"progress", torrent.Progress*100,
					"state", string(torrent.State),
					"seeds", torrent.NumSeeds,
					"dlspeed", torrent.DlSpeed,
					"stalled_for", stalledFor.Round(time.Second).String())...)
		}
	}
}

// isDeadSwarm decides that a download will not finish: it has sat at the same
// progress for at least grace while connected to no seeds and transferring
// nothing. Leechers alone do not count; with no seed in the swarm the missing
// pieces are not there to be had.
func isDeadSwarm(stalledFor time.Duration, connectedSeeds, dlSpeed int64, grace time.Duration) bool {
	return stalledFor >= grace && connectedSeeds == 0 && dlSpeed == 0
}

// deadSwarmGrace reads how long a download may make no progress with no seeds
// before it is replaced, from DEAD_SWARM_GRACE (a Go duration).
func deadSwarmGrace() time.Duration {
	if v := os.Getenv("DEAD_SWARM_GRACE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultDeadSwarmGrace
}

const (
	defaultDeadSwarmGrace = 15 * time.Minute
	// maxDeadReplacements bounds how many dead torrents are swapped out for one
	// item before it is reported as failed.
	maxDeadReplacements = 3
)

// replaceDeadTorrent searches for another copy of an item whose download was
// given up on (why says what happened, in words the user sees), skipping every
// release already tried, and starts a fresh download and monitor for it. After maxDeadReplacements the item is reported
// as failed instead. Nothing happens for in-flight rows written before the
// search request was recorded.
func (q *QbittHandler) replaceDeadTorrent(
	ctx context.Context,
	request *models.SearchRequest,
	ss *models.SearchStrategy,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
	why string,
) {
	if request == nil {
		q.logger.WarnContext(ctx, "Cannot look for a replacement: no search request recorded for this torrent",
			telemetry.WithTraceContext(ctx, "tvdb_id", ss.EpisodeTvdbID)...)
		q.failNotification(ctx, ss.EpisodeTvdbID, dlstatus.CodeMonitorLost, why)
		return
	}
	if request.Replaced >= maxDeadReplacements {
		reason := fmt.Sprintf("No working copy found after %d tries", request.Replaced+1)
		q.logger.WarnContext(ctx, "Giving up on item: every replacement failed too",
			telemetry.WithTraceContext(ctx, "tvdb_id", ss.EpisodeTvdbID, "tries", request.Replaced+1)...)
		q.failNotification(ctx, ss.EpisodeTvdbID, dlstatus.CodeNoTorrentFound, reason)
		return
	}
	request.Replaced++

	q.updateNotificationStatus(ctx, ss.EpisodeTvdbID, why+", looking for another copy", notifications.StatusSearching)
	q.logger.InfoContext(ctx, "Searching for a replacement torrent",
		telemetry.WithTraceContext(ctx, "media", request.Media.Name, "season", ss.Season, "episode", ss.Episode, "replacement", request.Replaced)...)

	media := request.Media
	var strategies []*models.SearchStrategy
	if request.Episode == nil {
		strategies = q.createMovieSearchStrategy(&media)
	} else {
		strategies = q.createSearchStrategy(&media, request.Episode)
	}
	if len(strategies) == 0 {
		q.failNotification(ctx, ss.EpisodeTvdbID, dlstatus.CodeInvalidMedia, why+" and no other copy could be searched for")
		return
	}

	searchCtx, cancel := context.WithTimeout(ctx, episodeSearchDeadline)
	defer cancel()
	searchCtx = searchgate.WithRequest(searchCtx, searchgate.PriorityFor(media.Source), searchLabel(&media, strategies[0]))
	excluded := q.loadExcludedTorrents(searchCtx, media.Id, ss.Season, ss.Episode)
	outcome := q.searchForMatch(searchCtx, strategies, &media, excluded)
	if outcome.match == nil {
		q.logger.WarnContext(ctx, "No replacement torrent found",
			telemetry.WithTraceContext(ctx, "media", media.Name, "season", ss.Season, "episode", ss.Episode)...)
		q.recordStrategyFailure(ctx, ss, "no replacement torrent found")
		q.failNotification(ctx, ss.EpisodeTvdbID, dlstatus.CodeNoTorrentFound, why+" and no other copy was found")
		return
	}

	q.initiateDownloadAndMonitor(outcome.winningCtx, outcome.match, request, done, wg)
}

func (q *QbittHandler) didTorrentComplete(torrent *qbittorrent.Torrent) bool {
	q.logger.Debug("torrent state", "state", string(torrent.State), "progress", torrent.Progress)
	// Complete if fully downloaded (progress == 1.0) OR the original stalled-upload
	// condition. Using progress catches terminal states other than "stalledUP"
	// (e.g. uploading / pausedUP) that would otherwise be missed.
	if torrent.Progress == 1 {
		return true
	}
	return torrent.Completed == torrent.Size && torrent.State == "stalledUP"
}
