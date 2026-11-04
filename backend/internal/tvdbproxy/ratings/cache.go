package ratings

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

const (
	// pruneAbove is the entry count past which a set sweeps expired entries.
	pruneAbove = 5000
	// storeTimeout bounds one read or write of the store.
	storeTimeout = 5 * time.Second
)

// persistence keeps one cache's answers in a Store under a key prefix, so
// they outlive the process. Store failures are logged and the cache carries
// on in memory. A nil persistence does nothing.
type persistence struct {
	store  Store
	prefix string
	logger *slog.Logger
}

// answer is one value to save with its expiry.
type answer struct {
	value   any
	expires time.Time
}

// load returns the stored entries for the keys, given and returned without
// the prefix; nothing on failure.
func (p *persistence) load(ctx context.Context, keys []string) map[string]Entry {
	if p == nil || len(keys) == 0 {
		return nil
	}
	prefixed := make([]string, len(keys))
	for i, key := range keys {
		prefixed[i] = p.prefix + key
	}
	ctx, cancel := context.WithTimeout(ctx, storeTimeout)
	defer cancel()
	entries, err := p.store.Get(ctx, prefixed)
	if err != nil {
		p.logger.WarnContext(ctx, "Ratings store read failed",
			telemetry.WithTraceContext(ctx, "prefix", p.prefix, "count", len(keys), "error", err)...)
		return nil
	}
	out := make(map[string]Entry, len(entries))
	for key, entry := range entries {
		out[strings.TrimPrefix(key, p.prefix)] = entry
	}
	return out
}

// save writes the answers as JSON with their expiry, under its own timeout
// and detached from the caller's cancellation.
func (p *persistence) save(ctx context.Context, answers map[string]answer) {
	if p == nil || len(answers) == 0 {
		return
	}
	entries := make(map[string]Entry, len(answers))
	for key, a := range answers {
		value, err := json.Marshal(a.value)
		if err != nil {
			continue
		}
		entries[p.prefix+key] = Entry{Value: value, Expires: a.expires}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer cancel()
	if err := p.store.Put(ctx, entries); err != nil {
		p.logger.WarnContext(ctx, "Ratings store write failed",
			telemetry.WithTraceContext(ctx, "prefix", p.prefix, "count", len(entries), "error", err)...)
	}
}

// asyncCache memoises detached lookups by key. A caller waits for an answer
// only as long as its context allows; the lookup itself runs on under its
// own timeout and lands in the cache, so a caller giving up does not waste
// the call and concurrent callers for one key share it. With a persistence
// the answers also land in the store and are read back from it after a
// restart.
type asyncCache[V any] struct {
	sem     chan struct{}
	timeout time.Duration
	now     func() time.Time
	persist *persistence

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

// get returns the current cached value for key, or runs fetch (once,
// however many callers ask) and waits for it until ctx ends. fetch reports
// the value and how long to keep it; a ttl of zero or less means the answer
// is not kept and the next caller asks again. An answer that has expired,
// in memory or in the store, stands in when no fresh one arrives in time:
// a week-old score beats none. ok is false when nothing is known by the
// time ctx ends.
func (c *asyncCache[V]) get(
	ctx context.Context, key string, fetch func(ctx context.Context) (V, time.Duration),
) (V, bool) {
	known, current, haveKnown := c.restore(ctx, key)
	if current {
		return known, true
	}

	c.mu.Lock()
	l, ok := c.inflight[key]
	if !ok {
		l = &inflight[V]{done: make(chan struct{})}
		c.inflight[key] = l
		go c.run(context.WithoutCancel(ctx), key, l, fetch)
	}
	c.mu.Unlock()

	select {
	case <-l.done:
		if l.ok {
			return l.value, true
		}
	case <-ctx.Done():
	}
	return known, haveKnown
}

// restore returns what the cache knows for key: the entry in memory, or
// the stored one brought into memory (expired or not, so the next asker
// finds it without a read). current says whether it is still live; known
// whether there is one at all.
func (c *asyncCache[V]) restore(ctx context.Context, key string) (value V, current, known bool) {
	c.mu.Lock()
	e, held := c.entries[key]
	c.mu.Unlock()
	if held {
		return e.value, c.now().Before(e.expires), true
	}
	entry, found := c.persist.load(ctx, []string{key})[key]
	if !found {
		return value, false, false
	}
	if err := json.Unmarshal(entry.Value, &value); err != nil {
		var zero V
		return zero, false, false
	}
	c.mu.Lock()
	c.set(key, value, entry.Expires.Sub(c.now()))
	c.mu.Unlock()
	return value, c.now().Before(entry.Expires), true
}

// peek reports whether key holds a live entry in memory, without fetching.
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
	c.persist.save(ctx, map[string]answer{key: {value: value, expires: c.now().Add(ttl)}})
}

// set stores one answer; the caller holds c.mu. A ttl of zero or less
// stores it already expired, as a stand-in.
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
