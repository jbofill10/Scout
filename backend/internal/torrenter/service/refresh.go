package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/clients"
	"github.com/jbofill10/scout/backend/pkg/library"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var refreshTracer = otel.Tracer("service.refresh")

// RefreshService handles periodic TVDB episode metadata refresh
type RefreshService struct {
	repo            Repository
	webserverClient *clients.WebserverClient
	refreshInterval time.Duration
	logger          *slog.Logger
}

// NewRefreshService creates a new refresh service
func NewRefreshService(
	repo Repository,
	webserverClient *clients.WebserverClient,
	refreshInterval time.Duration,
	logger *slog.Logger,
) *RefreshService {
	return &RefreshService{
		repo:            repo,
		webserverClient: webserverClient,
		refreshInterval: refreshInterval,
		logger:          logger,
	}
}

// Start begins the periodic TVDB refresh job
// Waits for the first interval before running, then runs every refreshInterval
func (r *RefreshService) Start(ctx context.Context) {
	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()

	r.logger.InfoContext(ctx, "TVDB refresh service started",
		"refresh_interval", r.refreshInterval.String())

	for {
		select {
		case <-ticker.C:
			// Run refresh on each tick
			if err := r.refreshTvdbEpisodes(ctx); err != nil {
				r.logger.ErrorContext(ctx, "TVDB refresh failed", "error", err)
			}
		case <-ctx.Done():
			r.logger.InfoContext(ctx, "TVDB refresh service stopped")
			return
		}
	}
}

// refreshTvdbEpisodes fetches TVDB episode metadata for all library shows
func (r *RefreshService) refreshTvdbEpisodes(ctx context.Context) error {
	ctx, span := refreshTracer.Start(ctx, "service.RefreshTvdbEpisodes")
	defer span.End()

	startTime := time.Now()

	r.logger.InfoContext(ctx, "Starting TVDB episode refresh")

	// 1. Get all shows with TVDB IDs from database
	shows, err := r.repo.GetAllShows(ctx)
	if err != nil {
		return fmt.Errorf("failed to get shows from database: %w", err)
	}

	if len(shows) == 0 {
		r.logger.InfoContext(ctx, "No shows found in library, skipping refresh")
		return nil
	}

	r.logger.InfoContext(ctx, "Fetched shows from database",
		telemetry.WithTraceContext(ctx, "show_count", len(shows))...)

	// 2. Split shows into batches of 50
	batches := chunkShows(shows, 50)

	r.logger.InfoContext(ctx, "Split shows into batches",
		telemetry.WithTraceContext(ctx, "batch_count", len(batches))...)

	// 3. Process each batch
	totalEpisodes := 0
	for i, batch := range batches {
		batchCtx, batchSpan := refreshTracer.Start(ctx, "service.FetchBatch")
		batchSpan.SetAttributes(attribute.Int("batch_index", i+1))
		batchSpan.SetAttributes(attribute.Int("batch_size", len(batch)))

		r.logger.InfoContext(batchCtx, "Processing batch",
			telemetry.WithTraceContext(batchCtx,
				"batch_index", i+1,
				"batch_size", len(batch))...)

		episodeCount, err := r.processBatch(batchCtx, batch)
		if err != nil {
			// Log error but continue with other batches (partial failure tolerance)
			r.logger.ErrorContext(batchCtx, "Batch processing failed",
				telemetry.WithTraceContext(batchCtx,
					"batch_index", i+1,
					"error", err)...)
			batchSpan.RecordError(err)
		} else {
			totalEpisodes += episodeCount
		}

		batchSpan.End()
	}

	duration := time.Since(startTime)

	r.logger.InfoContext(ctx, "TVDB refresh completed",
		telemetry.WithTraceContext(ctx,
			"shows_processed", len(shows),
			"episodes_upserted", totalEpisodes,
			"duration_ms", duration.Milliseconds())...)

	return nil
}

// processBatch fetches and stores episodes for a batch of shows
func (r *RefreshService) processBatch(ctx context.Context, shows []library.LibraryShow) (int, error) {
	// Build requests for webserver batch endpoint
	requests := make([]clients.EpisodesRequest, 0, len(shows))
	for _, show := range shows {
		requests = append(requests, clients.EpisodesRequest{
			SeriesId: show.TvdbId,
		})
	}

	// Call webserver to fetch episodes from TVDB
	responses, err := r.webserverClient.GetEpisodesBatch(ctx, requests)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch episodes from webserver: %w", err)
	}

	// Process each response and upsert episodes to database
	totalEpisodes := 0
	for _, resp := range responses {
		if resp.Error != "" {
			// Log error but continue with other shows (partial failure tolerance)
			r.logger.WarnContext(ctx, "Failed to fetch episodes for show",
				telemetry.WithTraceContext(ctx,
					"series_id", resp.Request.SeriesId,
					"error", resp.Error)...)
			continue
		}

		if resp.Data == nil || len(resp.Data.Episodes) == 0 {
			r.logger.WarnContext(ctx, "No episodes found for show",
				telemetry.WithTraceContext(ctx, "series_id", resp.Request.SeriesId)...)
			continue
		}

		// Convert tvdb.Episode to library.TvdbEpisode
		episodes := convertEpisodesToLibraryFormat(resp.Request.SeriesId, resp.Data.Episodes)

		// Upsert episodes to database
		err := r.repo.UpsertTvdbEpisodes(ctx, resp.Request.SeriesId, episodes)
		if err != nil {
			// Log error but continue with other shows (partial failure tolerance)
			r.logger.ErrorContext(ctx, "Failed to upsert episodes",
				telemetry.WithTraceContext(ctx,
					"series_id", resp.Request.SeriesId,
					"error", err)...)
			continue
		}

		totalEpisodes += len(episodes)
	}

	return totalEpisodes, nil
}

// chunkShows splits shows into batches of the specified size
func chunkShows(shows []library.LibraryShow, batchSize int) [][]library.LibraryShow {
	var batches [][]library.LibraryShow

	for i := 0; i < len(shows); i += batchSize {
		end := i + batchSize
		if end > len(shows) {
			end = len(shows)
		}
		batches = append(batches, shows[i:end])
	}

	return batches
}

// convertEpisodesToLibraryFormat converts tvdb.Episode to library.TvdbEpisode
func convertEpisodesToLibraryFormat(seriesTvdbId string, episodes []tvdb.Episode) []library.TvdbEpisode {
	result := make([]library.TvdbEpisode, 0, len(episodes))

	for _, ep := range episodes {
		result = append(result, library.TvdbEpisode{
			TvdbId:         strconv.Itoa(ep.Id),
			SeriesTvdbId:   seriesTvdbId,
			SeasonNumber:   ep.SeasonNumber,
			EpisodeNumber:  ep.Number,
			AbsoluteNumber: ep.AbsoluteNumber,
			Name:           ep.Name,
			Aired:          ep.Aired,
		})
	}

	return result
}
