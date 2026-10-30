// Package genres caches TVDB's genre list for the /genres endpoint and for
// resolving genre names to ids in the popular-media filters.
package genres

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/pkg/media"
)

// refreshEvery is how long a fetched genre list is served before it is
// considered stale (weekly, as per design.md).
const refreshEvery = 7 * 24 * time.Hour

// Fetcher performs an authenticated TVDB GET; the tvdb client implements it.
type Fetcher interface {
	Get(ctx context.Context, uri string) ([]byte, error)
}

// Cache holds TVDB's genres with a case-insensitive name and slug index.
type Cache struct {
	fetcher Fetcher
	logger  *slog.Logger

	mu           sync.RWMutex
	genres       []media.Genres
	nameToID     map[string]int
	lastRefresh  time.Time
	refreshEvery time.Duration
}

// NewCache returns an empty cache; call Refresh to fill it.
func NewCache(fetcher Fetcher, logger *slog.Logger) *Cache {
	return &Cache{
		fetcher:      fetcher,
		logger:       logger,
		nameToID:     make(map[string]int),
		refreshEvery: refreshEvery,
	}
}

// Refresh re-fetches the genre list from TVDB and rebuilds the lookup index.
func (gc *Cache) Refresh(ctx context.Context) error {
	gc.logger.InfoContext(ctx, "Refreshing genre cache")

	// Call TVDB API to fetch genres
	result, err := gc.fetcher.Get(ctx, "/genres")
	if err != nil {
		gc.logger.ErrorContext(ctx, "Failed to fetch genres from TVDB", "error", err)
		return err
	}

	// Parse response
	var genresResponse struct {
		Status string         `json:"status"`
		Data   []media.Genres `json:"data"`
	}

	if err := json.Unmarshal(result, &genresResponse); err != nil {
		gc.logger.ErrorContext(ctx, "Failed to unmarshal genres response", "error", err)
		return err
	}

	// Update cache with write lock
	gc.mu.Lock()
	defer gc.mu.Unlock()

	gc.genres = genresResponse.Data
	gc.lastRefresh = time.Now()

	// Build name → ID mapping with case-insensitive keys
	gc.nameToID = make(map[string]int)
	for _, genre := range gc.genres {
		// Store by lowercase name for case-insensitive lookup
		gc.nameToID[strings.ToLower(genre.Name)] = genre.Id
		// Also store by slug
		gc.nameToID[strings.ToLower(genre.Slug)] = genre.Id
	}

	// Add "Sci-Fi" → "Science Fiction" alias as per design.md
	if id, ok := gc.nameToID["science fiction"]; ok {
		gc.nameToID["sci-fi"] = id
	}

	gc.logger.InfoContext(ctx, "Genre cache refreshed", "genres_count", len(gc.genres))
	return nil
}

// Genres returns the cached genre list.
func (gc *Cache) Genres() []media.Genres {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return gc.genres
}

// NeedsRefresh reports whether the cached list is older than the refresh period.
func (gc *Cache) NeedsRefresh() bool {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return time.Since(gc.lastRefresh) > gc.refreshEvery
}

// LookupID resolves a genre name or slug, case-insensitively, to its TVDB id.
func (gc *Cache) LookupID(name string) (int, bool) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	id, ok := gc.nameToID[strings.ToLower(name)]
	return id, ok
}
