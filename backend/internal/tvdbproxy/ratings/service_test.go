package ratings

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSource answers from a table and counts calls per id. Lookups block
// while gate is held, so tests can leave one in flight.
type stubSource struct {
	mu      sync.Mutex
	answers map[string][]tvdb.Rating
	err     error
	calls   map[string]int
	gate    sync.RWMutex
}

func newStubSource() *stubSource {
	return &stubSource{answers: map[string][]tvdb.Rating{}, calls: map[string]int{}}
}

func (s *stubSource) Lookup(_ context.Context, imdbID string) ([]tvdb.Rating, error) {
	s.gate.RLock()
	defer s.gate.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[imdbID]++
	if s.err != nil {
		return nil, s.err
	}
	return s.answers[imdbID], nil
}

func (s *stubSource) count(imdbID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[imdbID]
}

var imdbEight = []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8}}

func newTestService(source Source) (*Service, *time.Time) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	s := NewService(source, slog.Default())
	s.now = func() time.Time { return now }
	return s, &now
}

func TestAttachFillsRatingsOncePerTitle(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, _ := newTestService(source)

	media := []tvdb.Media{
		{Id: "a", ImdbID: "tt1"},
		{Id: "b"},
		{Id: "c", ImdbID: "tt1"},
		{Id: "d", ImdbID: "tt2"},
		{Id: "e", ImdbID: "tt3", Ratings: []tvdb.Rating{{Source: "x", Value: 1}}},
	}
	s.Attach(context.Background(), media)

	assert.Equal(t, imdbEight, media[0].Ratings)
	assert.Nil(t, media[1].Ratings, "no IMDb id, nothing to look up")
	assert.Equal(t, imdbEight, media[2].Ratings, "the second item with the same id shares the answer")
	assert.Nil(t, media[3].Ratings, "the source has nothing for tt2")
	assert.Equal(t, []tvdb.Rating{{Source: "x", Value: 1}}, media[4].Ratings, "already rated items are left alone")
	assert.Equal(t, 1, source.count("tt1"))
	assert.Equal(t, 1, source.count("tt2"))
	assert.Equal(t, 0, source.count("tt3"))
}

func TestLookupCachesHitsAndMisses(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, now := newTestService(source)

	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt1"))
	assert.Nil(t, s.Lookup(context.Background(), "tt2"))
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt1"))
	assert.Nil(t, s.Lookup(context.Background(), "tt2"))
	assert.Equal(t, 1, source.count("tt1"), "a hit is cached")
	assert.Equal(t, 1, source.count("tt2"), "a miss is cached too")

	*now = now.Add(missTTL + time.Second)
	assert.Nil(t, s.Lookup(context.Background(), "tt2"))
	assert.Equal(t, 2, source.count("tt2"), "a miss expires after missTTL")
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt1"))
	assert.Equal(t, 1, source.count("tt1"), "a hit outlives missTTL")

	*now = now.Add(ratingTTL)
	s.Lookup(context.Background(), "tt1")
	assert.Equal(t, 2, source.count("tt1"), "a hit expires after ratingTTL")
}

func TestLookupEmptyIDAsksNothing(t *testing.T) {
	source := newStubSource()
	s, _ := newTestService(source)

	assert.Nil(t, s.Lookup(context.Background(), ""))
	assert.Empty(t, source.calls)
}

func TestLookupPausesWhenTheKeyIsRefused(t *testing.T) {
	source := newStubSource()
	source.err = ErrRateLimited
	s, now := newTestService(source)

	assert.Nil(t, s.Lookup(context.Background(), "tt1"))
	assert.Nil(t, s.Lookup(context.Background(), "tt2"))
	assert.Equal(t, 1, source.count("tt1"))
	assert.Equal(t, 0, source.count("tt2"), "no lookups while paused")

	source.err = nil
	source.answers["tt2"] = imdbEight
	*now = now.Add(pauseFor + time.Second)
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt2"), "lookups resume after the pause")
}

func TestLookupRetriesAfterASourceError(t *testing.T) {
	source := newStubSource()
	source.err = errors.New("connection reset")
	s, _ := newTestService(source)

	assert.Nil(t, s.Lookup(context.Background(), "tt1"))
	source.err = nil
	source.answers["tt1"] = imdbEight
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt1"))
	assert.Equal(t, 2, source.count("tt1"), "a failed call is not cached")
}

func TestAttachGivesUpAfterTheBudgetAndCachesLater(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, _ := newTestService(source)
	s.budget = 30 * time.Millisecond

	source.gate.Lock() // the lookup blocks until released
	media := []tvdb.Media{{Id: "a", ImdbID: "tt1"}}
	started := time.Now()
	s.Attach(context.Background(), media)
	assert.Less(t, time.Since(started), time.Second)
	assert.Nil(t, media[0].Ratings, "the slow answer is left off")

	source.gate.Unlock()
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, ok := s.cache["tt1"]
		return ok
	}, time.Second, 5*time.Millisecond, "the detached lookup still lands in the cache")

	s.Attach(context.Background(), media)
	assert.Equal(t, imdbEight, media[0].Ratings)
	assert.Equal(t, 1, source.count("tt1"))
}

func TestConcurrentLookupsShareOneCall(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, _ := newTestService(source)

	source.gate.Lock()
	var wg sync.WaitGroup
	results := make([][]tvdb.Rating, 5)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = s.Lookup(context.Background(), "tt1")
		}(i)
	}
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.inflight) == 1
	}, time.Second, time.Millisecond)
	source.gate.Unlock()
	wg.Wait()

	for _, got := range results {
		assert.Equal(t, imdbEight, got)
	}
	assert.Equal(t, 1, source.count("tt1"))
}

func TestCachePrunesExpiredEntriesOnceLarge(t *testing.T) {
	source := newStubSource()
	source.answers["ttnew"] = imdbEight
	s, now := newTestService(source)

	s.mu.Lock()
	for i := 0; i < pruneAbove; i++ {
		s.cache["old"+strconv.Itoa(i)] = entry{expires: now.Add(-time.Second)}
	}
	s.cache["live"] = entry{ratings: imdbEight, expires: now.Add(time.Hour)}
	s.mu.Unlock()

	s.Lookup(context.Background(), "ttnew")

	s.mu.Lock()
	defer s.mu.Unlock()
	assert.Len(t, s.cache, 2, "expired entries are swept once the cache is large")
	assert.Contains(t, s.cache, "live")
	assert.Contains(t, s.cache, "ttnew")
}
