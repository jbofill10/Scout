package popular

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSource answers Rank from canned results and counts its calls.
type stubSource struct {
	mu      sync.Mutex
	name    string
	results []tvdb.Media
	err     error
	calls   int
	limits  []int
}

func (s *stubSource) Name() string { return s.name }

func (s *stubSource) Rank(_ context.Context, _ Kind, _ string, limit int) ([]tvdb.Media, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.limits = append(s.limits, limit)
	return s.results, s.err
}

func media(ids ...string) []tvdb.Media {
	out := make([]tvdb.Media, len(ids))
	for i, id := range ids {
		out[i] = tvdb.Media{Id: id, Name: "title " + id}
	}
	return out
}

func TestServiceServesPrimaryAndCaches(t *testing.T) {
	primary := &stubSource{name: "primary", results: media("1", "2")}
	fallback := &stubSource{name: "fallback", results: media("9")}
	svc := NewService(primary, fallback, slog.Default())

	first, err := svc.Rank(context.Background(), KindSeries, "Drama", 20)
	require.NoError(t, err)
	assert.Equal(t, media("1", "2"), first)

	second, err := svc.Rank(context.Background(), KindSeries, "drama", 20)
	require.NoError(t, err)
	assert.Equal(t, first, second, "genre lookup is case-insensitive")
	assert.Equal(t, 1, primary.calls, "second call is served from the cache")
	assert.Equal(t, 0, fallback.calls)

	// A different limit is a different row.
	_, err = svc.Rank(context.Background(), KindSeries, "Drama", 5)
	require.NoError(t, err)
	assert.Equal(t, 2, primary.calls)
	assert.Equal(t, []int{20, 5}, primary.limits)
}

func TestServiceReturnsCopies(t *testing.T) {
	primary := &stubSource{name: "primary", results: media("1", "2")}
	svc := NewService(primary, &stubSource{name: "fallback"}, slog.Default())

	first, err := svc.Rank(context.Background(), KindMovie, "", 20)
	require.NoError(t, err)
	first[0].Name = "mutated"

	second, err := svc.Rank(context.Background(), KindMovie, "", 20)
	require.NoError(t, err)
	assert.Equal(t, "title 1", second[0].Name)
}

func TestServiceFallsBackWhenPrimaryFails(t *testing.T) {
	primary := &stubSource{name: "primary", err: errors.New("trakt down")}
	fallback := &stubSource{name: "fallback", results: media("9")}
	svc := NewService(primary, fallback, slog.Default())

	results, err := svc.Rank(context.Background(), KindSeries, "", 20)
	require.NoError(t, err)
	assert.Equal(t, media("9"), results)
	assert.Equal(t, 1, fallback.calls)
}

func TestServiceFallsBackWhenPrimaryIsEmpty(t *testing.T) {
	primary := &stubSource{name: "primary", results: []tvdb.Media{}}
	fallback := &stubSource{name: "fallback", results: media("9")}
	svc := NewService(primary, fallback, slog.Default())

	results, err := svc.Rank(context.Background(), KindSeries, "Anime", 20)
	require.NoError(t, err)
	assert.Equal(t, media("9"), results)
}

func TestServiceWithoutPrimaryUsesFallback(t *testing.T) {
	fallback := &stubSource{name: "fallback", results: media("9")}
	svc := NewService(nil, fallback, slog.Default())

	results, err := svc.Rank(context.Background(), KindMovie, "", 0)
	require.NoError(t, err)
	assert.Equal(t, media("9"), results)
	assert.Equal(t, []int{DefaultLimit}, fallback.limits, "a zero limit becomes the default")
}

func TestServiceReportsFallbackError(t *testing.T) {
	fallback := &stubSource{name: "fallback", err: errors.New("tvdb down")}
	svc := NewService(nil, fallback, slog.Default())

	_, err := svc.Rank(context.Background(), KindMovie, "", 20)
	require.Error(t, err)
}

func TestServiceClampsLimit(t *testing.T) {
	fallback := &stubSource{name: "fallback", results: media("9")}
	svc := NewService(nil, fallback, slog.Default())

	_, err := svc.Rank(context.Background(), KindMovie, "", 500)
	require.NoError(t, err)
	assert.Equal(t, []int{MaxLimit}, fallback.limits)
}

func TestTTLCacheExpires(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	cache := newTTLCache[string]()
	cache.now = func() time.Time { return now }

	cache.set("k", "v", time.Minute)
	got, ok := cache.get("k")
	require.True(t, ok)
	assert.Equal(t, "v", got)

	now = now.Add(2 * time.Minute)
	_, ok = cache.get("k")
	assert.False(t, ok)
}

