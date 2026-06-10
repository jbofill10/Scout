package interactors

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/internal/webserver/retry"
	"github.com/jbofill10/scout/backend/internal/webserver/scheduler"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	status "github.com/jbofill10/scout/backend/pkg/status"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("webserver")

type DownloadInteractor struct {
	scheduler        *scheduler.Scheduler
	repo             repository.SchedulerRepository
	notificationRepo repository.NotificationRepository
	logger           *slog.Logger
	mediaQueue       chan repository.DueItem
	tvdbClient       *clients.TVDBProxyClient
	torrenterClient  *clients.TorrenterClient
}

func NewDownloadInteractor(
	repo repository.SchedulerRepository,
	notificationRepo repository.NotificationRepository,
	sched *scheduler.Scheduler,
	mediaQueue chan repository.DueItem,
	logger *slog.Logger,
	tvdbClient *clients.TVDBProxyClient,
	torrenterClient *clients.TorrenterClient,
) *DownloadInteractor {
	return &DownloadInteractor{
		repo:             repo,
		notificationRepo: notificationRepo,
		scheduler:        sched,
		mediaQueue:       mediaQueue,
		logger:           logger,
		tvdbClient:       tvdbClient,
		torrenterClient:  torrenterClient,
	}
}

// WatchForDueMedia starts the scheduler and processes due media from the queue
func (i *DownloadInteractor) WatchForDueMedia() {
	i.scheduler.Start(i.mediaQueue)
	go func() {
		for item := range i.mediaQueue {
			i.processDueItem(item)
		}
	}()
}

