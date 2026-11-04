package interactors

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/internal/webserver/retry"
	"github.com/jbofill10/scout/backend/internal/webserver/scheduler"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/settings"
	status "github.com/jbofill10/scout/backend/pkg/status"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("webserver")

// DownloadDispatcher dispatches a media download request to the torrenter and
// returns the structured per-episode outcomes. *clients.TorrenterClient
// satisfies this; tests substitute a fake.
type DownloadDispatcher interface {
	Download(ctx context.Context, req tvdb.Media) (dlstatus.DownloadResponse, error)
}

// DownloadSummary describes what a download request actually did, so the caller
// can tell the user something concrete instead of a bare "request sent". The
// torrent search itself runs in the background, so QueuedNow counts the items
// handed off to the torrenter rather than items that finished downloading.
type DownloadSummary struct {
	MediaTitle string `json:"media_title"`
	Category   string `json:"category"`
	// QueuedNow is how many episodes (or movies) were dispatched for an
	// immediate torrent search.
	QueuedNow int `json:"queued_now"`
	// Scheduled is how many were stored for a future release date.
	Scheduled int `json:"scheduled"`
	// Skipped is how many were ignored: specials, unparseable air dates,
	// already-scheduled duplicates, or scheduling errors.
	Skipped int `json:"skipped"`
	// InProgress is how many Scout was already searching for or downloading,
	// so nothing new was started for them.
	InProgress int `json:"in_progress"`
	// NextRelease is the earliest release time among the scheduled items.
	NextRelease *time.Time `json:"next_release,omitempty"`
	// TraceID correlates this request with its logs and spans.
	TraceID string `json:"trace_id,omitempty"`
}

type DownloadInteractor struct {
	scheduler        *scheduler.Scheduler
	repo             repository.SchedulerRepository
	notificationRepo repository.NotificationRepository
	logger           *slog.Logger
	mediaQueue       chan repository.DueItem
	tvdbClient       *clients.TVDBProxyClient
	torrenterClient  DownloadDispatcher
	// dispatch runs the torrenter hand-off. It defaults to spawning a goroutine
	// so the HTTP request returns immediately; tests substitute a synchronous
	// runner.
	dispatch func(func())
	// unrecoverable remembers lost episodes with no row to retry from, so
	// they are logged once rather than every reconciliation pass.
	unrecoverable sync.Map
}

func NewDownloadInteractor(
	repo repository.SchedulerRepository,
	notificationRepo repository.NotificationRepository,
	sched *scheduler.Scheduler,
	mediaQueue chan repository.DueItem,
	logger *slog.Logger,
	tvdbClient *clients.TVDBProxyClient,
	torrenterClient DownloadDispatcher,
) *DownloadInteractor {
	return &DownloadInteractor{
		repo:             repo,
		notificationRepo: notificationRepo,
		scheduler:        sched,
		mediaQueue:       mediaQueue,
		logger:           logger,
		tvdbClient:       tvdbClient,
		torrenterClient:  torrenterClient,
		dispatch:         func(fn func()) { go fn() },
	}
}

// RunDispatchInline makes the torrenter hand-off synchronous. Tests use it so
// the background dispatch is observable without waiting on a goroutine.
func (i *DownloadInteractor) RunDispatchInline() {
	i.dispatch = func(fn func()) { fn() }
}

// WatchForDueMedia starts the scheduler and processes due media from the queue
func (i *DownloadInteractor) WatchForDueMedia() {
	i.scheduler.Start(i.mediaQueue)
	go func() {
		for item := range i.mediaQueue {
			i.processDueBatch(item)
		}
	}()
	go i.runAsyncFailureReconciler(context.Background())
}

