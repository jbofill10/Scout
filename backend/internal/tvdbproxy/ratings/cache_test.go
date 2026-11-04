package ratings

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
