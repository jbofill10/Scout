package interactors

import (
	"context"
	"log/slog"
	"time"

	tvdb "shared/media"
	status "shared/status"
	"webserver/internal/clients"
	"webserver/internal/repository"
	"webserver/internal/scheduler"
)

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
			i.logger.Info("Processing due media", "media", media.Name)

			// Media is already complete and ready for Download()
			// Use background context since this is not tied to an HTTP request
			err := i.torrenterClient.Download(context.Background(), media)
			if err != nil {
				i.logger.Error("Failed to download media", "error", err)
				// Log failure for each episode in the media
				for _, ep := range media.Metadata.Episodes {
					_ = i.repo.InsertDownloadHistory(media.Name, ep.SeasonNumber, ep.Number, ep.AbsoluteNumber,
						status.Failure, "[Scheduled Download Error]: "+err.Error())
				}
			}
		}
	}()
}

// DownloadShow handles the download request for a TV show
func (i *DownloadInteractor) DownloadShow(ctx context.Context, req tvdb.Media) error {
	extendedInfo, err := i.getExtendedInformation(ctx, req.Id)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to get extended information", "error", err)
		return err
	}

	isAnime := isMediaAnime(extendedInfo)

	today := time.Now()
	i.logger.InfoContext(ctx, "Download request", "media", req)
	mediaToDownload := []tvdb.Episode{}
	absoluteNumber := 1

	for _, episode := range req.Metadata.Episodes {
		episodeAired, err := time.Parse("2006-01-02", episode.Aired)
		if err != nil {
			i.logger.WarnContext(ctx, "Failed to parse episode aired date", "error", err)
			continue
		}

		if isAnime {
			episode.AbsoluteNumber = absoluteNumber
			absoluteNumber++
		}

		// Has the episode aired yet?
		if today.After(episodeAired) {
			// Episode has aired - add to immediate download batch
			mediaToDownload = append(mediaToDownload, episode)
			err = i.repo.InsertDownloadHistory(req.Name, episode.SeasonNumber, episode.Number, episode.AbsoluteNumber,
				status.Searching, "")
			if err != nil {
				i.logger.ErrorContext(ctx, "Failed to insert download history", "episode", episode.Number, "error", err)
			}
		} else {
			// Episode hasn't aired - schedule it for future download
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
			}
			scheduledMedia.Metadata.Episodes = []tvdb.Episode{episode}

			err := i.repo.Schedule(scheduledMedia, episodeAired)
			if err != nil {
				// Check if it's a duplicate (already scheduled)
				if err == repository.ErrDuplicateScheduled {
					i.logger.InfoContext(ctx, "Episode already scheduled, skipping", "season", episode.SeasonNumber, "episode", episode.Number)
					continue
				}
				// Other scheduling error - log failure
				i.logger.ErrorContext(ctx, "Failed to schedule episode", "error", err)
				err = i.repo.InsertDownloadHistory(req.Name, episode.SeasonNumber, episode.Number, episode.AbsoluteNumber,
					status.Failure, "[Scheduling Error]: "+err.Error())
				if err != nil {
					i.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
				}
			} else {
				i.logger.InfoContext(ctx, "Scheduled episode", "season", episode.SeasonNumber, "episode", episode.Number, "air_date", episodeAired.Format("2006-01-02"))
			}
		}
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

func (i *DownloadInteractor) getExtendedInformation(ctx context.Context, mediaId string) (tvdb.TVDBSeriesExtendedResponse, error) {
	i.logger.DebugContext(ctx, "Requesting extended info", "media_id", mediaId)
	seriesInfo, err := i.tvdbClient.GetExtendedInfo(ctx, mediaId)
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