// processDueItem dispatches a single row and transitions it to a terminal
// status (completed/failed), reschedules it for retry, or defers it when the
// torrenter could not be reached. The scheduler calls it with a background
// context (the scheduled_trace_id column correlates back to the request that
// scheduled the row); a direct request passes its own, detached from
// cancellation, so the dispatch stays in the request's trace.
func (i *DownloadInteractor) processDueItem(ctx context.Context, item repository.DueItem) {
	media := item.Media
	ctx, span := tracer.Start(ctx, "scheduled_download",
		trace.WithAttributes(
			attribute.String("media.name", media.Name),
			attribute.String("media.id", media.Id),
			attribute.Int("episode.count", len(media.Metadata.Episodes)),
		),
	)
	defer span.End()

	i.logger.InfoContext(ctx, "Processing due media", "media", media.Name, "attempts", item.Attempts)

	traceID, spanID := telemetry.GetTraceSpanIDs(ctx)

	resp, err := i.torrenterClient.Download(ctx, media)
	if torrenterDown(err) {
		i.deferRows(ctx, []repository.DueItem{item}, err)
		return
	}
	if err != nil {
		i.logger.ErrorContext(ctx, "Download dispatch error", "error", err, "media", media.Name)
	}

	i.settleDueRow(ctx, item, failedResult(resp.Results), traceID, spanID)
}

// settleDueRow transitions a dispatched row from the failure (if any) the
// torrenter reported for it: completed, left as cancelled, permanently failed,
// or rescheduled with backoff.
func (i *DownloadInteractor) settleDueRow(
	ctx context.Context, item repository.DueItem, failed *dlstatus.EpisodeResult, traceID, spanID string,
) {
	media := item.Media
	if failed == nil {
		// No failed results → every episode is downloading/exists, so the row is
		// done. (A transport error still populates resp.Results via
		// unreachableResponse, so a nil-failed result here genuinely means
		// success; the empty-results case is only a defensive fallback.)
		if markErr := i.repo.MarkCompleted(ctx, item.ID); markErr != nil {
			i.logger.ErrorContext(ctx, "Failed to mark download completed", "error", markErr, "id", item.ID)
		}
		i.logger.InfoContext(ctx, "Scheduled download completed", "media", media.Name)
		return
	}

	code := failed.Code
	reason := failed.Reason
	if reason == "" {
		reason = code.HumanReason()
	}

	if code == dlstatus.CodeCancelled {
		// The user cancelled the item while it was being dispatched; the row
		// and the notification already say so, and no retry is wanted.
		i.logger.InfoContext(ctx, "Scheduled download was cancelled during dispatch", "media", media.Name)
		return
	}

	if code.Category() == dlstatus.CategoryPermanent {
		i.handlePermanentRowFailure(ctx, item, *failed, reason, traceID, spanID)
		return
	}

	// Transient failure → apply backoff policy.
	now := time.Now()
	maxAttempts := retry.MaxAttemptsFor(code, airedAt(media), now)
	next, ok := retry.NextAttemptWithin(now, item.Attempts, maxAttempts)
	if !ok {
		// Exhausted retries → terminal failure.
		exhaustReason := fmt.Sprintf("Gave up after %d attempts: %s", maxAttempts, reason)
		i.handlePermanentRowFailureWithCode(ctx, item, *failed, string(dlstatus.CodeMaxRetries), exhaustReason, traceID, spanID)
		return
	}

	// Retries remain → reschedule the row and keep the notification searching.
	if recErr := i.repo.RecordFailure(ctx, item.ID, string(code), reason, next); recErr != nil {
		i.logger.ErrorContext(ctx, "Failed to record transient failure", "error", recErr, "id", item.ID)
	}
	attemptReason := fmt.Sprintf("%s (attempt %d of %d, next try %s)",
		code.HumanReason(), item.Attempts+1, maxAttempts, next.Format("15:04"))
	i.updateNotificationStatus(ctx, failed.TvdbID, notifications.StatusSearching, attemptReason)
	i.logger.InfoContext(ctx, "Scheduled download will retry",
		"media", media.Name, "code", code, "next_attempt", next.Format(time.RFC3339))
}

