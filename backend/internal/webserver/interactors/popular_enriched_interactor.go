package interactors

import (
	"context"
	"log/slog"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// TVDBClient defines the interface for TVDB operations
type TVDBClient interface {
	GetPopularShows(ctx context.Context, genre string, limit int) ([]tvdb.Media, error)
	GetPopularMovies(ctx context.Context, genre string, limit int) ([]tvdb.Media, error)
	GetEpisodesBatch(ctx context.Context, requests []clients.EpisodesRequest) ([]clients.EpisodesResponse, error)
	GetExtendedBatch(ctx context.Context, requests []clients.ExtendedRequest) ([]tvdb.Media, error)
}

// TorrenterClient defines the interface for Torrenter operations
type TorrenterClient interface {
	GetStatusBatch(ctx context.Context, requests []tvdb.StatusRequest) (tvdb.StatusBatchResponse, error)
}

type PopularEnrichedInteractor struct {
	tvdbClient      TVDBClient
	torrenterClient TorrenterClient
	logger          *slog.Logger
}

func NewPopularEnrichedInteractor(
	tvdbClient TVDBClient,
	torrenterClient TorrenterClient,
	logger *slog.Logger,
) *PopularEnrichedInteractor {
	return &PopularEnrichedInteractor{
		tvdbClient:      tvdbClient,
		torrenterClient: torrenterClient,
		logger:          logger,
	}
}

// EnrichedPopularShows fetches popular shows and enriches them with episode metadata, extended info, and Plex status
func (i *PopularEnrichedInteractor) EnrichedPopularShows(
	ctx context.Context,
	genre string,
	limit int,
) ([]tvdb.EnrichedMedia, error) {
	// Step 1: Get popular shows (basic info, no episode metadata)
	i.logger.InfoContext(
		ctx,
		"Fetching popular shows",
		telemetry.WithTraceContext(ctx, "genre", genre, "limit", limit)...,
	)

	popularShows, err := i.tvdbClient.GetPopularShows(ctx, genre, limit)
	if err != nil {
		i.logger.ErrorContext(
			ctx,
			"Failed to fetch popular shows",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		return nil, err
	}

	i.logger.InfoContext(
		ctx,
		"Popular shows fetched",
		telemetry.WithTraceContext(ctx, "count", len(popularShows))...,
	)

	if len(popularShows) == 0 {
		return []tvdb.EnrichedMedia{}, nil
	}

	// Step 2: Fetch episode metadata for all shows via batch endpoint
	episodeRequests := make([]clients.EpisodesRequest, len(popularShows))
	for idx, show := range popularShows {
		episodeRequests[idx] = clients.EpisodesRequest{
			SeriesId: show.Id,
		}
	}

	i.logger.InfoContext(
		ctx,
		"Fetching episode metadata batch",
		telemetry.WithTraceContext(ctx, "request_count", len(episodeRequests))...,
	)

	episodeResponses, err := i.tvdbClient.GetEpisodesBatch(ctx, episodeRequests)
	if err != nil {
		i.logger.WarnContext(
			ctx,
			"Episode metadata batch failed - continuing without episodes",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		// Continue without episode metadata
	} else {
		// Merge episode metadata into media objects
		episodeMap := make(map[string]*tvdb.TVDBSeriesMetadata)
		for _, resp := range episodeResponses {
			if resp.Error == "" && resp.Data != nil {
				episodeMap[resp.Request.SeriesId] = resp.Data
			}
		}

		for idx := range popularShows {
			if metadata, found := episodeMap[popularShows[idx].Id]; found {
				popularShows[idx].Metadata = *metadata
			}
		}

		i.logger.InfoContext(
			ctx,
			"Episode metadata batch completed",
			telemetry.WithTraceContext(ctx, "episodes_fetched", len(episodeMap))...,
		)
	}

	// Step 3: Fetch extended info (genres, aliases) via batch/extended
	extendedRequests := make([]clients.ExtendedRequest, len(popularShows))
	for idx, show := range popularShows {
		extendedRequests[idx] = clients.ExtendedRequest{
			Id:        show.Id,
			MediaType: show.Category,
		}
	}

	i.logger.InfoContext(
		ctx,
		"Fetching extended info batch",
		telemetry.WithTraceContext(ctx, "request_count", len(extendedRequests))...,
	)

	enrichedShows, err := i.tvdbClient.GetExtendedBatch(ctx, extendedRequests)
	if err != nil {
		i.logger.WarnContext(
			ctx,
			"Extended info batch failed - continuing with basic info",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		// Use popularShows as fallback if extended info fails
		enrichedShows = popularShows
	} else {
		// Merge episode metadata from Step 2 into enriched shows from Step 3
		// GetExtendedBatch returns new Media objects without episode metadata
		// Build map for O(n) lookups instead of O(n²) nested loops
		episodeMetadataMap := make(map[string]tvdb.TVDBSeriesMetadata)
		for _, show := range popularShows {
			if len(show.Metadata.Episodes) > 0 {
				episodeMetadataMap[show.Id] = show.Metadata
			}
		}

		// Merge with O(n) complexity
		for idx := range enrichedShows {
			if metadata, found := episodeMetadataMap[enrichedShows[idx].Id]; found {
				enrichedShows[idx].Metadata = metadata
			}
		}
	}

	i.logger.InfoContext(
		ctx,
		"Extended info batch completed",
		telemetry.WithTraceContext(ctx, "result_count", len(enrichedShows))...,
	)

	// Step 4: Fetch Plex status via torrenter batch/status
	statusRequests := make([]tvdb.StatusRequest, len(enrichedShows))
	for idx, show := range enrichedShows {
		statusRequests[idx] = tvdb.StatusRequest{
			TvdbId:    show.Id,
			MediaType: "show",
		}
	}

	i.logger.InfoContext(
		ctx,
		"Fetching status batch",
		telemetry.WithTraceContext(ctx, "request_count", len(statusRequests))...,
	)

	statusResponse, err := i.torrenterClient.GetStatusBatch(ctx, statusRequests)
	if err != nil {
		i.logger.WarnContext(
			ctx,
			"Status batch failed - continuing without status",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		// Return enriched results without status
		enrichedResults := make([]tvdb.EnrichedMedia, len(enrichedShows))
		for idx, show := range enrichedShows {
			enrichedResults[idx] = tvdb.EnrichedMedia{
				Media: show,
				Status: tvdb.MediaStatusInfo{
					Type: "series",
				},
			}
		}
		return enrichedResults, nil
	}

	i.logger.InfoContext(
		ctx,
		"Status batch completed",
		telemetry.WithTraceContext(ctx, "show_count", len(statusResponse.Shows))...,
	)

	// Step 5: Merge status with media
	enrichedResults := i.mergeStatusWithMedia(ctx, enrichedShows, statusResponse)

	i.logger.InfoContext(
		ctx,
		"Enriched popular shows completed",
		telemetry.WithTraceContext(ctx, "enriched_count", len(enrichedResults))...,
	)

	return enrichedResults, nil
}

// EnrichedPopularMovies fetches popular movies and enriches them with extended info and Plex status
func (i *PopularEnrichedInteractor) EnrichedPopularMovies(
	ctx context.Context,
	genre string,
	limit int,
) ([]tvdb.EnrichedMedia, error) {
	// Step 1: Get popular movies (basic info)
	i.logger.InfoContext(
		ctx,
		"Fetching popular movies",
		telemetry.WithTraceContext(ctx, "genre", genre, "limit", limit)...,
	)

	popularMovies, err := i.tvdbClient.GetPopularMovies(ctx, genre, limit)
	if err != nil {
		i.logger.ErrorContext(
			ctx,
			"Failed to fetch popular movies",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		return nil, err
	}

	i.logger.InfoContext(
		ctx,
		"Popular movies fetched",
		telemetry.WithTraceContext(ctx, "count", len(popularMovies))...,
	)

	if len(popularMovies) == 0 {
		return []tvdb.EnrichedMedia{}, nil
	}

	// Step 2: Fetch extended info (genres, aliases) via batch/extended
	extendedRequests := make([]clients.ExtendedRequest, len(popularMovies))
	for idx, movie := range popularMovies {
		extendedRequests[idx] = clients.ExtendedRequest{
			Id:        movie.Id,
			MediaType: movie.Category,
		}
	}

	i.logger.InfoContext(
		ctx,
		"Fetching extended info batch",
		telemetry.WithTraceContext(ctx, "request_count", len(extendedRequests))...,
	)

	enrichedMovies, err := i.tvdbClient.GetExtendedBatch(ctx, extendedRequests)
	if err != nil {
		i.logger.WarnContext(
			ctx,
			"Extended info batch failed - continuing with basic info",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		// Use popularMovies as fallback if extended info fails
		enrichedMovies = popularMovies
	}

	i.logger.InfoContext(
		ctx,
		"Extended info batch completed",
		telemetry.WithTraceContext(ctx, "result_count", len(enrichedMovies))...,
	)

	// Step 3: Fetch Plex status via torrenter batch/status
	statusRequests := make([]tvdb.StatusRequest, len(enrichedMovies))
	for idx, movie := range enrichedMovies {
		statusRequests[idx] = tvdb.StatusRequest{
			TvdbId:    movie.Id,
			MediaType: "movie",
		}
	}

	i.logger.InfoContext(
		ctx,
		"Fetching status batch",
		telemetry.WithTraceContext(ctx, "request_count", len(statusRequests))...,
	)

	statusResponse, err := i.torrenterClient.GetStatusBatch(ctx, statusRequests)
	if err != nil {
		i.logger.WarnContext(
			ctx,
			"Status batch failed - continuing without status",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		// Return enriched results without status
		enrichedResults := make([]tvdb.EnrichedMedia, len(enrichedMovies))
		for idx, movie := range enrichedMovies {
			enrichedResults[idx] = tvdb.EnrichedMedia{
				Media: movie,
				Status: tvdb.MediaStatusInfo{
					Type: "movie",
				},
			}
		}
		return enrichedResults, nil
	}

	i.logger.InfoContext(
		ctx,
		"Status batch completed",
		telemetry.WithTraceContext(ctx, "movie_count", len(statusResponse.Movies))...,
	)

	// Step 4: Merge status with media
	enrichedResults := i.mergeStatusWithMedia(ctx, enrichedMovies, statusResponse)

	i.logger.InfoContext(
		ctx,
		"Enriched popular movies completed",
		telemetry.WithTraceContext(ctx, "enriched_count", len(enrichedResults))...,
	)

	return enrichedResults, nil
}

// mergeStatusWithMedia merges status information from StatusBatchResponse into Media objects
func (i *PopularEnrichedInteractor) mergeStatusWithMedia(
	ctx context.Context,
	mediaList []tvdb.Media,
	statusResponse tvdb.StatusBatchResponse,
) []tvdb.EnrichedMedia {
	// Build lookup maps for efficient merging
	showStatusMap := make(map[string]tvdb.ShowStatus)
	for _, showStatus := range statusResponse.Shows {
		showStatusMap[showStatus.TvdbId] = showStatus
	}

	movieStatusMap := make(map[string]tvdb.MovieStatus)
	for _, movieStatus := range statusResponse.Movies {
		movieStatusMap[movieStatus.TvdbId] = movieStatus
	}

	// Merge status into each media item
	enrichedResults := make([]tvdb.EnrichedMedia, len(mediaList))
	for idx, media := range mediaList {
		statusInfo := tvdb.MediaStatusInfo{
			Type: media.Category,
		}

		switch media.Category {
		case "series":
			// Merge show status
			if showStatus, found := showStatusMap[media.Id]; found {
				statusInfo.Seasons = showStatus.Seasons
				// Calculate downloaded/total episodes
				downloaded, total := i.calculateEpisodeCounts(showStatus.Seasons)
				statusInfo.Downloaded = downloaded
				statusInfo.Total = total
			}
		case "movie":
			// Merge movie status
			if movieStatus, found := movieStatusMap[media.Id]; found {
				statusInfo.InLibrary = movieStatus.InLibrary
			}
		}

		enrichedResults[idx] = tvdb.EnrichedMedia{
			Media:  media,
			Status: statusInfo,
		}
	}

	return enrichedResults
}

// calculateEpisodeCounts calculates the total and downloaded episode counts from season status
func (i *PopularEnrichedInteractor) calculateEpisodeCounts(seasons []tvdb.SeasonStatus) (downloaded, total int) {
	for _, season := range seasons {
		for _, episode := range season.Episodes {
			total++
			if episode.Downloaded {
				downloaded++
			}
		}
	}
	return downloaded, total
}
