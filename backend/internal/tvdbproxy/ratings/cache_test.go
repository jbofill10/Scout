package ratings

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncCachePrunesExpiredEntriesOnceLarge(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	c := newAsyncCache[int](2, time.Second)
	c.now = func() time.Time { return now }

	c.mu.Lock()
	for i := 0; i < pruneAbove; i++ {
		c.entries["old"+strconv.Itoa(i)] = cacheEntry[int]{expires: now.Add(-time.Second)}
	}
	c.entries["live"] = cacheEntry[int]{value: 1, expires: now.Add(time.Hour)}
	c.mu.Unlock()

	got, ok := c.get(context.Background(), "new", func(context.Context) (int, time.Duration) { return 2, time.Hour })

	assert.True(t, ok)
	assert.Equal(t, 2, got)
	assert.Equal(t, 2, c.size(), "expired entries are swept once the cache is large")
	live, ok := c.peek("live")
	assert.True(t, ok)
	assert.Equal(t, 1, live)
}

func TestAsyncCacheDoesNotKeepUnkeptAnswers(t *testing.T) {
	c := newAsyncCache[int](2, time.Second)
	calls := 0
	fetch := func(context.Context) (int, time.Duration) {
		calls++
		return 7, 0
	}

	_, ok := c.get(context.Background(), "k", fetch)
	assert.False(t, ok)
	_, ok = c.get(context.Background(), "k", fetch)
	assert.False(t, ok)
	assert.Equal(t, 2, calls, "a ttl of zero means ask again next time")
	_, ok = c.peek("k")
	assert.False(t, ok)
}

// stubStore is an in-memory Store that counts calls and can fail.
type stubStore struct {
	mu         sync.Mutex
	entries    map[string]Entry
	err        error
	gets, puts int
}

func newStubStore() *stubStore {
	return &stubStore{entries: map[string]Entry{}}
}

func (s *stubStore) Get(_ context.Context, keys []string) (map[string]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets++
	if s.err != nil {
		return nil, s.err
	}
	out := map[string]Entry{}
	for _, key := range keys {
		if entry, ok := s.entries[key]; ok {
			out[key] = entry
		}
	}
	return out, nil
}

func (s *stubStore) Put(_ context.Context, entries map[string]Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts++
	if s.err != nil {
		return s.err
	}
	for key, entry := range entries {
		s.entries[key] = entry
	}
	return nil
}

// seed stores value as JSON under key.
func (s *stubStore) seed(t *testing.T, key string, value any, expires time.Time) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = Entry{Value: raw, Expires: expires}
}

func (s *stubStore) entry(key string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	return entry, ok
}

func (s *stubStore) count(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch key {
	case "gets":
		return s.gets
	default:
		return s.puts
	}
}

func newPersistedCache(store Store, now time.Time) *asyncCache[int] {
	c := newAsyncCache[int](2, time.Second)
	c.now = func() time.Time { return now }
	c.persist = &persistence{store: store, prefix: "n:", logger: slog.Default()}
	return c
}

func TestAsyncCacheServesStoredAnswerWithoutFetching(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	store := newStubStore()
	store.seed(t, "n:k", 5, now.Add(time.Hour))
	c := newPersistedCache(store, now)
	fetched := false
	fetch := func(context.Context) (int, time.Duration) { fetched = true; return 9, time.Hour }

	got, ok := c.get(context.Background(), "k", fetch)
	assert.True(t, ok)
	assert.Equal(t, 5, got, "the stored answer is current")
	assert.False(t, fetched)

	got, ok = c.get(context.Background(), "k", fetch)
	assert.True(t, ok)
	assert.Equal(t, 5, got)
	assert.Equal(t, 1, store.count("gets"), "a restored answer is held in memory")
}

func TestAsyncCachePersistsFetchedAnswers(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	store := newStubStore()
	c := newPersistedCache(store, now)

	got, ok := c.get(context.Background(), "k", func(context.Context) (int, time.Duration) { return 7, time.Hour })
	require.True(t, ok)
	assert.Equal(t, 7, got)

	entry, found := store.entry("n:k")
	require.True(t, found, "the answer is written under the cache's prefix")
	assert.JSONEq(t, "7", string(entry.Value))
	assert.Equal(t, now.Add(time.Hour), entry.Expires)
	_, found = store.entry("k")
	assert.False(t, found)
}