// handlePermanentRowFailure marks the row failed using the failure's own code.
func (i *DownloadInteractor) handlePermanentRowFailure(ctx context.Context, item repository.DueItem, failed dlstatus.EpisodeResult, reason, traceID, spanID string) {
	i.handlePermanentRowFailureWithCode(ctx, item, failed, string(failed.Code), reason, traceID, spanID)
}

// handlePermanentRowFailureWithCode marks the row terminally failed, then
// surfaces the failure to the user (failed notification + history row).
func (i *DownloadInteractor) handlePermanentRowFailureWithCode(ctx context.Context, item repository.DueItem, failed dlstatus.EpisodeResult, code, reason, traceID, spanID string) {
	if markErr := i.repo.MarkPermanentlyFailed(ctx, item.ID, code, reason); markErr != nil {
		i.logger.ErrorContext(ctx, "Failed to mark download permanently failed", "error", markErr, "id", item.ID)
	}
	i.surfacePermanentFailure(ctx, item.Media.Name, failed, code, reason, traceID, spanID)
	i.logger.ErrorContext(ctx, "Scheduled download permanently failed",
		"media", item.Media.Name, "code", code, "reason", reason)
}

// surfacePermanentFailure updates the episode/movie notification to failed and
// records a history failure row. Shared by the scheduled-dispatch and both
// immediate-download paths so the "notify + history" pattern lives in one place.
func (i *DownloadInteractor) surfacePermanentFailure(ctx context.Context, mediaTitle string, failed dlstatus.EpisodeResult, code, reason, traceID, spanID string) {
	i.setNotification(ctx, failed.TvdbID, notifications.StatusFailed, reason, code)
	if histErr := i.repo.InsertDownloadHistory(mediaTitle, failed.Season, failed.Episode, 0,
		status.Failure, "[Download Failed]: "+reason, traceID, spanID); histErr != nil {
		i.logger.ErrorContext(ctx, "Failed to insert download history", "error", histErr)
	}
}

// failedResult picks the representative failed result from a download response.
// Rows are per-episode (or per-movie) in practice, so 0-1 failures are expected;
// if multiple, the first permanent failure wins, else the first transient one.
func failedResult(results []dlstatus.EpisodeResult) *dlstatus.EpisodeResult {
	var firstTransient *dlstatus.EpisodeResult
	for idx := range results {
		r := &results[idx]
		if r.Outcome != dlstatus.OutcomeFailed {
			continue
		}
		if r.Code.Category() == dlstatus.CategoryPermanent {
			return r
		}
		if firstTransient == nil {
			firstTransient = r
		}
	}
	return firstTransient
}

// updateNotificationStatus updates an existing notification (by tvdb_id) to the
// given status and reason. Missing notifications are logged, not fatal.
func (i *DownloadInteractor) updateNotificationStatus(ctx context.Context, tvdbID string, st notifications.NotificationStatus, reason string) {
	i.setNotification(ctx, tvdbID, st, reason, "")
}

// setNotification writes a status, its reason and, for a failure, the
// dlstatus code behind it; any other transition clears the code.
func (i *DownloadInteractor) setNotification(ctx context.Context, tvdbID string, st notifications.NotificationStatus, reason, failureCode string) {
	notification, err := i.notificationRepo.GetNotification(ctx, tvdbID)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to load notification for update", "error", err, "tvdb_id", tvdbID)
		return
	}
	if notification == nil {
		i.logger.WarnContext(ctx, "No notification to update", "tvdb_id", tvdbID)
		return
	}
	notification.Status = st
	notification.Reason = reason
	notification.FailureCode = failureCode
	if err := i.notificationRepo.UpdateNotification(ctx, notification); err != nil {
		i.logger.ErrorContext(ctx, "Failed to update notification", "error", err, "tvdb_id", tvdbID)
	}
}

