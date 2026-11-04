package cache

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// Mock implementations for testing

type mockTVDBClient struct {
	genres           []clients.Genre
	genresErr        error
	popularShows     map[string][]tvdb.Media
	popularShowsErr  error
	popularMovies    map[string][]tvdb.Media
	popularMoviesErr error
	// Counters are written from the refresher's background goroutine and read
	// by the test goroutine, so they must be race-free.
	getGenresCalls        atomic.Int64
	getPopularShowsCalls  atomic.Int64
	getPopularMoviesCalls atomic.Int64
	// failShowsCalls makes the first N GetPopularShows calls fail, to stand in
	// for a tvdb-proxy that is still building its rows.
	failShowsCalls atomic.Int64
}

func (m *mockTVDBClient) GetGenres(ctx context.Context) ([]clients.Genre, error) {
	m.getGenresCalls.Add(1)
	return m.genres, m.genresErr
}

func (m *mockTVDBClient) GetPopularShows(ctx context.Context, genre string, limit int) ([]tvdb.Media, error) {
	call := m.getPopularShowsCalls.Add(1)
	if m.popularShowsErr != nil {
		return nil, m.popularShowsErr
	}
	if call <= m.failShowsCalls.Load() {
		return nil, errors.New("tvdb-proxy still building rows")
	}
	if shows, ok := m.popularShows[genre]; ok {
		return shows, nil
	}
	return []tvdb.Media{}, nil
}

func (m *mockTVDBClient) GetPopularMovies(ctx context.Context, genre string, limit int) ([]tvdb.Media, error) {
	m.getPopularMoviesCalls.Add(1)
	if m.popularMoviesErr != nil {
		return nil, m.popularMoviesErr
	}
	if movies, ok := m.popularMovies[genre]; ok {
		return movies, nil
	}
	return []tvdb.Media{}, nil
}

type mockTorrenterClient struct {
	statusResponse tvdb.StatusBatchResponse
	statusErr      error
}

func (m *mockTorrenterClient) GetStatusBatch(ctx context.Context, requests []tvdb.StatusRequest) (tvdb.StatusBatchResponse, error) {
	return m.statusResponse, m.statusErr
}

type mockPopularEnrichedInteractor struct {
	enrichedShows       map[string][]tvdb.EnrichedMedia
	enrichedShowsErr    error
	enrichedMovies      map[string][]tvdb.EnrichedMedia
	enrichedMoviesErr   error
	enrichedShowsCalls  atomic.Int64
	enrichedMoviesCalls atomic.Int64
}

func (m *mockPopularEnrichedInteractor) EnrichedPopularShows(ctx context.Context, genre string, limit int) ([]tvdb.EnrichedMedia, error) {
	m.enrichedShowsCalls.Add(1)
	if m.enrichedShowsErr != nil {
		return nil, m.enrichedShowsErr
	}
	if shows, ok := m.enrichedShows[genre]; ok {
		return shows, nil
	}
	return []tvdb.EnrichedMedia{}, nil
}

func (m *mockPopularEnrichedInteractor) EnrichedPopularMovies(ctx context.Context, genre string, limit int) ([]tvdb.EnrichedMedia, error) {
	m.enrichedMoviesCalls.Add(1)
	if m.enrichedMoviesErr != nil {
		return nil, m.enrichedMoviesErr
	}
	if movies, ok := m.enrichedMovies[genre]; ok {
		return movies, nil
	}
	return []tvdb.EnrichedMedia{}, nil
}

func TestNewPopularRefresher(t *testing.T) {
	cache := NewPopularCache()
	tvdbClient := &mockTVDBClient{}
	torrenterClient := &mockTorrenterClient{}
	interactor := &mockPopularEnrichedInteractor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	refresher := NewPopularRefresher(cache, tvdbClient, torrenterClient, interactor, logger)

	if refresher == nil {
		t.Fatal("NewPopularRefresher returned nil")
	}

	if refresher.cache != cache {
		t.Error("Cache not set correctly")
	}

	if refresher.refreshInterval != 6*time.Hour {
		t.Errorf("Expected refresh interval of 6 hours, got %v", refresher.refreshInterval)
	}

	if refresher.limit != 20 {
		t.Error("Expected default limit of 20")
	}
}

