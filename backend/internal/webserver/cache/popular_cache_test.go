package cache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

func TestNewPopularCache(t *testing.T) {
	cache := NewPopularCache()

	if cache == nil {
		t.Fatal("NewPopularCache returned nil")
	}

	if cache.basicShows == nil || cache.basicMovies == nil {
		t.Error("Cache maps not initialized")
	}

	if cache.enrichedShows == nil || cache.enrichedMovies == nil {
		t.Error("Cache enriched maps not initialized")
	}
}

func TestGetGenres_EmptyCache(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	genres, hit := cache.GetGenres(ctx)

	if hit {
		t.Error("Expected cache miss for empty cache")
	}

	if genres != nil {
		t.Error("Expected nil genres for cache miss")
	}
}

func TestGetGenres_CacheHit(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	// Populate cache
	testGenres := []clients.Genre{
		{ID: 1, Name: "Action", Slug: "action"},
		{ID: 2, Name: "Comedy", Slug: "comedy"},
	}

	cache.updateAll(testGenres, nil, nil, nil, nil)

	// Retrieve from cache
	genres, hit := cache.GetGenres(ctx)

	if !hit {
		t.Error("Expected cache hit")
	}

	if len(genres) != 2 {
		t.Errorf("Expected 2 genres, got %d", len(genres))
	}

	if genres[0].Name != "Action" {
		t.Errorf("Expected first genre to be Action, got %s", genres[0].Name)
	}
}

func TestGetBasicShows_CacheMiss(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	shows, hit := cache.GetBasicShows(ctx, "Action")

	if hit {
		t.Error("Expected cache miss for empty cache")
	}

	if shows != nil {
		t.Error("Expected nil shows for cache miss")
	}
}

func TestGetBasicShows_CacheHit(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	// Populate cache
	testShows := map[string][]tvdb.Media{
		"Action": {
			{Id: "123", Name: "Test Show 1"},
			{Id: "456", Name: "Test Show 2"},
		},
	}

	cache.updateAll(nil, testShows, nil, nil, nil)

	// Retrieve from cache
	shows, hit := cache.GetBasicShows(ctx, "Action")

	if !hit {
		t.Error("Expected cache hit")
	}

	if len(shows) != 2 {
		t.Errorf("Expected 2 shows, got %d", len(shows))
	}

	if shows[0].Name != "Test Show 1" {
		t.Errorf("Expected first show to be 'Test Show 1', got %s", shows[0].Name)
	}
}

func TestGetBasicMovies_CacheHit(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	// Populate cache
	testMovies := map[string][]tvdb.Media{
		"Comedy": {
			{Id: "789", Name: "Test Movie 1"},
		},
	}

	cache.updateAll(nil, nil, testMovies, nil, nil)

	// Retrieve from cache
	movies, hit := cache.GetBasicMovies(ctx, "Comedy")

	if !hit {
		t.Error("Expected cache hit")
	}

	if len(movies) != 1 {
		t.Errorf("Expected 1 movie, got %d", len(movies))
	}
}

func TestGetEnrichedShows_CacheHit(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	// Populate cache
	testShows := map[string][]tvdb.EnrichedMedia{
		"Drama": {
			{Media: tvdb.Media{Id: "111", Name: "Enriched Show 1"}},
		},
	}

	cache.updateAll(nil, nil, nil, testShows, nil)

	// Retrieve from cache
	shows, hit := cache.GetEnrichedShows(ctx, "Drama")

	if !hit {
		t.Error("Expected cache hit")
	}

	if len(shows) != 1 {
		t.Errorf("Expected 1 show, got %d", len(shows))
	}
}

func TestGetEnrichedMovies_CacheHit(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	// Populate cache
	testMovies := map[string][]tvdb.EnrichedMedia{
		"Sci-Fi": {
			{Media: tvdb.Media{Id: "222", Name: "Enriched Movie 1"}},
		},
	}

	cache.updateAll(nil, nil, nil, nil, testMovies)

	// Retrieve from cache
	movies, hit := cache.GetEnrichedMovies(ctx, "Sci-Fi")

	if !hit {
		t.Error("Expected cache hit")
	}

	if len(movies) != 1 {
		t.Errorf("Expected 1 movie, got %d", len(movies))
	}
}