func (i *DownloadInteractor) extractAliases(aliases []tvdb.Alias) []string {
	var aliasNames []string
	for _, alias := range aliases {
		aliasNames = append(aliasNames, alias.Name)
	}
	return aliasNames
}

// DownloadShow handles the download request for a TV show. It returns as soon
// as every episode has been classified (queued / scheduled / skipped) and a
// notification exists for it; the torrent search runs in the background and
// reports progress through those notifications.
func (i *DownloadInteractor) DownloadShow(ctx context.Context, req tvdb.Media) (DownloadSummary, error) {
	traceID, _ := telemetry.GetTraceSpanIDs(ctx)
	summary := DownloadSummary{MediaTitle: req.Name, Category: "series", TraceID: traceID}

	extendedInfo, err := i.getExtendedInformation(ctx, req.Id, "series")
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to get extended information", "error", err)
		return summary, err
	}

	isAnime := isMediaAnime(extendedInfo)

	today := time.Now()
	i.logger.InfoContext(ctx, "Download request",
		"media_id", req.Id,
		"media_name", req.Name,
		"category", req.Category,
		"anime", isAnime,
		"episode_count", len(req.Metadata.Episodes))
	toDispatch := []repository.DueItem{}

	ids := make([]string, 0, len(req.Metadata.Episodes))
	for _, episode := range req.Metadata.Episodes {
		ids = append(ids, strconv.Itoa(episode.Id))
	}
	inProgress := i.inProgressItems(ctx, ids)

	for _, episode := range req.Metadata.Episodes {
		// Create a child span for each episode
		episodeCtx, episodeSpan := tracer.Start(ctx, fmt.Sprintf("%s S%dE%d", req.Name, episode.SeasonNumber, episode.Number),
			trace.WithAttributes(
				attribute.String("media.name", req.Name),
				attribute.Int("episode.season", episode.SeasonNumber),
				attribute.Int("episode.number", episode.Number),
				attribute.String("episode.aired", episode.Aired),
				attribute.Bool("anime", isAnime),
				attribute.Int("episode.count", len(req.Metadata.Episodes)),
			),
		)

		if episode.SeasonNumber == 0 {
			// Temporary, skip specials
			i.logger.InfoContext(episodeCtx, "Skipping special episode", "episode", episode.Number)
			summary.Skipped++
			episodeSpan.End()
			continue
		}

		if inProgress[strconv.Itoa(episode.Id)] {
			// Scout is already on it: a second request would only add a
			// duplicate notification and a duplicate search.
			i.logger.InfoContext(episodeCtx, "Episode already in progress, skipping",
				"season", episode.SeasonNumber, "episode", episode.Number)
			episodeSpan.SetAttributes(attribute.String("episode.status", "in_progress"))
			summary.InProgress++
			episodeSpan.End()
			continue
		}

		episodeAired, err := time.Parse("2006-01-02", episode.Aired)
		if err != nil {
			i.logger.WarnContext(episodeCtx, "Failed to parse episode aired date", "error", err)
			summary.Skipped++
			episodeSpan.End()
			continue
		}

		if isAnime {
			episodeSpan.SetAttributes(attribute.Int("episode.absolute", episode.AbsoluteNumber))
		}

		// Get trace/span IDs for this episode
		traceID, spanID := telemetry.GetTraceSpanIDs(episodeCtx)

		// Has the episode aired yet?
		if today.After(episodeAired) {
			// The request is written down before the torrenter is asked, so a
			// restart mid-dispatch finds the row and sends it again.
			rowMedia := i.buildScheduledMedia(req, extendedInfo, isAnime, episode)
			rowMedia.Source = req.Source
			item, started, err := i.repo.EnqueueNow(episodeCtx, rowMedia, traceID, spanID)
			if err != nil {
				summary.Skipped++
				episodeSpan.SetAttributes(attribute.String("error", err.Error()))
				i.logger.ErrorContext(episodeCtx, "Failed to record episode request", "error", err)
				if histErr := i.repo.InsertDownloadHistory(req.Name, episode.SeasonNumber, episode.Number,
					episode.AbsoluteNumber, status.Failure, "[Scheduling Error]: "+err.Error(), traceID, spanID); histErr != nil {
					i.logger.ErrorContext(episodeCtx, "Failed to insert download history", "error", histErr)
				}
				episodeSpan.End()
				continue
			}
			if !started {
				// A dispatch already holds the row: the episode is on its way.
				i.logger.InfoContext(episodeCtx, "Episode already being dispatched, skipping",
					"season", episode.SeasonNumber, "episode", episode.Number)
				episodeSpan.SetAttributes(attribute.String("episode.status", "in_progress"))
				summary.InProgress++
				episodeSpan.End()
				continue
			}
			if i.scheduler != nil {
				// A pending retry row taken over may have had a timer armed.
				i.scheduler.CancelTimer(item.ID)
			}
			episodeSpan.SetAttributes(attribute.String("episode.status", "downloading"))
			toDispatch = append(toDispatch, item)
			err = i.repo.InsertDownloadHistory(req.Name, episode.SeasonNumber, episode.Number, episode.AbsoluteNumber,
				status.Searching, "", traceID, spanID)
			if err != nil {
				i.logger.ErrorContext(episodeCtx, "Failed to insert download history", "episode", episode.Number, "error", err)
			}

			// Create notification for aired episode (status: searching)
			// IMPORTANT: Uses episode.Id as tvdb_id, not req.Id (show ID)
			notification, err := notifications.NewFromSeries(req, episode, traceID, spanID)
			if err != nil {
				i.logger.ErrorContext(episodeCtx, "Failed to create notification object",
					"error", err, "episode", episode.Number)
			} else {
				notification.Status = notifications.StatusSearching
				if err := i.notificationRepo.CreateNotification(episodeCtx, notification); err != nil {
					i.logger.ErrorContext(episodeCtx, "Failed to create notification for aired episode",
						"error", err, "episode", episode.Number)
				}
			}
		} else {
			// Episode hasn't aired - schedule it for future download
			episodeSpan.SetAttributes(attribute.String("episode.status", "scheduled"))
			scheduledMedia := tvdb.Media{
				Id:           req.Id,
				Name:         req.Name,
				Category:     req.Category,
				Anime:        isAnime,
				Score:        req.Score,
				Slug:         req.Slug,
				ImageUrl:     req.ImageUrl,
				OriginalName: req.OriginalName,
				Status:       req.Status,
				Overview:     req.Overview,
				Year:         req.Year,
				Aliases:      i.extractAliases(extendedInfo.Data.Aliases),
				ImdbID:       extendedInfo.Data.ImdbID(),
			}
			scheduledMedia.Metadata.Episodes = []tvdb.Episode{episode}

			err := i.repo.Schedule(ctx, scheduledMedia, episodeAired, traceID, spanID)
			if err != nil {
				// Check if it's a duplicate (already scheduled)
				if err == repository.ErrDuplicateScheduled {
					i.logger.InfoContext(episodeCtx, "Episode already scheduled, skipping", "season", episode.SeasonNumber, "episode", episode.Number)
					summary.Skipped++
					episodeSpan.End()
					continue
				}
				// Other scheduling error - log failure
				summary.Skipped++
				episodeSpan.SetAttributes(attribute.String("error", err.Error()))
				i.logger.ErrorContext(episodeCtx, "Failed to schedule episode", "error", err)
				err = i.repo.InsertDownloadHistory(req.Name, episode.SeasonNumber, episode.Number, episode.AbsoluteNumber,
					status.Failure, "[Scheduling Error]: "+err.Error(), traceID, spanID)
				if err != nil {
					i.logger.ErrorContext(episodeCtx, "Failed to insert download history", "error", err)
				}
			} else {
				summary.Scheduled++
				if summary.NextRelease == nil || episodeAired.Before(*summary.NextRelease) {
					released := episodeAired
					summary.NextRelease = &released
				}
				i.logger.InfoContext(episodeCtx, "Scheduled episode", "season", episode.SeasonNumber, "episode", episode.Number, "air_date", episodeAired.Format("2006-01-02"))

				// Create notification for scheduled episode (status: scheduled)
				// IMPORTANT: Uses episode.Id as tvdb_id, not req.Id (show ID)
				notification, err := notifications.NewFromSeries(req, episode, traceID, spanID)
				if err != nil {
					i.logger.ErrorContext(episodeCtx, "Failed to create notification object",
						"error", err, "episode", episode.Number)
				} else {
					notification.Status = notifications.StatusScheduled
					if err := i.notificationRepo.CreateNotification(episodeCtx, notification); err != nil {
						i.logger.ErrorContext(episodeCtx, "Failed to create notification for scheduled episode",
							"error", err, "episode", episode.Number)
					}
				}
			}
		}

		episodeSpan.End()
	}

	// Log episode processing summary
	i.logger.InfoContext(ctx, "Episode processing complete",
		"media_name", req.Name,
		"total_episodes", len(req.Metadata.Episodes),
		"aired_episodes", len(toDispatch),
		"scheduled_or_skipped", len(req.Metadata.Episodes)-len(toDispatch))

	// The torrenter hand-off searches indexers and can take minutes, so it runs
	// detached from the request, down the same path a due scheduled row takes:
	// pack-first when packs are on, then one episode per call, each row settled
	// on its own answer. Every queued episode already has a "searching"
	// notification and a queued row, so progress and failures surface through
	// the notification and a restart before an answer re-arms the row.
	if len(toDispatch) > 0 {
		summary.QueuedNow = len(toDispatch)
		bgCtx := context.WithoutCancel(ctx)
		i.dispatch(func() {
			i.processDueGroup(bgCtx, toDispatch)
		})
	} else {
		i.logger.InfoContext(ctx, "No aired episodes to download immediately")
	}

	return summary, nil
}