func TestRefreshAll_Success(t *testing.T) {
	cache := NewPopularCache()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Setup mock responses
	tvdbClient := &mockTVDBClient{
		genres: []clients.Genre{
			{ID: 1, Name: "Action", Slug: "action"},
			{ID: 2, Name: "Comedy", Slug: "comedy"},
		},
		popularShows: map[string][]tvdb.Media{
			"":       {{Id: "1", Name: "Popular Show"}},
			"Action": {{Id: "2", Name: "Action Show"}},
		},
		popularMovies: map[string][]tvdb.Media{
			"":       {{Id: "3", Name: "Popular Movie"}},
			"Action": {{Id: "4", Name: "Action Movie"}},
		},
	}

	torrenterClient := &mockTorrenterClient{}

	interactor := &mockPopularEnrichedInteractor{
		enrichedShows: map[string][]tvdb.EnrichedMedia{
			"": {{Media: tvdb.Media{Id: "5", Name: "Enriched Show"}}},
		},
		enrichedMovies: map[string][]tvdb.EnrichedMedia{
			"": {{Media: tvdb.Media{Id: "6", Name: "Enriched Movie"}}},
		},
	}

	refresher := NewPopularRefresher(cache, tvdbClient, torrenterClient, interactor, logger)

	// Execute refresh
	refresher.refreshAll()

	// Verify cache was populated
	ctx := context.Background()

	genres, hit := cache.GetGenres(ctx)
	if !hit || len(genres) != 2 {
		t.Errorf("Expected 2 genres in cache, got %d (hit: %v)", len(genres), hit)
	}

	shows, hit := cache.GetBasicShows(ctx, "Action")
	if !hit || len(shows) != 1 {
		t.Errorf("Expected 1 action show in cache, got %d (hit: %v)", len(shows), hit)
	}

	movies, hit := cache.GetBasicMovies(ctx, "Action")
	if !hit || len(movies) != 1 {
		t.Errorf("Expected 1 action movie in cache, got %d (hit: %v)", len(movies), hit)
	}

	// Verify API calls were made
	// 9 categories: "", "Action", "Comedy", "Drama", "Sci-Fi", "Horror", "Thriller", "Anime", "Documentary"
	if tvdbClient.getPopularShowsCalls.Load() != 9 {
		t.Errorf("Expected 9 GetPopularShows calls, got %d", tvdbClient.getPopularShowsCalls.Load())
	}

	if tvdbClient.getPopularMoviesCalls.Load() != 9 {
		t.Errorf("Expected 9 GetPopularMovies calls, got %d", tvdbClient.getPopularMoviesCalls.Load())
	}
}

func TestRefreshAll_GenresFailure(t *testing.T) {
	cache := NewPopularCache()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Setup mock to fail genres fetch
	tvdbClient := &mockTVDBClient{
		genresErr: errors.New("genres fetch failed"),
	}

	torrenterClient := &mockTorrenterClient{}
	interactor := &mockPopularEnrichedInteractor{}

	refresher := NewPopularRefresher(cache, tvdbClient, torrenterClient, interactor, logger)

	// Execute refresh
	refresher.refreshAll()

	// Verify cache is still empty
	ctx := context.Background()
	genres, hit := cache.GetGenres(ctx)
	if hit || len(genres) != 0 {
		t.Error("Expected cache to remain empty after genres failure")
	}

	// Verify failure was tracked
	stats := cache.GetCacheStats()
	if stats.ConsecutiveFailures != 1 {
		t.Errorf("Expected 1 failure, got %d", stats.ConsecutiveFailures)
	}
}