func TestArtworkURL(t *testing.T) {
	assert.Equal(t, "", artworkURL(""))
	assert.Equal(t, "https://artworks.thetvdb.com/banners/x.jpg", artworkURL("/banners/x.jpg"))
	assert.Equal(t, "https://artworks.thetvdb.com/banners/x.jpg", artworkURL("https://artworks.thetvdb.com/banners/x.jpg"))
}

func TestTTLCachePrunesExpiredEntriesWhenLarge(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	cache := newTTLCache[int]()
	cache.now = func() time.Time { return now }

	for i := 0; i < pruneAbove; i++ {
		cache.set(fmt.Sprintf("old-%d", i), i, time.Minute)
	}
	now = now.Add(2 * time.Minute)
	cache.set("fresh", 1, time.Minute)

	cache.mu.Lock()
	size := len(cache.entries)
	cache.mu.Unlock()
	assert.Equal(t, 1, size, "a set past the threshold sweeps expired entries")
	got, ok := cache.get("fresh")
	require.True(t, ok)
	assert.Equal(t, 1, got)
}

// slowSource blocks each Rank until released and counts calls.
type slowSource struct {
	release chan struct{}
	calls   int32
}

func (s *slowSource) Name() string { return "slow" }

func (s *slowSource) Rank(ctx context.Context, _ Kind, _ string, _ int) ([]tvdb.Media, error) {
	atomic.AddInt32(&s.calls, 1)
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return media("1"), nil
}

func TestServiceSharesConcurrentBuilds(t *testing.T) {
	src := &slowSource{release: make(chan struct{})}
	svc := NewService(nil, src, slog.Default())

	var wg sync.WaitGroup
	results := make([][]tvdb.Media, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = svc.Rank(context.Background(), KindMovie, "", 20)
		}(i)
	}
	require.Eventually(t, func() bool { return atomic.LoadInt32(&src.calls) == 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	close(src.release)
	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&src.calls), "five concurrent misses build the row once")
	for _, r := range results {
		assert.Equal(t, media("1"), r)
	}
}

func TestServiceWarmBuildsDefaultRows(t *testing.T) {
	fallback := &stubSource{name: "fallback", results: media("9")}
	svc := NewService(nil, fallback, slog.Default())
	rows := DefaultRows()
	assert.Len(t, rows, 18)

	svc.Warm(context.Background(), rows)
	require.Eventually(t, func() bool {
		fallback.mu.Lock()
		defer fallback.mu.Unlock()
		return len(fallback.limits) == 18
	}, 2*time.Second, 5*time.Millisecond)

	// Every warmed row is now a cache hit.
	_, err := svc.Rank(context.Background(), KindSeries, "anime", DefaultLimit)
	require.NoError(t, err)
	fallback.mu.Lock()
	defer fallback.mu.Unlock()
	assert.Equal(t, 18, len(fallback.limits))
}

// A caller that gives up does not abort the build others wait for, and the
// finished build is cached for the next request.
func TestServiceBuildOutlivesCaller(t *testing.T) {
	src := &slowSource{release: make(chan struct{})}
	svc := NewService(nil, src, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() {
		_, err := svc.Rank(ctx, KindMovie, "", 20)
		errs <- err
	}()
	require.Eventually(t, func() bool { return atomic.LoadInt32(&src.calls) == 1 }, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-errs, context.Canceled, "the caller stops waiting")

	close(src.release)
	require.Eventually(t, func() bool {
		results, err := svc.Rank(context.Background(), KindMovie, "", 20)
		return err == nil && len(results) == 1
	}, time.Second, time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&src.calls), "the abandoned build finished and was cached")
}

type stubRowRater struct {
	mu    sync.Mutex
	calls int
	waits []time.Duration
}

func (r *stubRowRater) Attach(_ context.Context, media []tvdb.Media, wait time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.waits = append(r.waits, wait)
	for i := range media {
		media[i].Ratings = []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8}}
	}
}

func TestRankRatesRowsAsTheyAreBuilt(t *testing.T) {
	src := &stubSource{name: "stub", results: []tvdb.Media{{Id: "1"}, {Id: "2"}}}
	svc := NewService(nil, src, slog.Default())
	rater := &stubRowRater{}
	svc.SetRater(rater)

	first, err := svc.Rank(context.Background(), KindMovie, "", 5)
	require.NoError(t, err)
	second, err := svc.Rank(context.Background(), KindMovie, "", 5)
	require.NoError(t, err)

	assert.Equal(t, 1, rater.calls, "rated once per build, not per request")
	assert.Equal(t, []time.Duration{rowRatingWait}, rater.waits)
	for _, row := range [][]tvdb.Media{first, second} {
		require.Len(t, row, 2)
		assert.Equal(t, 8.0, row[0].Ratings[0].Value, "the cached row carries its scores")
	}
}
