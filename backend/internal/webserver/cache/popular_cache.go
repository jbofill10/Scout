package cache

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// PopularCache provides thread-safe in-memory caching for popular content
type PopularCache struct {
	mu                sync.RWMutex
	genres            []clients.Genre
	basicShows        map[string][]tvdb.Media           // genre -> shows
	basicMovies       map[string][]tvdb.Media           // genre -> movies
	enrichedShows     map[string][]tvdb.EnrichedMedia   // genre -> enriched shows
	enrichedMovies    map[string][]tvdb.EnrichedMedia   // genre -> enriched movies
	lastRefreshTime   time.Time
	refreshInProgress bool
	failureCount      int
	tracer            trace.Tracer
}

// CacheStats provides metrics about cache state
type CacheStats struct {
	LastRefreshTime       time.Time
	GenresCount           int
	BasicShowsCategories  int
	BasicMoviesCategories int
	EnrichedShowsCount    int
	EnrichedMoviesCount   int
	RefreshInProgress     bool
	ConsecutiveFailures   int
	IsStale               bool
}

// NewPopularCache creates a new popular content cache
func NewPopularCache() *PopularCache {
	return &PopularCache{
		genres:         []clients.Genre{},
		basicShows:     make(map[string][]tvdb.Media),
		basicMovies:    make(map[string][]tvdb.Media),
		enrichedShows:  make(map[string][]tvdb.EnrichedMedia),
		enrichedMovies: make(map[string][]tvdb.EnrichedMedia),
		tracer:         otel.Tracer("webserver"),
	}
}

// GetGenres retrieves cached genres if available
// Returns the genres and true if found in cache, empty slice and false otherwise
func (c *PopularCache) GetGenres(ctx context.Context) ([]clients.Genre, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.genres) == 0 {
		return nil, false
	}

	// Return a copy to prevent external modifications
	genresCopy := make([]clients.Genre, len(c.genres))
	copy(genresCopy, c.genres)

	return genresCopy, true
}

// GetBasicShows retrieves cached basic shows for a genre
// Returns the shows and true if found in cache, nil and false otherwise
func (c *PopularCache) GetBasicShows(ctx context.Context, genre string) ([]tvdb.Media, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	shows, exists := c.basicShows[genre]
	if !exists || len(shows) == 0 {
		return nil, false
	}

	// Return a copy to prevent external modifications
	showsCopy := make([]tvdb.Media, len(shows))
	copy(showsCopy, shows)

	return showsCopy, true
}

// GetBasicMovies retrieves cached basic movies for a genre
// Returns the movies and true if found in cache, nil and false otherwise
func (c *PopularCache) GetBasicMovies(ctx context.Context, genre string) ([]tvdb.Media, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	movies, exists := c.basicMovies[genre]
	if !exists || len(movies) == 0 {
		return nil, false
	}

	// Return a copy to prevent external modifications
	moviesCopy := make([]tvdb.Media, len(movies))
	copy(moviesCopy, movies)

	return moviesCopy, true
}

// GetEnrichedShows retrieves cached enriched shows for a genre
// Returns the shows and true if found in cache, nil and false otherwise
func (c *PopularCache) GetEnrichedShows(ctx context.Context, genre string) ([]tvdb.EnrichedMedia, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	shows, exists := c.enrichedShows[genre]
	if !exists || len(shows) == 0 {
		return nil, false
	}

	// Return a copy to prevent external modifications
	showsCopy := make([]tvdb.EnrichedMedia, len(shows))
	copy(showsCopy, shows)

	return showsCopy, true
}

// GetEnrichedMovies retrieves cached enriched movies for a genre
// Returns the movies and true if found in cache, nil and false otherwise
func (c *PopularCache) GetEnrichedMovies(ctx context.Context, genre string) ([]tvdb.EnrichedMedia, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	movies, exists := c.enrichedMovies[genre]
	if !exists || len(movies) == 0 {
		return nil, false
	}

	// Return a copy to prevent external modifications
	moviesCopy := make([]tvdb.EnrichedMedia, len(movies))
	copy(moviesCopy, movies)

	return moviesCopy, true
}

// updateAll atomically updates all cached data
// This is called by the refresher after fetching new data
func (c *PopularCache) updateAll(
	genres []clients.Genre,
	basicShows map[string][]tvdb.Media,
	basicMovies map[string][]tvdb.Media,
	enrichedShows map[string][]tvdb.EnrichedMedia,
	enrichedMovies map[string][]tvdb.EnrichedMedia,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.genres = genres
	c.basicShows = basicShows
	c.basicMovies = basicMovies
	c.enrichedShows = enrichedShows
	c.enrichedMovies = enrichedMovies
	c.lastRefreshTime = time.Now()
	c.failureCount = 0 // Reset failure count on successful update
}

// markRefreshStart marks that a refresh is in progress
func (c *PopularCache) markRefreshStart() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.refreshInProgress {
		return false // Already in progress
	}

	c.refreshInProgress = true
	return true
}

// markRefreshEnd marks that a refresh has completed
func (c *PopularCache) markRefreshEnd(success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.refreshInProgress = false

	if !success {
		c.failureCount++
	}
}

// IsStale returns true if cache hasn't been refreshed in 6+ hours
func (c *PopularCache) IsStale() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastRefreshTime.IsZero() {
		return true // Never refreshed
	}

	return time.Since(c.lastRefreshTime) > 6*time.Hour
}

// GetCacheStats returns metrics about cache state
func (c *PopularCache) GetCacheStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Compute IsStale inline to avoid recursive lock acquisition
	isStale := c.lastRefreshTime.IsZero() || time.Since(c.lastRefreshTime) > 6*time.Hour

	return CacheStats{
		LastRefreshTime:       c.lastRefreshTime,
		GenresCount:           len(c.genres),
		BasicShowsCategories:  len(c.basicShows),
		BasicMoviesCategories: len(c.basicMovies),
		EnrichedShowsCount:    len(c.enrichedShows),
		EnrichedMoviesCount:   len(c.enrichedMovies),
		RefreshInProgress:     c.refreshInProgress,
		ConsecutiveFailures:   c.failureCount,
		IsStale:               isStale,
	}
}

// Clear removes all cached data
// Useful for testing or manual cache invalidation
func (c *PopularCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.genres = []clients.Genre{}
	c.basicShows = make(map[string][]tvdb.Media)
	c.basicMovies = make(map[string][]tvdb.Media)
	c.enrichedShows = make(map[string][]tvdb.EnrichedMedia)
	c.enrichedMovies = make(map[string][]tvdb.EnrichedMedia)
	c.lastRefreshTime = time.Time{}
	c.failureCount = 0
}