func TestRefreshAll_PartialFailure(t *testing.T) {
	cache := NewPopularCache()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Setup mock with genres success but shows failure
	tvdbClient := &mockTVDBClient{
		genres: []clients.Genre{
			{ID: 1, Name: "Action", Slug: "action"},
		},
		popularShowsErr: errors.New("shows fetch failed"),
		popularMovies: map[string][]tvdb.Media{
			"":       {{Id: "1", Name: "Popular Movie"}}, // For generic popular content
			"Action": {{Id: "2", Name: "Action Movie"}},
		},
	}

	torrenterClient := &mockTorrenterClient{}
	interactor := &mockPopularEnrichedInteractor{
		enrichedShows: map[string][]tvdb.EnrichedMedia{},
		enrichedMovies: map[string][]tvdb.EnrichedMedia{
			"":       {{Media: tvdb.Media{Id: "3", Name: "Popular Enriched Movie"}}},
			"Action": {{Media: tvdb.Media{Id: "4", Name: "Action Enriched Movie"}}},
		},
	}

	refresher := NewPopularRefresher(cache, tvdbClient, torrenterClient, interactor, logger)

	// Execute refresh
	refresher.refreshAll()

	// Verify partial data was cached
	ctx := context.Background()

	genres, hit := cache.GetGenres(ctx)
	if !hit || len(genres) != 1 {
		t.Error("Expected genres to be cached despite shows failure")
	}

	movies, hit := cache.GetBasicMovies(ctx, "Action")
	if !hit || len(movies) != 1 {
		t.Error("Expected movies to be cached despite shows failure")
	}

	// Shows should be empty/not found
	_, hit = cache.GetBasicShows(ctx, "Action")
	if hit {
		t.Error("Expected shows to not be cached due to failure")
	}
}

func TestRefreshAll_AlreadyInProgress(t *testing.T) {
	cache := NewPopularCache()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tvdbClient := &mockTVDBClient{
		genres: []clients.Genre{{ID: 1, Name: "Action", Slug: "action"}},
	}
	torrenterClient := &mockTorrenterClient{}
	interactor := &mockPopularEnrichedInteractor{}

	refresher := NewPopularRefresher(cache, tvdbClient, torrenterClient, interactor, logger)

	// Mark refresh as in progress
	cache.markRefreshStart()

	// Try to refresh (should skip)
	refresher.refreshAll()

	// Verify no API calls were made (refresh was skipped)
	if tvdbClient.getGenresCalls.Load() != 0 {
		t.Error("Expected refresh to be skipped when already in progress")
	}
}

func TestStartStop(t *testing.T) {
	cache := NewPopularCache()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tvdbClient := &mockTVDBClient{
		genres: []clients.Genre{{ID: 1, Name: "Action", Slug: "action"}},
		popularShows: map[string][]tvdb.Media{
			"": {{Id: "1", Name: "Show"}},
		},
		popularMovies: map[string][]tvdb.Media{
			"": {{Id: "2", Name: "Movie"}},
		},
	}
	torrenterClient := &mockTorrenterClient{}
	interactor := &mockPopularEnrichedInteractor{
		enrichedShows: map[string][]tvdb.EnrichedMedia{
			"": {{Media: tvdb.Media{Id: "3", Name: "Show"}}},
		},
		enrichedMovies: map[string][]tvdb.EnrichedMedia{
			"": {{Media: tvdb.Media{Id: "4", Name: "Movie"}}},
		},
	}

	refresher := NewPopularRefresher(cache, tvdbClient, torrenterClient, interactor, logger)

	// Override refresh interval for faster testing
	refresher.refreshInterval = 100 * time.Millisecond

	// Start refresher (background refresh + periodic goroutine)
	refresher.Start()

	// The first refresh runs in the background; wait for it to land
	ctx := context.Background()
	deadline := time.Now().Add(2 * time.Second)
	for {
		genres, hit := cache.GetGenres(ctx)
		if hit && len(genres) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Expected initial refresh to populate cache")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Stop refresher
	refresher.Stop()

	// Wait for goroutine to stop
	time.Sleep(50 * time.Millisecond)

	// Test passes if no panic or deadlock
}

func TestRefreshInterval_PeriodicRefresh(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping periodic refresh test in short mode")
	}

	cache := NewPopularCache()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tvdbClient := &mockTVDBClient{
		genres: []clients.Genre{{ID: 1, Name: "Action", Slug: "action"}},
		popularShows: map[string][]tvdb.Media{
			"": {{Id: "1", Name: "Show"}},
		},
		popularMovies: map[string][]tvdb.Media{
			"": {{Id: "2", Name: "Movie"}},
		},
	}
	torrenterClient := &mockTorrenterClient{}
	interactor := &mockPopularEnrichedInteractor{
		enrichedShows: map[string][]tvdb.EnrichedMedia{
			"": {{Media: tvdb.Media{Id: "3", Name: "Show"}}},
		},
		enrichedMovies: map[string][]tvdb.EnrichedMedia{
			"": {{Media: tvdb.Media{Id: "4", Name: "Movie"}}},
		},
	}

	refresher := NewPopularRefresher(cache, tvdbClient, torrenterClient, interactor, logger)

	// Override refresh interval for faster testing
	refresher.refreshInterval = 200 * time.Millisecond

	// Start refresher
	refresher.Start()
	defer refresher.Stop()

	// Wait for at least 2 refresh cycles
	time.Sleep(500 * time.Millisecond)

	// Verify multiple refreshes occurred (initial + periodic)
	// GetGenres should be called at least 2 times (initial + 1-2 periodic)
	if tvdbClient.getGenresCalls.Load() < 2 {
		t.Errorf("Expected at least 2 GetGenres calls, got %d", tvdbClient.getGenresCalls.Load())
	}
}

