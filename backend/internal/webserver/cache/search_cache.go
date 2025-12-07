package cache

import (
	"strings"
	"sync"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// SearchCache provides thread-safe in-memory caching for search results
type SearchCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

// cacheEntry represents a cached search result with timestamp
type cacheEntry struct {
	results   []tvdb.Media
	timestamp time.Time
}

// NewSearchCache creates a new search cache with the specified TTL
func NewSearchCache(ttl time.Duration) *SearchCache {
	return &SearchCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

// buildCacheKey creates a normalized cache key from query and media type
// Query is lowercased and trimmed for consistent lookups
func buildCacheKey(query, mediaType string) string {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	return normalizedQuery + ":" + mediaType
}

// Get retrieves cached search results if they exist and haven't expired
// Returns the results and true if found and valid, nil and false otherwise
func (c *SearchCache) Get(query, mediaType string) ([]tvdb.Media, bool) {
	key := buildCacheKey(query, mediaType)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// Check if entry has expired
	if time.Since(entry.timestamp) > c.ttl {
		// Clean up expired entry
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry.results, true
}

// Set stores search results in the cache with the current timestamp
func (c *SearchCache) Set(query, mediaType string, results []tvdb.Media) {
	key := buildCacheKey(query, mediaType)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		results:   results,
		timestamp: time.Now(),
	}
}

// Clear removes all entries from the cache
// Useful for testing or manual cache invalidation
func (c *SearchCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry)
}

// Size returns the number of entries currently in the cache
func (c *SearchCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

// CleanExpired removes all expired entries from the cache
// This can be called periodically to free memory from stale entries
func (c *SearchCache) CleanExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	cleaned := 0
	now := time.Now()

	for key, entry := range c.entries {
		if now.Sub(entry.timestamp) > c.ttl {
			delete(c.entries, key)
			cleaned++
		}
	}

	return cleaned
}
