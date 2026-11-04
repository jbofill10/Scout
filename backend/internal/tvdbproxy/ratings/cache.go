package ratings

import (
	"context"
	"sync"
	"time"
)

// pruneAbove is the entry count past which a set sweeps expired entries.
const pruneAbove = 5000

// asyncCache memoises detached lookups by key. A caller waits for an answer
// only as long as its context allows; the lookup itself runs on under its
// own timeout and lands in the cache, so a caller giving up does not waste
// the call and concurrent callers for one key share it.
type asyncCache[V any] struct {
	sem     chan struct{}
	timeout time.Duration
	now     func() time.Time

	mu       sync.Mutex
	entries  map[string]cacheEntry[V]
	inflight map[string]*inflight[V]
}

type cacheEntry[V any] struct {
	value   V
	expires time.Time
}

type inflight[V any] struct {
	done  chan struct{}
	value V
	ok    bool
}

// newAsyncCache bounds lookups to concurrency at once and timeout each.
func newAsyncCache[V any](concurrency int, timeout time.Duration) *asyncCache[V] {
	return &asyncCache[V]{
		sem:      make(chan struct{}, concurrency),
		timeout:  timeout,
		now:      time.Now,
		entries:  map[string]cacheEntry[V]{},
		inflight: map[string]*inflight[V]{},
	}
}

// get returns the live cached value for key, or runs fetch (once, however
// many callers ask) and waits for it until ctx ends. fetch reports the value
// and how long to keep it; a ttl of zero or less means the answer is not
// kept and the next caller asks again. ok is false when nothing is known
// by the time ctx ends.
func (c *asyncCache[V]) get(
	ctx context.Context, key string, fetch func(ctx context.Context) (V, time.Duration),
) (V, bool) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok && c.now().Before(e.expires) {
		c.mu.Unlock()
		return e.value, true
	}
	l, ok := c.inflight[key]
	if !ok {
		l = &inflight[V]{done: make(chan struct{})}
		c.inflight[key] = l
		go c.run(context.WithoutCancel(ctx), key, l, fetch)
	}
	c.mu.Unlock()

	select {
	case <-l.done:
		return l.value, l.ok
	case <-ctx.Done():
		var zero V
		return zero, false
	}
}

// peek reports whether key holds a live entry, without fetching.
func (c *asyncCache[V]) peek(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !c.now().Before(e.expires) {
		var zero V
		return zero, false
	}
	return e.value, true
}

func (c *asyncCache[V]) run(ctx context.Context, key string, l *inflight[V], fetch func(context.Context) (V, time.Duration)) {
	defer func() {
		c.mu.Lock()
		delete(c.inflight, key)
		c.mu.Unlock()
		close(l.done)
	}()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-c.sem }()

	value, ttl := fetch(ctx)
	if ttl <= 0 {
		return
	}
	l.value, l.ok = value, true
	c.mu.Lock()
	c.set(key, value, ttl)
	c.mu.Unlock()
}

// set stores one answer; the caller holds c.mu.
func (c *asyncCache[V]) set(key string, value V, ttl time.Duration) {
	now := c.now()
	if len(c.entries) >= pruneAbove {
		for k, e := range c.entries {
			if now.After(e.expires) {
				delete(c.entries, k)
			}
		}
	}
	c.entries[key] = cacheEntry[V]{value: value, expires: now.Add(ttl)}
}

// size is the number of entries held, live or expired.
func (c *asyncCache[V]) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