// DownloadMovie handles the download request for a movie. Like DownloadShow it
// returns once the movie is queued or scheduled; the torrent search itself runs
// in the background.
func (i *DownloadInteractor) DownloadMovie(ctx context.Context, req tvdb.Media) (DownloadSummary, error) {
	traceID, _ := telemetry.GetTraceSpanIDs(ctx)
	summary := DownloadSummary{MediaTitle: req.Name, Category: "movie", TraceID: traceID}

	// Scout is already on it: a second request would only add a duplicate
	// notification and a duplicate search, and the torrenter updates the newest
	// row for an item, which would leave the older one stranded on "searching".
	if i.inProgressItems(ctx, []string{req.Id})[req.Id] {
		i.logger.InfoContext(ctx, "Movie already in progress, skipping", "movie", req.Name)
		summary.InProgress = 1
		return summary, nil
	}

	// Get extended info to determine anime classification
	extendedInfo, err := i.getExtendedInformation(ctx, req.Id, "movie")
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to get extended information", "error", err)
		return summary, err
	}

	// Set anime status and aliases from extended info
	isAnime := isMediaAnime(extendedInfo)
	req.Anime = isAnime
	req.Aliases = i.extractAliases(extendedInfo.Data.Aliases)
	req.ImdbID = extendedInfo.Data.ImdbID()

	// Parse release date from FirstAired field
	releaseDate, err := time.Parse("2006-01-02", req.Metadata.FirstAired)
	if err != nil {
		// If parse fails or date is invalid, treat as already released
		i.logger.WarnContext(ctx, "Failed to parse movie release date, treating as released",
			"error", err, "first_aired", req.Metadata.FirstAired)
		return i.queueMovieNow(ctx, req, summary)
	}

	// Check if release date is in the future
	today := time.Now()
	if releaseDate.After(today) {
		// Schedule for future download
		traceID, spanID := telemetry.GetTraceSpanIDs(ctx)
		err := i.repo.Schedule(ctx, req, releaseDate, traceID, spanID)
		if err != nil {
			if err == repository.ErrDuplicateScheduled {
				// Already tracked — not an error the user needs to see as a failure.
				i.logger.InfoContext(ctx, "Movie already scheduled, skipping", "movie", req.Name)
				summary.Skipped = 1
				summary.NextRelease = &releaseDate
				return summary, nil
			}
			i.logger.ErrorContext(ctx, "Failed to schedule movie", "error", err)
			return summary, err
		}
		summary.Scheduled = 1
		summary.NextRelease = &releaseDate
		i.logger.InfoContext(ctx, "Scheduled movie for future release",
			"movie", req.Name, "release_date", releaseDate.Format("2006-01-02"))

		// Create notification for scheduled movie (status: scheduled)
		notification, err := notifications.NewFromMovie(req, traceID, spanID)
		if err != nil {
			i.logger.ErrorContext(ctx, "Failed to create notification object",
				"error", err, "movie", req.Name)
		} else {
			notification.Status = notifications.StatusScheduled
			if err := i.notificationRepo.CreateNotification(ctx, notification); err != nil {
				i.logger.ErrorContext(ctx, "Failed to create notification for scheduled movie",
					"error", err, "movie", req.Name)
			}
		}

		return summary, nil
	}

	// Movie has already been released - download immediately
	return i.queueMovieNow(ctx, req, summary)
}

