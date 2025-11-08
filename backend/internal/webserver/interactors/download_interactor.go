package interactors

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	status "github.com/jbofill10/scout/backend/pkg/status"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/internal/webserver/scheduler"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("webserver")

type DownloadInteractor struct {
	scheduler       *scheduler.Scheduler
	repo            repository.SchedulerRepository
	logger          *slog.Logger
	mediaQueue      chan tvdb.Media
	tvdbClient      *clients.TVDBProxyClient
	torrenterClient *clients.TorrenterClient
}

func NewDownloadInteractor(
	repo repository.SchedulerRepository,
	sched *scheduler.Scheduler,
	mediaQueue chan tvdb.Media,
	logger *slog.Logger,
	tvdbClient *clients.TVDBProxyClient,
	torrenterClient *clients.TorrenterClient,
) *DownloadInteractor {
	return &DownloadInteractor{
		repo:            repo,
		scheduler:       sched,
		mediaQueue:      mediaQueue,
		logger:          logger,
		tvdbClient:      tvdbClient,
		torrenterClient: torrenterClient,
	}
}

// WatchForDueMedia starts the scheduler and processes due media from the queue
func (i *DownloadInteractor) WatchForDueMedia() {
	i.scheduler.Start(i.mediaQueue)
	go func() {
		for media := range i.mediaQueue {
			// Create a new trace for this scheduled download execution
			// Note: This is a new trace, not tied to the original scheduling request
			// However, the scheduled_trace_id in the database can be used to correlate back
			ctx, span := tracer.Start(context.Background(), "scheduled_download",
				trace.WithAttributes(
					attribute.String("media.name", media.Name),
					attribute.String("media.id", media.Id),
					attribute.Int("episode.count", len(media.Metadata.Episodes)),
				),
			)
			defer span.End()

			i.logger.InfoContext(ctx, "Processing due media", "media", media.Name)

			// Get trace/span IDs for this execution
			traceID, spanID := telemetry.GetTraceSpanIDs(ctx)

			// Media is already complete and ready for Download()
			err := i.torrenterClient.Download(ctx, media)
			if err != nil {
				i.logger.ErrorContext(ctx, "Failed to download media", "error", err)
				// Log failure for each episode in the media
				for _, ep := range media.Metadata.Episodes {
					_ = i.repo.InsertDownloadHistory(media.Name, ep.SeasonNumber, ep.Number, ep.AbsoluteNumber,
						status.Failure, "[Scheduled Download Error]: "+err.Error(), traceID, spanID)
				}
			}
		}
	}()
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
	i.logger.InfoContext(ctx, "Download request", "media", extendedInfo)
	mediaToDownload := []tvdb.Episode{}
	absoluteNumber := 1

	for _, episode := range req.Metadata.Episodes {
		// Create a child span for each episode
		episodeCtx, episodeSpan := tracer.Start(ctx, fmt.Sprintf("%s S%dE%d", req.Name, episode.SeasonNumber, episode.Number),
			trace.WithAttributes(
				attribute.String("media.name", req.Name),
				attribute.Int("episode.season", episode.SeasonNumber),
				attribute.Int("episode.number", episode.Number),
				attribute.String("episode.aired", episode.Aired),
				attribute.Bool("anime", isAnime),
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
			episode.AbsoluteNumber = absoluteNumber
			absoluteNumber++
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

			err := i.repo.Schedule(scheduledMedia, episodeAired, traceID, spanID)
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
			}
		}

		episodeSpan.End()
	}

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

		err = i.torrenterClient.Download(ctx, downloadPayload)
		if err != nil {
			i.logger.ErrorContext(ctx, "Failed to download show", "error", err)
			return err
		}
		i.logger.InfoContext(ctx, "Sent aired episodes to torrenter", "count", len(mediaToDownload))
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
		err = i.torrenterClient.Download(ctx, req)
		if err != nil {
			i.logger.ErrorContext(ctx, "Failed to download movie", "error", err)
			return err
		}
		i.logger.InfoContext(ctx, "Sent movie to torrenter", "movie", req.Name)
		return nil
	}

	// Check if release date is in the future
	today := time.Now()
	if releaseDate.After(today) {
		// Schedule for future download
		traceID, spanID := telemetry.GetTraceSpanIDs(ctx)
		err := i.repo.Schedule(req, releaseDate, traceID, spanID)
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
		return nil
	}

	// Movie has already been released - download immediately
	err = i.torrenterClient.Download(ctx, req)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to download movie", "error", err)
		return err
	}
	i.logger.InfoContext(ctx, "Sent movie to torrenter", "movie", req.Name)
	return nil
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
