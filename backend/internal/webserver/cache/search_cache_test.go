package cache

import (
	"testing"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

func TestNewSearchCache(t *testing.T) {
	ttl := 5 * time.Minute
	cache := NewSearchCache(ttl)

	if cache == nil {
		t.Fatal("NewSearchCache returned nil")
	}

	if cache.ttl != ttl {
		t.Errorf("Expected TTL %v, got %v", ttl, cache.ttl)
	}

	if cache.entries == nil {
		t.Error("Cache entries map not initialized")
	}

	if cache.Size() != 0 {
		t.Errorf("Expected empty cache, got size %d", cache.Size())
	}
}

func TestBuildCacheKey(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		mediaType string
		expected  string
	}{
		{
			name:      "simple query",
			query:     "batman",
			mediaType: "series",
			expected:  "batman:series",
		},
		{
			name:      "uppercase query normalized to lowercase",
			query:     "BATMAN",
			mediaType: "series",
			expected:  "batman:series",
		},
		{
			name:      "query with whitespace trimmed",
			query:     "  batman  ",
			mediaType: "series",
			expected:  "batman:series",
		},
		{
			name:      "mixed case with whitespace",
			query:     "  Breaking Bad  ",
			mediaType: "series",
			expected:  "breaking bad:series",
		},
		{
			name:      "movie media type",
			query:     "inception",
			mediaType: "movie",
			expected:  "inception:movie",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCacheKey(tt.query, tt.mediaType)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSearchCache_SetAndGet(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	// Create test data
	testMedia := []tvdb.Media{
		{
			Id:   "1",
			Name: "Batman",
		},
		{
			Id:   "2",
			Name: "Batman: The Animated Series",
		},
	}

	// Test Set
	cache.Set("batman", "series", testMedia)

	// Test Get - should find the results
	results, found := cache.Get("batman", "series")
	if !found {
		t.Fatal("Expected to find cached results")
	}

	if len(results) != len(testMedia) {
		t.Errorf("Expected %d results, got %d", len(testMedia), len(results))
	}

	if results[0].Id != "1" || results[0].Name != "Batman" {
		t.Errorf("Unexpected result data: %+v", results[0])
	}
}

func TestSearchCache_GetCaseSensitivity(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	testMedia := []tvdb.Media{
		{Id: "1", Name: "Batman"},
	}

	// Set with lowercase
	cache.Set("batman", "series", testMedia)

	// Get with uppercase - should still find it
	results, found := cache.Get("BATMAN", "series")
	if !found {
		t.Fatal("Expected to find cached results with uppercase query")
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	// Get with mixed case and whitespace
	results, found = cache.Get("  BaTmAn  ", "series")
	if !found {
		t.Fatal("Expected to find cached results with mixed case and whitespace")
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestSearchCache_GetMiss(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	// Try to get non-existent entry
	results, found := cache.Get("nonexistent", "series")
	if found {
		t.Error("Expected cache miss for non-existent key")
	}

	if results != nil {
		t.Errorf("Expected nil results on cache miss, got %+v", results)
	}
}

func TestSearchCache_GetDifferentMediaTypes(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	seriesMedia := []tvdb.Media{
		{Id: "1", Name: "Batman Series"},
	}

	movieMedia := []tvdb.Media{
		{Id: "2", Name: "Batman Movie"},
	}

	// Set same query with different media types
	cache.Set("batman", "series", seriesMedia)
	cache.Set("batman", "movie", movieMedia)

	// Get series
	results, found := cache.Get("batman", "series")
	if !found {
		t.Fatal("Expected to find cached series results")
	}
	if results[0].Id != "1" {
		t.Errorf("Expected series media, got %+v", results[0])
	}

	// Get movie
	results, found = cache.Get("batman", "movie")
	if !found {
		t.Fatal("Expected to find cached movie results")
	}
	if results[0].Id != "2" {
		t.Errorf("Expected movie media, got %+v", results[0])
	}
}

func TestSearchCache_Expiration(t *testing.T) {
	// Create cache with very short TTL for testing
	cache := NewSearchCache(100 * time.Millisecond)

	testMedia := []tvdb.Media{
		{Id: "1", Name: "Batman"},
	}

	// Set cache entry
	cache.Set("batman", "series", testMedia)

	// Immediate get should succeed
	results, found := cache.Get("batman", "series")
	if !found {
		t.Fatal("Expected to find cached results immediately")
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Get should now fail due to expiration
	results, found = cache.Get("batman", "series")
	if found {
		t.Error("Expected cache miss after expiration")
	}
	if results != nil {
		t.Errorf("Expected nil results after expiration, got %+v", results)
	}

	// Verify entry was cleaned up
	if cache.Size() != 0 {
		t.Errorf("Expected cache to be cleaned up, but size is %d", cache.Size())
	}
}

func TestSearchCache_Clear(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	// Add multiple entries
	cache.Set("batman", "series", []tvdb.Media{{Id: "1", Name: "Batman"}})
	cache.Set("superman", "series", []tvdb.Media{{Id: "2", Name: "Superman"}})
	cache.Set("inception", "movie", []tvdb.Media{{Id: "3", Name: "Inception"}})

	// Verify entries exist
	if cache.Size() != 3 {
		t.Errorf("Expected 3 entries, got %d", cache.Size())
	}

	// Clear cache
	cache.Clear()

	// Verify all entries removed
	if cache.Size() != 0 {
		t.Errorf("Expected empty cache after clear, got size %d", cache.Size())
	}

	// Verify no entries can be retrieved
	_, found := cache.Get("batman", "series")
	if found {
		t.Error("Expected cache miss after clear")
	}
}

func TestSearchCache_Size(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	// Empty cache
	if cache.Size() != 0 {
		t.Errorf("Expected size 0, got %d", cache.Size())
	}

	// Add entries
	cache.Set("batman", "series", []tvdb.Media{{Id: "1"}})
	if cache.Size() != 1 {
		t.Errorf("Expected size 1, got %d", cache.Size())
	}

	cache.Set("superman", "series", []tvdb.Media{{Id: "2"}})
	if cache.Size() != 2 {
		t.Errorf("Expected size 2, got %d", cache.Size())
	}

	// Overwrite existing entry (shouldn't increase size)
	cache.Set("batman", "series", []tvdb.Media{{Id: "1"}, {Id: "3"}})
	if cache.Size() != 2 {
		t.Errorf("Expected size 2 after overwrite, got %d", cache.Size())
	}
}

func TestSearchCache_CleanExpired(t *testing.T) {
	// Create cache with short TTL
	cache := NewSearchCache(100 * time.Millisecond)

	// Add multiple entries
	cache.Set("batman", "series", []tvdb.Media{{Id: "1"}})
	cache.Set("superman", "series", []tvdb.Media{{Id: "2"}})

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Add fresh entry
	cache.Set("spiderman", "series", []tvdb.Media{{Id: "3"}})

	// Clean expired entries
	cleaned := cache.CleanExpired()

	// Should have cleaned 2 expired entries
	if cleaned != 2 {
		t.Errorf("Expected 2 entries cleaned, got %d", cleaned)
	}

	// Should have 1 entry remaining
	if cache.Size() != 1 {
		t.Errorf("Expected 1 entry remaining, got %d", cache.Size())
	}

	// Fresh entry should still be retrievable
	results, found := cache.Get("spiderman", "series")
	if !found {
		t.Fatal("Expected to find fresh entry after cleanup")
	}
	if len(results) != 1 || results[0].Id != "3" {
		t.Errorf("Unexpected results: %+v", results)
	}
}

func TestSearchCache_Concurrency(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	// Run concurrent operations
	done := make(chan bool)
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Set
			cache.Set("test", "series", []tvdb.Media{{Id: string(rune(id))}})

			// Get
			cache.Get("test", "series")

			// Size
			cache.Size()

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify cache is in valid state
	if cache.Size() < 1 {
		t.Error("Expected at least one entry after concurrent operations")
	}
}

func TestSearchCache_EmptyResults(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	// Set empty results slice
	emptyResults := []tvdb.Media{}
	cache.Set("notfound", "series", emptyResults)

	// Should still be cached
	results, found := cache.Get("notfound", "series")
	if !found {
		t.Fatal("Expected to find cached empty results")
	}

	if results == nil {
		t.Error("Expected non-nil empty slice")
	}

	if len(results) != 0 {
		t.Errorf("Expected empty slice, got %d items", len(results))
	}
}