// inProgressItems reports which of the item keys already have a live
// notification: Scout is searching for them, downloading them, or holding them
// for a release date. A lookup failure yields an empty set, so the request is
// handled as before; the torrenter dedupes in-flight work too, so the worst
// case is the duplicate row this check exists to avoid, not a lost download.
func (i *DownloadInteractor) inProgressItems(ctx context.Context, ids []string) map[string]bool {
	active, err := i.notificationRepo.ActiveTvdbIDs(ctx, ids)
	if err != nil {
		i.logger.WarnContext(ctx, "Could not check for in-progress items, treating all as new", "error", err)
		return map[string]bool{}
	}
	return active
}

// queueMovieNow records the released movie as a queued row, creates its
// "searching" notification and hands the download to the torrenter in the
// background, down the path a due scheduled row takes. The row exists before
// the torrenter is asked, so a restart mid-search re-arms it. A row already
// queued belongs to a dispatch under way, which the summary reports as in
// progress.
func (i *DownloadInteractor) queueMovieNow(ctx context.Context, req tvdb.Media, summary DownloadSummary) (DownloadSummary, error) {
	traceID, spanID := telemetry.GetTraceSpanIDs(ctx)

	item, started, err := i.repo.EnqueueNow(ctx, req, traceID, spanID)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to record movie request", "error", err, "movie", req.Name)
		return summary, err
	}
	if !started {
		i.logger.InfoContext(ctx, "Movie already being dispatched, skipping", "movie", req.Name)
		summary.InProgress = 1
		return summary, nil
	}
	if i.scheduler != nil {
		i.scheduler.CancelTimer(item.ID)
	}

	notification, err := notifications.NewFromMovie(req, traceID, spanID)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to create notification object",
			"error", err, "movie", req.Name)
	} else {
		notification.Status = notifications.StatusSearching
		if err := i.notificationRepo.CreateNotification(ctx, notification); err != nil {
			i.logger.ErrorContext(ctx, "Failed to create notification for released movie",
				"error", err, "movie", req.Name)
		}
	}

	summary.QueuedNow = 1
	bgCtx := context.WithoutCancel(ctx)
	i.dispatch(func() {
		i.processDueItem(bgCtx, item)
	})
	return summary, nil
}