func TestAsyncCacheDoesNotPersistUnkeptAnswers(t *testing.T) {
	store := newStubStore()
	c := newPersistedCache(store, time.Now())

	_, ok := c.get(context.Background(), "k", func(context.Context) (int, time.Duration) { return 7, 0 })

	assert.False(t, ok)
	assert.Equal(t, 0, store.count("puts"))
}

func TestAsyncCacheStandsInWithExpiredStoredAnswerWhileLookupRuns(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	store := newStubStore()
	store.seed(t, "n:k", 5, now.Add(-time.Minute))
	c := newPersistedCache(store, now)
	gate := make(chan struct{})
	fetch := func(ctx context.Context) (int, time.Duration) {
		select {
		case <-gate:
			return 9, time.Hour
		case <-ctx.Done():
			return 0, 0
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	got, ok := c.get(ctx, "k", fetch)
	assert.True(t, ok)
	assert.Equal(t, 5, got, "the expired answer stands in until the lookup lands")

	close(gate)
	assert.Eventually(t, func() bool {
		entry, found := store.entry("n:k")
		return found && string(entry.Value) == "9"
	}, time.Second, 10*time.Millisecond, "the fresh answer replaces it in the store")
	v, live := c.peek("k")
	assert.True(t, live, "and in memory")
	assert.Equal(t, 9, v)
}

func TestAsyncCacheStandsInWithExpiredAnswerWhenLookupFails(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	c := newAsyncCache[int](2, time.Second)
	c.now = func() time.Time { return now }
	c.mu.Lock()
	c.entries["k"] = cacheEntry[int]{value: 5, expires: now.Add(-time.Minute)}
	c.mu.Unlock()

	got, ok := c.get(context.Background(), "k", func(context.Context) (int, time.Duration) { return 0, 0 })

	assert.True(t, ok)
	assert.Equal(t, 5, got)
	_, live := c.peek("k")
	assert.False(t, live, "a stand-in is not a live entry")
}

func TestAsyncCacheCarriesOnWhenTheStoreFails(t *testing.T) {
	store := newStubStore()
	store.err = errors.New("connection refused")
	c := newPersistedCache(store, time.Now())

	got, ok := c.get(context.Background(), "k", func(context.Context) (int, time.Duration) { return 7, time.Hour })

	assert.True(t, ok)
	assert.Equal(t, 7, got)
	v, live := c.peek("k")
	assert.True(t, live)
	assert.Equal(t, 7, v)
}

func TestAsyncCacheIgnoresUndecodableStoredAnswers(t *testing.T) {
	store := newStubStore()
	store.mu.Lock()
	store.entries["n:k"] = Entry{Value: json.RawMessage(`"not a number"`), Expires: time.Now().Add(time.Hour)}
	store.mu.Unlock()
	c := newPersistedCache(store, time.Now())

	got, ok := c.get(context.Background(), "k", func(context.Context) (int, time.Duration) { return 7, time.Hour })

	assert.True(t, ok)
	assert.Equal(t, 7, got, "an unreadable entry is looked up afresh")
}

func TestAsyncCachePrefixesKeepStoredAnswersApart(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	store := newStubStore()
	a := newPersistedCache(store, now)
	b := newPersistedCache(store, now)
	b.persist = &persistence{store: store, prefix: "other:", logger: slog.Default()}

	got, ok := a.get(context.Background(), "k", func(context.Context) (int, time.Duration) { return 1, time.Hour })
	require.True(t, ok)
	assert.Equal(t, 1, got)

	got, ok = b.get(context.Background(), "k", func(context.Context) (int, time.Duration) { return 2, time.Hour })
	require.True(t, ok)
	assert.Equal(t, 2, got, "the other cache's answer for the same key is not read")
	_, found := store.entry("n:k")
	assert.True(t, found)
	_, found = store.entry("other:k")
	assert.True(t, found)
}
