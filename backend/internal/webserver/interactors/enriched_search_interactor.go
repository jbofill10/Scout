package interactors

import (
	"context"
	"log/slog"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type EnrichedSearchInteractor struct {
	searchInteractor *SearchInteractor
	tvdbClient       *clients.TVDBProxyClient
	torrenterClient  *clients.TorrenterClient
	logger           *slog.Logger
}

func NewEnrichedSearchInteractor(
	searchInteractor *SearchInteractor,
	tvdbClient *clients.TVDBProxyClient,
	torrenterClient *clients.TorrenterClient,
	logger *slog.Logger,
) *EnrichedSearchInteractor {
	return &EnrichedSearchInteractor{
		searchInteractor: searchInteractor,
		tvdbClient:       tvdbClient,
		torrenterClient:  torrenterClient,
		logger:           logger,
	}
}

// EnrichedSearch orchestrates search, extended info, and status fetching to return enriched media data
func (i *EnrichedSearchInteractor) EnrichedSearch(
	ctx context.Context,
	mediaType, query string,
) ([]tvdb.EnrichedMedia, error) {
	// Step 1: Call existing search logic (with cache)
	i.logger.InfoContext(
		ctx,
		"Starting enriched search",
		telemetry.WithTraceContext(ctx, "query", query, "media_type", mediaType)...,
	)

	searchResults, err := i.searchInteractor.Search(ctx, mediaType, query)
	if err != nil {
		i.logger.ErrorContext(
			ctx,
			"Search failed in enriched search",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		return nil, err
	}

	i.logger.InfoContext(
		ctx,
		"Search completed",
		telemetry.WithTraceContext(ctx, "result_count", len(searchResults))...,
	)

	// If no results, return empty enriched array
	if len(searchResults) == 0 {
		return []tvdb.EnrichedMedia{}, nil
	}

	// Step 2: Fetch episode metadata for TV shows (similar to PopularEnrichedInteractor)
	if mediaType == "series" {
		episodeRequests := make([]clients.EpisodesRequest, len(searchResults))
		for idx, media := range searchResults {
			episodeRequests[idx] = clients.EpisodesRequest{
				SeriesId: media.Id,
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
			// Merge episode metadata into search results
			episodeMap := make(map[string]*tvdb.TVDBSeriesMetadata)
			for _, resp := range episodeResponses {
				if resp.Error == "" && resp.Data != nil {
					episodeMap[resp.Request.SeriesId] = resp.Data
				}
			}

			for idx := range searchResults {
				if metadata, found := episodeMap[searchResults[idx].Id]; found {
					searchResults[idx].Metadata = *metadata
				}
			}

			i.logger.InfoContext(
				ctx,
				"Episode metadata batch completed",
				telemetry.WithTraceContext(ctx, "episodes_fetched", len(episodeMap))...,
			)
		}
	}

	// Step 3: Build batch extended requests from search results
	extendedRequests := make([]clients.ExtendedRequest, len(searchResults))
	for idx, media := range searchResults {
		extendedRequests[idx] = clients.ExtendedRequest{
			Id:        media.Id,
			MediaType: media.Category,
		}
	}

	i.logger.InfoContext(
		ctx,
		"Fetching extended info batch",
		telemetry.WithTraceContext(ctx, "request_count", len(extendedRequests))...,
	)

	// Step 3: Call tvdb-proxy batch extended endpoint
	extendedResults, err := i.tvdbClient.GetExtendedBatch(ctx, extendedRequests)
	if err != nil {
		// Graceful degradation: log error and continue with search results only
		i.logger.WarnContext(
			ctx,
			"Extended info batch failed - continuing with search results only",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		// Return search results without status enrichment
		enrichedResults := make([]tvdb.EnrichedMedia, len(searchResults))
		for idx, media := range searchResults {
			enrichedResults[idx] = tvdb.EnrichedMedia{
				Media: media,
				Status: tvdb.MediaStatusInfo{
					Type: media.Category,
				},
			}
		}
		return enrichedResults, nil
	}

	i.logger.InfoContext(
		ctx,
		"Extended info batch completed",
		telemetry.WithTraceContext(ctx, "result_count", len(extendedResults))...,
	)

	// Merge episode metadata from Step 2 into extended results from Step 3
	// GetExtendedBatch returns new Media objects without episode metadata
	if mediaType == "series" {
		episodeMetadataMap := make(map[string]tvdb.TVDBSeriesMetadata)
		for _, media := range searchResults {
			if len(media.Metadata.Episodes) > 0 {
				episodeMetadataMap[media.Id] = media.Metadata
			}
		}

		// Merge with O(n) complexity
		for idx := range extendedResults {
			if metadata, found := episodeMetadataMap[extendedResults[idx].Id]; found {
				extendedResults[idx].Metadata = metadata
			}
		}
	}

	// Step 4: Build status batch requests from extended results
	statusRequests := make([]tvdb.StatusRequest, len(extendedResults))
	for idx, media := range extendedResults {
		// Map media.Category to torrenter's expected format
		// TVDB uses "series" but torrenter expects "show"
		mediaType := media.Category
		if mediaType == "series" {
			mediaType = "show"
		}

		statusRequests[idx] = tvdb.StatusRequest{
			TvdbId:    media.Id,
			MediaType: mediaType,
		}
	}

	i.logger.InfoContext(
		ctx,
		"Fetching status batch",
		telemetry.WithTraceContext(ctx, "request_count", len(statusRequests))...,
	)

	// Step 5: Call torrenter status batch endpoint
	statusResponse, err := i.torrenterClient.GetStatusBatch(ctx, statusRequests)
	if err != nil {
		// Graceful degradation: log error and continue without status
		i.logger.WarnContext(
			ctx,
			"Status batch failed - continuing without status information",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		// Return extended results without status enrichment
		enrichedResults := make([]tvdb.EnrichedMedia, len(extendedResults))
		for idx, media := range extendedResults {
			enrichedResults[idx] = tvdb.EnrichedMedia{
				Media: media,
				Status: tvdb.MediaStatusInfo{
					Type: media.Category,
				},
			}
		}
		return enrichedResults, nil
	}

	i.logger.InfoContext(
		ctx,
		"Status batch completed",
		telemetry.WithTraceContext(
			ctx,
			"show_count",
			len(statusResponse.Shows),
			"movie_count",
			len(statusResponse.Movies),
		)...,
	)

	// Step 6: Merge all data into EnrichedMedia objects
	enrichedResults := i.mergeStatusWithMedia(ctx, extendedResults, statusResponse)

	i.logger.InfoContext(
		ctx,
		"Enriched search completed",
		telemetry.WithTraceContext(ctx, "enriched_count", len(enrichedResults))...,
	)

	return enrichedResults, nil
}

// mergeStatusWithMedia merges status information from StatusBatchResponse into Media objects
func (i *EnrichedSearchInteractor) mergeStatusWithMedia(
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
				// Calculate downloaded/total using TVDB metadata for accurate counts
				downloaded, total := calculateEpisodeCountsFromMetadata(media, showStatus)
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