// packSettingsReader is what the torrenter client offers beyond dispatching:
// the runtime settings, which say whether packs are on.
type packSettingsReader interface {
	Settings(ctx context.Context) (settings.View, error)
}

// packFirstRequest sends the episode set to the torrenter as a pack-only
// request when season packs are on and there are enough episodes for one. It
// reports attempted=false, with every episode in rest, when packs are off, the
// set is too small, the settings are unreadable or the request failed. Otherwise
// it returns the torrenter's response and the episodes still to be sent one at
// a time (answered no_pack, or not answered at all).
func (i *DownloadInteractor) packFirstRequest(
	ctx context.Context, payload tvdb.Media,
) (resp dlstatus.DownloadResponse, rest []tvdb.Episode, attempted bool) {
	episodes := payload.Metadata.Episodes
	reader, ok := i.torrenterClient.(packSettingsReader)
	if !ok || len(episodes) < 2 {
		return resp, episodes, false
	}
	view, err := reader.Settings(ctx)
	if err != nil {
		i.logger.WarnContext(ctx, "Could not read torrenter settings, sending episodes one at a time", "error", err)
		return resp, episodes, false
	}
	if !view.Values.SeasonPacks || len(episodes) < view.Values.SeasonPackMinEpisodes {
		return resp, episodes, false
	}

	packReq := payload
	packReq.PackOnly = true
	resp, err = i.torrenterClient.Download(ctx, packReq)
	if err != nil {
		i.logger.WarnContext(ctx, "Pack request failed, sending episodes one at a time",
			"error", err, "media", payload.Name, "episodes", len(episodes))
		return dlstatus.DownloadResponse{}, episodes, false
	}

	answered := make(map[string]bool, len(resp.Results))
	for _, r := range resp.Results {
		answered[r.TvdbID] = r.Code != dlstatus.CodeNoPack
	}
	rest = []tvdb.Episode{}
	for _, ep := range episodes {
		if !answered[strconv.Itoa(ep.Id)] {
			rest = append(rest, ep)
		}
	}
	return resp, rest, true
}

