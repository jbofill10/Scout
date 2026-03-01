package interactors

import (
	"context"
	"log/slog"

	"github.com/jbofill10/scout/backend/internal/torrenter/service"
	media "github.com/jbofill10/scout/backend/pkg/media"
)

type StatusInteractor struct {
	repo   service.Repository
	logger *slog.Logger
}

func NewStatusInteractor(repo service.Repository, logger *slog.Logger) *StatusInteractor {
	return &StatusInteractor{
		repo:   repo,
		logger: logger,
	}
}

// GetBatchStatus retrieves the download status for multiple media items
func (i *StatusInteractor) GetBatchStatus(ctx context.Context, requests []media.StatusRequest) (media.StatusBatchResponse, error) {
	response := media.StatusBatchResponse{
		Shows:  []media.ShowStatus{},
		Movies: []media.MovieStatus{},
	}

	i.logger.InfoContext(ctx, "Processing batch status request", "request_count", len(requests))

	// Separate requests by media type for efficient processing
	showRequests := make([]media.StatusRequest, 0)
	movieRequests := make([]media.StatusRequest, 0)

	for _, req := range requests {
		switch req.MediaType {
		case "show":
			showRequests = append(showRequests, req)
		case "movie":
			movieRequests = append(movieRequests, req)
		default:
			i.logger.WarnContext(ctx, "Unknown media type in request", "media_type", req.MediaType, "tvdb_id", req.TvdbId)
		}
	}

	// Process show requests
	for _, req := range showRequests {
		showStatus, err := i.getShowStatus(ctx, req.TvdbId)
		if err != nil {
			i.logger.ErrorContext(ctx, "Error getting show status", "tvdb_id", req.TvdbId, "error", err)
			// Continue processing other requests instead of failing entire batch
			continue
		}
		response.Shows = append(response.Shows, showStatus)
	}

	// Process movie requests
	for _, req := range movieRequests {
		movieStatus, err := i.getMovieStatus(ctx, req.TvdbId)
		if err != nil {
			i.logger.ErrorContext(ctx, "Error getting movie status", "tvdb_id", req.TvdbId, "error", err)
			// Continue processing other requests instead of failing entire batch
			continue
		}
		response.Movies = append(response.Movies, movieStatus)
	}

	i.logger.InfoContext(ctx, "Batch status request completed", "shows_count", len(response.Shows), "movies_count", len(response.Movies))

	return response, nil
}

// getShowStatus retrieves the download status for a single TV show
func (i *StatusInteractor) getShowStatus(ctx context.Context, tvdbId string) (media.ShowStatus, error) {
	seasonEpisodes, err := i.repo.GetShowSeasonEpisodes(ctx, tvdbId)
	if err != nil {
		return media.ShowStatus{}, err
	}

	// Build the ShowStatus response
	showStatus := media.ShowStatus{
		TvdbId:  tvdbId,
		Seasons: []media.SeasonStatus{},
	}

	// Convert map to structured SeasonStatus objects
	for seasonNum, episodeInfos := range seasonEpisodes {
		seasonStatus := media.SeasonStatus{
			SeasonNum: seasonNum,
			Episodes:  make([]media.EpisodeStatus, 0),
		}

		// Create episode status entries
		for _, epInfo := range episodeInfos {
			seasonStatus.Episodes = append(seasonStatus.Episodes, media.EpisodeStatus{
				EpisodeNum: epInfo.EpisodeNum,
				Downloaded: true, // If it's in the DB, it's downloaded
				TvdbId:     epInfo.TvdbId,
			})
		}

		showStatus.Seasons = append(showStatus.Seasons, seasonStatus)
	}

	return showStatus, nil
}

// getMovieStatus retrieves the download status for a single movie
func (i *StatusInteractor) getMovieStatus(ctx context.Context, tvdbId string) (media.MovieStatus, error) {
	inLibrary, err := i.repo.MovieExistsByTvdbId(ctx, tvdbId)
	if err != nil {
		return media.MovieStatus{}, err
	}

	return media.MovieStatus{
		TvdbId:    tvdbId,
		InLibrary: inLibrary,
	}, nil
}