func TestUpdateAll_AtomicUpdate(t *testing.T) {
	cache := NewPopularCache()

	testGenres := []clients.Genre{
		{ID: 1, Name: "Action", Slug: "action"},
	}
	testShows := map[string][]tvdb.Media{
		"Action": {{Id: "123", Name: "Show 1"}},
	}
	testMovies := map[string][]tvdb.Media{
		"Action": {{Id: "456", Name: "Movie 1"}},
	}
	testEnrichedShows := map[string][]tvdb.EnrichedMedia{
		"Action": {{Media: tvdb.Media{Id: "789", Name: "Enriched Show"}}},
	}
	testEnrichedMovies := map[string][]tvdb.EnrichedMedia{
		"Action": {{Media: tvdb.Media{Id: "101", Name: "Enriched Movie"}}},
	}

	// Update cache
	cache.updateAll(testGenres, testShows, testMovies, testEnrichedShows, testEnrichedMovies)

	// Verify all data is updated
	ctx := context.Background()

	genres, _ := cache.GetGenres(ctx)
	if len(genres) != 1 {
		t.Error("Genres not updated")
	}

	shows, _ := cache.GetBasicShows(ctx, "Action")
	if len(shows) != 1 {
		t.Error("Basic shows not updated")
	}

	movies, _ := cache.GetBasicMovies(ctx, "Action")
	if len(movies) != 1 {
		t.Error("Basic movies not updated")
	}

	enrichedShows, _ := cache.GetEnrichedShows(ctx, "Action")
	if len(enrichedShows) != 1 {
		t.Error("Enriched shows not updated")
	}

	enrichedMovies, _ := cache.GetEnrichedMovies(ctx, "Action")
	if len(enrichedMovies) != 1 {
		t.Error("Enriched movies not updated")
	}

	// Verify lastRefreshTime is set
	if cache.lastRefreshTime.IsZero() {
		t.Error("lastRefreshTime not set after update")
	}

	// Verify failure count is reset
	cache.mu.Lock()
	cache.failureCount = 5
	cache.mu.Unlock()

	cache.updateAll(testGenres, testShows, testMovies, testEnrichedShows, testEnrichedMovies)

	cache.mu.RLock()
	if cache.failureCount != 0 {
		t.Errorf("Expected failure count to be reset to 0, got %d", cache.failureCount)
	}
	cache.mu.RUnlock()
}

func TestIsStale_NeverRefreshed(t *testing.T) {
	cache := NewPopularCache()

	if !cache.IsStale() {
		t.Error("Expected cache to be stale when never refreshed")
	}
}

func TestIsStale_RecentRefresh(t *testing.T) {
	cache := NewPopularCache()

	// Simulate recent refresh
	cache.updateAll(nil, nil, nil, nil, nil)

	if cache.IsStale() {
		t.Error("Expected cache to not be stale after recent refresh")
	}
}

func TestIsStale_OldRefresh(t *testing.T) {
	cache := NewPopularCache()

	// Simulate old refresh (7 hours ago)
	cache.mu.Lock()
	cache.lastRefreshTime = time.Now().Add(-7 * time.Hour)
	cache.mu.Unlock()

	if !cache.IsStale() {
		t.Error("Expected cache to be stale after 7 hours")
	}
}

func TestGetCacheStats(t *testing.T) {
	cache := NewPopularCache()

	// Initial stats (empty cache)
	stats := cache.GetCacheStats()

	if stats.GenresCount != 0 {
		t.Errorf("Expected 0 genres, got %d", stats.GenresCount)
	}

	if !stats.IsStale {
		t.Error("Expected cache to be stale initially")
	}

	if stats.RefreshInProgress {
		t.Error("Expected refresh not in progress initially")
	}

	// Populate cache and check stats
	testGenres := []clients.Genre{{ID: 1, Name: "Action", Slug: "action"}}
	testShows := map[string][]tvdb.Media{
		"Action": {{Id: "123", Name: "Show 1"}},
		"Comedy": {{Id: "456", Name: "Show 2"}},
	}
	testMovies := map[string][]tvdb.Media{
		"Drama": {{Id: "789", Name: "Movie 1"}},
	}

	cache.updateAll(testGenres, testShows, testMovies, nil, nil)

	stats = cache.GetCacheStats()

	if stats.GenresCount != 1 {
		t.Errorf("Expected 1 genre, got %d", stats.GenresCount)
	}

	if stats.BasicShowsCategories != 2 {
		t.Errorf("Expected 2 show categories, got %d", stats.BasicShowsCategories)
	}

	if stats.BasicMoviesCategories != 1 {
		t.Errorf("Expected 1 movie category, got %d", stats.BasicMoviesCategories)
	}

	if stats.IsStale {
		t.Error("Expected cache not to be stale after update")
	}
}