// processDueItem dispatches a single scheduled download and transitions its row
// to a terminal status (completed/failed) or reschedules it for retry.
func (i *DownloadInteractor) processDueItem(item repository.DueItem) {
	media := item.Media
	// Create a new trace for this scheduled download execution.
	// Note: this is a new trace, not tied to the original scheduling request;
	// the scheduled_trace_id column can correlate back.
	ctx, span := tracer.Start(context.Background(), "scheduled_download",
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
	if err != nil {
		i.logger.ErrorContext(ctx, "Download dispatch error", "error", err, "media", media.Name)
	}

	failed := failedResult(resp.Results)
	if failed == nil {
		// All results downloading/exists (and no err with empty results) → done.
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

	if code.Category() == dlstatus.CategoryPermanent {
		i.handlePermanentRowFailure(ctx, item, *failed, reason, traceID, spanID)
		return
	}

	// Transient failure → apply backoff policy.
	next, ok := retry.NextAttempt(time.Now(), item.Attempts)
	if !ok {
		// Exhausted retries → terminal failure.
		exhaustReason := fmt.Sprintf("Gave up after %d attempts: %s", retry.MaxAttempts, reason)
		i.handlePermanentRowFailureWithCode(ctx, item, *failed, string(dlstatus.CodeMaxRetries), exhaustReason, traceID, spanID)
		return
	}

	// Retries remain → reschedule the row and keep the notification searching.
	if recErr := i.repo.RecordFailure(ctx, item.ID, string(code), reason, next); recErr != nil {
		i.logger.ErrorContext(ctx, "Failed to record transient failure", "error", recErr, "id", item.ID)
	}
	attemptReason := fmt.Sprintf("%s (attempt %d of %d, next try %s)",
		code.HumanReason(), item.Attempts+1, retry.MaxAttempts, next.Format("15:04"))
	i.updateNotificationStatus(ctx, failed.TvdbID, notifications.StatusSearching, attemptReason)
	i.logger.InfoContext(ctx, "Scheduled download will retry",
		"media", media.Name, "code", code, "next_attempt", next.Format(time.RFC3339))
}

// handlePermanentRowFailure marks the row failed using the failure's own code.
func (i *DownloadInteractor) handlePermanentRowFailure(ctx context.Context, item repository.DueItem, failed dlstatus.EpisodeResult, reason, traceID, spanID string) {
	i.handlePermanentRowFailureWithCode(ctx, item, failed, string(failed.Code), reason, traceID, spanID)
}

// handlePermanentRowFailureWithCode marks the row terminally failed, updates the
// episode notification to failed, and records a history failure row.
func (i *DownloadInteractor) handlePermanentRowFailureWithCode(ctx context.Context, item repository.DueItem, failed dlstatus.EpisodeResult, code, reason, traceID, spanID string) {
	if markErr := i.repo.MarkPermanentlyFailed(ctx, item.ID, code, reason); markErr != nil {
		i.logger.ErrorContext(ctx, "Failed to mark download permanently failed", "error", markErr, "id", item.ID)
	}
	i.updateNotificationStatus(ctx, failed.TvdbID, notifications.StatusFailed, reason)
	if histErr := i.repo.InsertDownloadHistory(item.Media.Name, failed.Season, failed.Episode, 0,
		status.Failure, "[Scheduled Download Failed]: "+reason, traceID, spanID); histErr != nil {
		i.logger.ErrorContext(ctx, "Failed to insert download history", "error", histErr)
	}
	i.logger.ErrorContext(ctx, "Scheduled download permanently failed",
		"media", item.Media.Name, "code", code, "reason", reason)
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

// DownloadShow handles the download request for a TV show
func (i *DownloadInteractor) DownloadShow(ctx context.Context, req tvdb.Media) error {
	extendedInfo, err := i.getExtendedInformation(ctx, req.Id, "series")
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to get extended information", "error", err)
		return err
	}

	isAnime := isMediaAnime(extendedInfo)

	today := time.Now()
	i.logger.InfoContext(ctx, "Download request",
		"media_id", req.Id,
		"media_name", req.Name,
		"category", req.Category,
		"anime", isAnime,
		"episode_count", len(req.Metadata.Episodes))
	mediaToDownload := []tvdb.Episode{}

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
			continue
		}

		episodeAired, err := time.Parse("2006-01-02", episode.Aired)
		if err != nil {
			i.logger.WarnContext(episodeCtx, "Failed to parse episode aired date", "error", err)
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
			episodeSpan.SetAttributes(attribute.String("episode.status", "downloading"))
			mediaToDownload = append(mediaToDownload, episode)
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
			}
			scheduledMedia.Metadata.Episodes = []tvdb.Episode{episode}

			err := i.repo.Schedule(ctx, scheduledMedia, episodeAired, traceID, spanID)
			if err != nil {
				// Check if it's a duplicate (already scheduled)
				if err == repository.ErrDuplicateScheduled {
					i.logger.InfoContext(episodeCtx, "Episode already scheduled, skipping", "season", episode.SeasonNumber, "episode", episode.Number)
					episodeSpan.End()
					continue
				}
				// Other scheduling error - log failure
				episodeSpan.SetAttributes(attribute.String("error", err.Error()))
				i.logger.ErrorContext(episodeCtx, "Failed to schedule episode", "error", err)
				err = i.repo.InsertDownloadHistory(req.Name, episode.SeasonNumber, episode.Number, episode.AbsoluteNumber,
					status.Failure, "[Scheduling Error]: "+err.Error(), traceID, spanID)
				if err != nil {
					i.logger.ErrorContext(episodeCtx, "Failed to insert download history", "error", err)
				}
			} else {
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
		"aired_episodes", len(mediaToDownload),
		"scheduled_or_skipped", len(req.Metadata.Episodes)-len(mediaToDownload))

	// Only send download request if there are aired episodes
	if len(mediaToDownload) > 0 {
		downloadPayload := tvdb.Media{
			Id:       req.Id,
			Name:     req.Name,
			Category: req.Category,
			Anime:    isAnime,
			Slug:     req.Slug,
			Year:     req.Year,
			Metadata: req.Metadata,
			Aliases:  i.extractAliases(extendedInfo.Data.Aliases),
		}
		downloadPayload.Metadata.Episodes = mediaToDownload

		resp, dlErr := i.torrenterClient.Download(ctx, downloadPayload)
		if dlErr != nil {
			i.logger.ErrorContext(ctx, "Failed to download show", "error", dlErr)
		}
		i.logger.InfoContext(ctx, "Sent aired episodes to torrenter", "count", len(mediaToDownload))

		// Handle per-episode failures: transient ones get a retry row, permanent
		// ones surface a failed notification + history entry.
		traceID, spanID := telemetry.GetTraceSpanIDs(ctx)
		allScheduled, scheduleErr := i.handleImmediateShowFailures(ctx, downloadPayload, extendedInfo, isAnime, resp, traceID, spanID)
		if dlErr != nil && (!allScheduled || scheduleErr != nil) {
			// Work was lost (couldn't schedule retries for some failures); surface the error.
			return dlErr
		}
	} else {
		i.logger.InfoContext(ctx, "No aired episodes to download immediately")
	}

	return nil
}

// DownloadMovie handles the download request for a movie
func (i *DownloadInteractor) DownloadMovie(ctx context.Context, req tvdb.Media) error {
	// Get extended info to determine anime classification
	extendedInfo, err := i.getExtendedInformation(ctx, req.Id, "movie")
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to get extended information", "error", err)
		return err
	}

	// Set anime status based on TVDB genres
	isAnime := isMediaAnime(extendedInfo)
	req.Anime = isAnime

	// Parse release date from FirstAired field
	releaseDate, err := time.Parse("2006-01-02", req.Metadata.FirstAired)
	if err != nil {
		// If parse fails or date is invalid, treat as already released
		i.logger.WarnContext(ctx, "Failed to parse movie release date, treating as released",
			"error", err, "first_aired", req.Metadata.FirstAired)
		// Download immediately
		traceID, spanID := telemetry.GetTraceSpanIDs(ctx)
		resp, dlErr := i.torrenterClient.Download(ctx, req)
		if dlErr != nil {
			i.logger.ErrorContext(ctx, "Failed to download movie", "error", dlErr)
		}
		i.logger.InfoContext(ctx, "Sent movie to torrenter", "movie", req.Name)
		scheduled, scheduleErr := i.handleImmediateMovieFailure(ctx, req, resp, traceID, spanID)
		if dlErr != nil && (!scheduled || scheduleErr != nil) {
			return dlErr
		}
		return nil
	}

	// Check if release date is in the future
	today := time.Now()
	if releaseDate.After(today) {
		// Schedule for future download
		traceID, spanID := telemetry.GetTraceSpanIDs(ctx)
		err := i.repo.Schedule(ctx, req, releaseDate, traceID, spanID)
		if err != nil {
			if err == repository.ErrDuplicateScheduled {
				i.logger.InfoContext(ctx, "Movie already scheduled, skipping", "movie", req.Name)
				return err
			}
			i.logger.ErrorContext(ctx, "Failed to schedule movie", "error", err)
			return err
		}
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

		return nil
	}

	// Movie has already been released - download immediately
	traceID, spanID := telemetry.GetTraceSpanIDs(ctx)

	// Create notification for released movie (status: searching)
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

	resp, dlErr := i.torrenterClient.Download(ctx, req)
	if dlErr != nil {
		i.logger.ErrorContext(ctx, "Failed to download movie", "error", dlErr)
	}
	i.logger.InfoContext(ctx, "Sent movie to torrenter", "movie", req.Name)
	scheduled, scheduleErr := i.handleImmediateMovieFailure(ctx, req, resp, traceID, spanID)
	if dlErr != nil && (!scheduled || scheduleErr != nil) {
		return dlErr
	}
	return nil
}