// buildScheduledMedia constructs the single-episode media payload a row holds
// for the episode, the same for a request dispatched now and one scheduled for
// its air date, so ComputeContentHash finds the one row for the episode.
func (i *DownloadInteractor) buildScheduledMedia(req tvdb.Media, extendedInfo tvdb.TVDBSeriesExtendedResponse, isAnime bool, episode tvdb.Episode) tvdb.Media {
	scheduledMedia := tvdb.Media{
		Id:           req.Id,
		Name:         req.Name,
		Category:     req.Category,
		Anime:        isAnime,
		Score:        req.Score,
		Slug:         req.Slug,
		ImageUrl:     req.ImageUrl,
		OriginalName: req.OriginalName,
		Status:       req.Status,
		Overview:     req.Overview,
		Year:         req.Year,
		Aliases:      i.extractAliases(extendedInfo.Data.Aliases),
		ImdbID:       extendedInfo.Data.ImdbID(),
	}
	scheduledMedia.Metadata.Episodes = []tvdb.Episode{episode}
	return scheduledMedia
}

func (i *DownloadInteractor) getExtendedInformation(
	ctx context.Context, mediaId, mediaType string,
) (tvdb.TVDBSeriesExtendedResponse, error) {
	i.logger.DebugContext(ctx, "Requesting extended info", "media_id", mediaId, "media_type", mediaType)
	seriesInfo, err := i.tvdbClient.GetExtendedInfo(ctx, mediaId, mediaType)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to get extended info from TVDBProxyClient", "error", err)
		return tvdb.TVDBSeriesExtendedResponse{}, err
	}
	return seriesInfo, nil
}

func isMediaAnime(req tvdb.TVDBSeriesExtendedResponse) bool {
	for _, genre := range req.Data.Genres {
		if genre.Name == "Anime" {
			return true
		}
	}
	return false
}