func TestClear(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	// Populate cache
	testGenres := []clients.Genre{{ID: 1, Name: "Action", Slug: "action"}}
	testShows := map[string][]tvdb.Media{
		"Action": {{Id: "123", Name: "Show 1"}},
	}

	cache.updateAll(testGenres, testShows, nil, nil, nil)

	// Verify data exists
	genres, hit := cache.GetGenres(ctx)
	if !hit || len(genres) == 0 {
		t.Error("Expected data in cache before clear")
	}

	// Clear cache
	cache.Clear()

	// Verify data is gone
	_, hit = cache.GetGenres(ctx)
	if hit {
		t.Error("Expected cache miss after clear")
	}

	_, hit = cache.GetBasicShows(ctx, "Action")
	if hit {
		t.Error("Expected cache miss for shows after clear")
	}

	// Verify lastRefreshTime is reset
	if !cache.lastRefreshTime.IsZero() {
		t.Error("Expected lastRefreshTime to be zero after clear")
	}
}

func TestConcurrency(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	// Populate initial data
	testShows := map[string][]tvdb.Media{
		"Action": {{Id: "123", Name: "Show 1"}},
	}
	cache.updateAll(nil, testShows, nil, nil, nil)

	var wg sync.WaitGroup
	numReaders := 10
	numWriters := 2

	// Concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cache.GetBasicShows(ctx, "Action")
				cache.GetGenres(ctx)
				cache.IsStale()
				cache.GetCacheStats()
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				newShows := map[string][]tvdb.Media{
					"Action": {{Id: "123", Name: "Show 1"}},
				}
				cache.updateAll(nil, newShows, nil, nil, nil)
			}
		}(i)
	}

	wg.Wait()

	// If we reach here without data races, test passes
}

func TestMarkRefreshStartEnd(t *testing.T) {
	cache := NewPopularCache()

	// Start refresh
	started := cache.markRefreshStart()
	if !started {
		t.Error("Expected refresh to start")
	}

	// Verify refresh in progress
	stats := cache.GetCacheStats()
	if !stats.RefreshInProgress {
		t.Error("Expected refresh to be in progress")
	}

	// Try to start again (should fail)
	started = cache.markRefreshStart()
	if started {
		t.Error("Expected refresh start to fail when already in progress")
	}

	// End refresh (success)
	cache.markRefreshEnd(true)

	stats = cache.GetCacheStats()
	if stats.RefreshInProgress {
		t.Error("Expected refresh not to be in progress after end")
	}

	if stats.ConsecutiveFailures != 0 {
		t.Errorf("Expected 0 failures after successful refresh, got %d", stats.ConsecutiveFailures)
	}

	// Start and fail
	cache.markRefreshStart()
	cache.markRefreshEnd(false)

	stats = cache.GetCacheStats()
	if stats.ConsecutiveFailures != 1 {
		t.Errorf("Expected 1 failure, got %d", stats.ConsecutiveFailures)
	}

	// Fail again
	cache.markRefreshStart()
	cache.markRefreshEnd(false)

	stats = cache.GetCacheStats()
	if stats.ConsecutiveFailures != 2 {
		t.Errorf("Expected 2 failures, got %d", stats.ConsecutiveFailures)
	}
}

func TestGetMethods_ReturnCopies(t *testing.T) {
	cache := NewPopularCache()
	ctx := context.Background()

	// Populate cache
	testShows := map[string][]tvdb.Media{
		"Action": {{Id: "123", Name: "Show 1"}},
	}
	cache.updateAll(nil, testShows, nil, nil, nil)

	// Get shows and modify returned slice
	shows1, hit := cache.GetBasicShows(ctx, "Action")
	if !hit {
		t.Fatal("Expected cache hit")
	}

	shows1[0].Name = "Modified Show"

	// Get shows again and verify not modified
	shows2, hit := cache.GetBasicShows(ctx, "Action")
	if !hit {
		t.Fatal("Expected cache hit")
	}

	if shows2[0].Name != "Show 1" {
		t.Error("Cache data was modified by external code (not a copy)")
	}
}