// handleImmediateShowFailures inspects an immediate show download response and,
// for each failed episode, schedules a retry (transient) or surfaces a failed
// notification + history row (permanent). It returns whether every failure was
// handled such that no work was lost (transient failures scheduled; permanent
// failures are terminal and count as handled), and the first scheduling error.
func (i *DownloadInteractor) handleImmediateShowFailures(
	ctx context.Context,
	payload tvdb.Media,
	extendedInfo tvdb.TVDBSeriesExtendedResponse,
	isAnime bool,
	resp dlstatus.DownloadResponse,
	traceID, spanID string,
) (bool, error) {
	// Index episodes by their tvdb id (string) for per-result lookup.
	episodeByID := make(map[string]tvdb.Episode, len(payload.Metadata.Episodes))
	for _, ep := range payload.Metadata.Episodes {
		episodeByID[strconv.Itoa(ep.Id)] = ep
	}

	allHandled := true
	var firstScheduleErr error

	for _, r := range resp.Results {
		if r.Outcome != dlstatus.OutcomeFailed {
			continue
		}
		reason := r.Reason
		if reason == "" {
			reason = r.Code.HumanReason()
		}

		if r.Code.Category() == dlstatus.CategoryPermanent {
			i.updateNotificationStatus(ctx, r.TvdbID, notifications.StatusFailed, reason)
			if err := i.repo.InsertDownloadHistory(payload.Name, r.Season, r.Episode, 0,
				status.Failure, "[Download Failed]: "+reason, traceID, spanID); err != nil {
				i.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
			}
			continue
		}

		// Transient → schedule a retry row for this single episode.
		ep, ok := episodeByID[r.TvdbID]
		if !ok {
			i.logger.WarnContext(ctx, "Failed result has no matching episode", "tvdb_id", r.TvdbID)
			allHandled = false
			continue
		}
		retryMedia := i.buildScheduledMedia(payload, extendedInfo, isAnime, ep)
		if err := i.repo.ScheduleRetry(ctx, retryMedia, time.Now().Add(1*time.Hour),
			string(r.Code), reason, traceID, spanID); err != nil {
			i.logger.ErrorContext(ctx, "Failed to schedule retry for episode", "error", err, "tvdb_id", r.TvdbID)
			allHandled = false
			if firstScheduleErr == nil {
				firstScheduleErr = err
			}
			continue
		}
		i.updateNotificationStatus(ctx, r.TvdbID, notifications.StatusSearching, r.Code.HumanReason()+" — retry scheduled")
	}

	return allHandled, firstScheduleErr
}