func TestRefreshAll_CacheStatsUpdate(t *testing.T) {
	cache := NewPopularCache()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tvdbClient := &mockTVDBClient{
		genres: []clients.Genre{
			{ID: 1, Name: "Action", Slug: "action"},
		},
		popularShows: map[string][]tvdb.Media{
			"":       {{Id: "1", Name: "Show 1"}},
			"Action": {{Id: "2", Name: "Show 2"}},
		},
		popularMovies: map[string][]tvdb.Media{
			"":       {{Id: "3", Name: "Movie 1"}},
			"Action": {{Id: "4", Name: "Movie 2"}},
		},
	}
	torrenterClient := &mockTorrenterClient{}
	interactor := &mockPopularEnrichedInteractor{
		enrichedShows: map[string][]tvdb.EnrichedMedia{
			"": {{Media: tvdb.Media{Id: "5", Name: "Show"}}},
		},
		enrichedMovies: map[string][]tvdb.EnrichedMedia{
			"": {{Media: tvdb.Media{Id: "6", Name: "Movie"}}},
		},
	}

	refresher := NewPopularRefresher(cache, tvdbClient, torrenterClient, interactor, logger)

	// Mark as failed initially
	cache.markRefreshStart()
	cache.markRefreshEnd(false)

	stats := cache.GetCacheStats()
	if stats.ConsecutiveFailures != 1 {
		t.Error("Expected 1 failure before successful refresh")
	}

	// Execute successful refresh
	refresher.refreshAll()

	// Verify stats updated
	stats = cache.GetCacheStats()

	if stats.ConsecutiveFailures != 0 {
		t.Error("Expected failure count to be reset after successful refresh")
	}

	if stats.LastRefreshTime.IsZero() {
		t.Error("Expected LastRefreshTime to be set")
	}

	if stats.IsStale {
		t.Error("Expected cache not to be stale after refresh")
	}

	if stats.GenresCount != 1 {
		t.Errorf("Expected 1 genre, got %d", stats.GenresCount)
	}
}

// A refresh that leaves categories empty is retried after retryInterval
// instead of waiting for the next six-hourly tick.
func TestStart_RetriesPartialRefresh(t *testing.T) {
	cache := NewPopularCache()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tvdbClient := &mockTVDBClient{
		genres:        []clients.Genre{{ID: 1, Name: "Action", Slug: "action"}},
		popularShows:  map[string][]tvdb.Media{"": {{Id: "1", Name: "Show"}}},
		popularMovies: map[string][]tvdb.Media{"": {{Id: "2", Name: "Movie"}}},
	}
	// The first refresh asks for shows once per category (9); every one fails.
	tvdbClient.failShowsCalls.Store(9)
	interactor := &mockPopularEnrichedInteractor{
		enrichedShows:  map[string][]tvdb.EnrichedMedia{"": {{Media: tvdb.Media{Id: "3"}}}},
		enrichedMovies: map[string][]tvdb.EnrichedMedia{"": {{Media: tvdb.Media{Id: "4"}}}},
	}
	refresher := NewPopularRefresher(cache, tvdbClient, &mockTorrenterClient{}, interactor, logger)
	refresher.refreshInterval = time.Hour
	refresher.retryInterval = 30 * time.Millisecond

	refresher.Start()
	defer refresher.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for {
		stats := cache.GetCacheStats()
		if stats.BasicShowsCategories == 9 && stats.BasicMoviesCategories == 9 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected the retry to fill every category, got %+v", stats)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if calls := tvdbClient.getPopularShowsCalls.Load(); calls < 18 {
		t.Errorf("expected a second refresh (at least 18 show calls), got %d", calls)
	}
}
