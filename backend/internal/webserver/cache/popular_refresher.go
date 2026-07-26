package cache

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// TVDBClient defines the interface for TVDB operations
type TVDBClient interface {
	GetGenres(ctx context.Context) ([]clients.Genre, error)
	GetPopularShows(ctx context.Context, genre string, limit int) ([]tvdb.Media, error)
	GetPopularMovies(ctx context.Context, genre string, limit int) ([]tvdb.Media, error)
}

// TorrenterClient defines the interface for Torrenter operations
type TorrenterClient interface {
	GetStatusBatch(ctx context.Context, requests []tvdb.StatusRequest) (tvdb.StatusBatchResponse, error)
}

// PopularEnrichedInteractor defines the interface for enrichment operations
type PopularEnrichedInteractor interface {
	EnrichedPopularShows(ctx context.Context, genre string, limit int) ([]tvdb.EnrichedMedia, error)
	EnrichedPopularMovies(ctx context.Context, genre string, limit int) ([]tvdb.EnrichedMedia, error)
}

// PopularRefresher manages background refresh of popular content cache
type PopularRefresher struct {
	cache                     *PopularCache
	tvdbClient                TVDBClient
	torrenterClient           TorrenterClient
	popularEnrichedInteractor PopularEnrichedInteractor
	logger                    *slog.Logger
	stopChan                  chan struct{}
	refreshInterval           time.Duration
	limit                     int
	tracer                    trace.Tracer
}

// NewPopularRefresher creates a new refresher instance
func NewPopularRefresher(
	cache *PopularCache,
	tvdbClient TVDBClient,
	torrenterClient TorrenterClient,
	popularEnrichedInteractor PopularEnrichedInteractor,
	logger *slog.Logger,
) *PopularRefresher {
	return &PopularRefresher{
		cache:                     cache,
		tvdbClient:                tvdbClient,
		torrenterClient:           torrenterClient,
		popularEnrichedInteractor: popularEnrichedInteractor,
		logger:                    logger,
		stopChan:                  make(chan struct{}),
		refreshInterval:           6 * time.Hour,
		limit:                     20, // Default UI limit
		tracer:                    otel.Tracer("webserver"),
	}
}

// Start begins the background refresh goroutine
// Performs immediate refresh on startup, then refreshes every 6 hours
func (r *PopularRefresher) Start() {
	r.logger.Info("Popular content cache refresher starting")

	// Immediate refresh on startup (blocking)
	r.refreshAll()

	// Start background goroutine for periodic refresh
	go func() {
		ticker := time.NewTicker(r.refreshInterval)
		defer ticker.Stop()

		r.logger.Info("Popular content cache refresher background goroutine started",
			"refresh_interval", r.refreshInterval.String())

		for {
			select {
			case <-ticker.C:
				r.refreshAll()
			case <-r.stopChan:
				r.logger.Info("Popular content cache refresher stopped")
				return
			}
		}
	}()
}

// Stop gracefully shuts down the refresher
func (r *PopularRefresher) Stop() {
	r.logger.Info("Stopping popular content cache refresher")
	close(r.stopChan)
}