// buildScheduledMedia constructs a single-episode media payload identical to the
// one DownloadShow uses for future scheduling, so ComputeContentHash matches the
// row a future poll would create (enabling retry dedup).
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
	}
	scheduledMedia.Metadata.Episodes = []tvdb.Episode{episode}
	return scheduledMedia
}

// handleImmediateMovieFailure inspects an immediate movie download response and
// schedules a retry (transient) or surfaces a failed notification + history row
// (permanent). The whole req is the media for retry scheduling. Returns whether
// any failure was handled without losing work, and the first scheduling error.
func (i *DownloadInteractor) handleImmediateMovieFailure(
	ctx context.Context,
	req tvdb.Media,
	resp dlstatus.DownloadResponse,
	traceID, spanID string,
) (bool, error) {
	failed := failedResult(resp.Results)
	if failed == nil {
		return true, nil
	}
	reason := failed.Reason
	if reason == "" {
		reason = failed.Code.HumanReason()
	}

	if failed.Code.Category() == dlstatus.CategoryPermanent {
		i.updateNotificationStatus(ctx, failed.TvdbID, notifications.StatusFailed, reason)
		if err := i.repo.InsertDownloadHistory(req.Name, 0, 0, 0,
			status.Failure, "[Download Failed]: "+reason, traceID, spanID); err != nil {
			i.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
		}
		return true, nil
	}

	if err := i.repo.ScheduleRetry(ctx, req, time.Now().Add(1*time.Hour),
		string(failed.Code), reason, traceID, spanID); err != nil {
		i.logger.ErrorContext(ctx, "Failed to schedule movie retry", "error", err, "movie", req.Name)
		return false, err
	}
	i.updateNotificationStatus(ctx, failed.TvdbID, notifications.StatusSearching, failed.Code.HumanReason()+" — retry scheduled")
	return true, nil
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