// refreshAll fetches all popular content and updates the cache atomically
func (r *PopularRefresher) refreshAll() {
	// Check if refresh is already in progress
	if !r.cache.markRefreshStart() {
		r.logger.Warn("Refresh already in progress, skipping")
		return
	}

	ctx, span := r.tracer.Start(context.Background(), "popular_cache.refresh")
	defer span.End()

	r.logger.InfoContext(ctx, "Popular cache refresh started")

	success := true
	defer func() {
		r.cache.markRefreshEnd(success)
		if success {
			r.logger.InfoContext(ctx, "Popular cache refresh completed successfully")
		} else {
			r.logger.ErrorContext(ctx, "Popular cache refresh failed")
			span.SetAttributes(attribute.String("refresh.status", "failed"))
		}
	}()

	// Step 1: Fetch genres
	genres, err := r.tvdbClient.GetGenres(ctx)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to fetch genres",
			telemetry.WithTraceContext(ctx, "error", err)...)
		span.RecordError(err)
		success = false
		return
	}

	r.logger.InfoContext(ctx, "Fetched genres", telemetry.WithTraceContext(ctx, "count", len(genres))...)

	// Step 2: Define categories to cache
	// Empty string ("") for generic popular content + 6 curated genres
	categories := []string{"", "Action", "Comedy", "Drama", "Sci-Fi", "Anime", "Documentary"}

	// Step 3: Fetch basic content for all categories
	basicShows := make(map[string][]tvdb.Media)
	basicMovies := make(map[string][]tvdb.Media)

	for _, genre := range categories {
		genreLabel := genre
		if genreLabel == "" {
			genreLabel = "popular" // For logging
		}

		// Fetch basic popular shows
		shows, err := r.tvdbClient.GetPopularShows(ctx, genre, r.limit)
		if err != nil {
			r.logger.WarnContext(ctx, "Failed to fetch basic popular shows",
				telemetry.WithTraceContext(ctx, "genre", genreLabel, "error", err)...)
		} else {
			basicShows[genre] = shows
			r.logger.InfoContext(ctx, "Fetched basic popular shows",
				telemetry.WithTraceContext(ctx, "genre", genreLabel, "count", len(shows))...)
		}

		// Fetch basic popular movies (independent of shows result)
		movies, err := r.tvdbClient.GetPopularMovies(ctx, genre, r.limit)
		if err != nil {
			r.logger.WarnContext(ctx, "Failed to fetch basic popular movies",
				telemetry.WithTraceContext(ctx, "genre", genreLabel, "error", err)...)
		} else {
			basicMovies[genre] = movies
			r.logger.InfoContext(ctx, "Fetched basic popular movies",
				telemetry.WithTraceContext(ctx, "genre", genreLabel, "count", len(movies))...)
		}
	}

	// Step 4: Fetch enriched content for all categories
	enrichedShows := make(map[string][]tvdb.EnrichedMedia)
	enrichedMovies := make(map[string][]tvdb.EnrichedMedia)

	for _, genre := range categories {
		genreLabel := genre
		if genreLabel == "" {
			genreLabel = "popular"
		}

		// Fetch enriched popular shows (includes episodes, extended info, Plex status)
		shows, err := r.popularEnrichedInteractor.EnrichedPopularShows(ctx, genre, r.limit)
		if err != nil {
			r.logger.WarnContext(ctx, "Failed to fetch enriched popular shows",
				telemetry.WithTraceContext(ctx, "genre", genreLabel, "error", err)...)
		} else {
			enrichedShows[genre] = shows
			r.logger.InfoContext(ctx, "Fetched enriched popular shows",
				telemetry.WithTraceContext(ctx, "genre", genreLabel, "count", len(shows))...)
		}

		// Fetch enriched popular movies (independent of shows result)
		movies, err := r.popularEnrichedInteractor.EnrichedPopularMovies(ctx, genre, r.limit)
		if err != nil {
			r.logger.WarnContext(ctx, "Failed to fetch enriched popular movies",
				telemetry.WithTraceContext(ctx, "genre", genreLabel, "error", err)...)
		} else {
			enrichedMovies[genre] = movies
			r.logger.InfoContext(ctx, "Fetched enriched popular movies",
				telemetry.WithTraceContext(ctx, "genre", genreLabel, "count", len(movies))...)
		}
	}

	// Step 5: Update cache atomically
	r.cache.updateAll(genres, basicShows, basicMovies, enrichedShows, enrichedMovies)

	// Log summary statistics
	r.logger.InfoContext(ctx, "Popular cache refreshed successfully",
		telemetry.WithTraceContext(ctx,
			"genres_count", len(genres),
			"basic_shows_categories", len(basicShows),
			"basic_movies_categories", len(basicMovies),
			"enriched_shows_categories", len(enrichedShows),
			"enriched_movies_categories", len(enrichedMovies),
		)...)

	// Set span attributes for observability
	span.SetAttributes(
		attribute.Int("genres.count", len(genres)),
		attribute.Int("basic_shows.categories", len(basicShows)),
		attribute.Int("basic_movies.categories", len(basicMovies)),
		attribute.Int("enriched_shows.categories", len(enrichedShows)),
		attribute.Int("enriched_movies.categories", len(enrichedMovies)),
		attribute.String("refresh.status", "success"),
	)

	// Check if we got partial results (warn if less than expected)
	expectedCategories := len(categories)
	if len(basicShows) < expectedCategories || len(basicMovies) < expectedCategories {
		r.logger.WarnContext(ctx, "Partial refresh: some categories failed to load",
			telemetry.WithTraceContext(ctx,
				"expected_categories", expectedCategories,
				"basic_shows_loaded", len(basicShows),
				"basic_movies_loaded", len(basicMovies),
			)...)
	}
}
