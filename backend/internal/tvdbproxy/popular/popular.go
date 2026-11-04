// Package popular ranks the shows and movies behind the Home, Shows and
// Movies rows.
//
// TVDB's own "score" is an all-time total, so sorting by it hands back
// Titanic, Avengers: Endgame and Frozen forever. This package ranks by what
// people are watching now instead: the primary source is Trakt's most
// watched (or trending) list, resolved to TVDB records so the rest of Scout
// keeps working on TVDB ids. Without a Trakt client id, or when Trakt is
// down, a TVDB-only source ranks recent releases by score weighted towards
// new and currently airing titles.
package popular

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// Kind is the media type of a row.
type Kind string

const (
	KindSeries Kind = "series"
	KindMovie  Kind = "movie"
)

const (
	// DefaultLimit is the row length when a caller gives none.
	DefaultLimit = 20
	// MaxLimit caps a row.
	MaxLimit = 50

	// listTTL is how long a ranked row is served before it is rebuilt.
	listTTL = time.Hour
	// recordTTL is how long a resolved TVDB record is kept; posters and
	// names rarely change.
	recordTTL = 24 * time.Hour
	// missTTL is how long a failed TVDB lookup is remembered, so one title
	// TVDB does not know is not asked for on every refresh.
	missTTL = time.Hour
	// tvdbConcurrency bounds the TVDB lookups a row's resolution runs at once.
	tvdbConcurrency = 6
	// buildTimeout bounds one row build. A build runs detached from the
	// request that started it, so a caller giving up (the webserver's 30s
	// client timeout) does not abort work other callers are waiting for and
	// does not leave a half-ranked row in the cache.
	buildTimeout = 3 * time.Minute
)

// ErrUnknownGenre is returned when a genre name is not in TVDB's list.
var ErrUnknownGenre = errors.New("unknown genre")

// Source ranks media of one kind for a genre; "" means every genre.
type Source interface {
	// Name labels the source in logs.
	Name() string
	Rank(ctx context.Context, kind Kind, genre string, limit int) ([]tvdb.Media, error)
}

// TVDBGetter performs an authenticated TVDB GET of a path relative to the TVDB host.
type TVDBGetter interface {
	Get(ctx context.Context, uri string) ([]byte, error)
}

// GenreLookup resolves a genre name to its TVDB id.
type GenreLookup interface {
	LookupID(name string) (int, bool)
}

// Service serves ranked rows from a cache. A miss is filled from the primary
// source; when there is none, or it fails or returns nothing, the fallback
// source answers instead. Concurrent misses on one row share a single build.
type Service struct {
	primary  Source
	fallback Source
	lists    *ttlCache[[]tvdb.Media]
	logger   *slog.Logger

	mu       sync.Mutex
	inflight map[string]*build
}

// build is one row being ranked, shared by every caller waiting for it.
type build struct {
	done    chan struct{}
	results []tvdb.Media
	err     error
}

// NewService returns a service ranking with primary first and fallback
// second. primary may be nil.
func NewService(primary, fallback Source, logger *slog.Logger) *Service {
	return &Service{
		primary:  primary,
		fallback: fallback,
		lists:    newTTLCache[[]tvdb.Media](),
		logger:   logger,
		inflight: map[string]*build{},
	}
}

// Rank returns up to limit items for the row, most popular first.
func (s *Service) Rank(ctx context.Context, kind Kind, genre string, limit int) ([]tvdb.Media, error) {
	limit = clampLimit(limit)
	key := fmt.Sprintf("%s|%s|%d", kind, strings.ToLower(genre), limit)
	if hit, ok := s.lists.get(key); ok {
		return cloneMedia(hit), nil
	}

	s.mu.Lock()
	b, ok := s.inflight[key]
	if !ok {
		b = &build{done: make(chan struct{})}
		s.inflight[key] = b
		go s.run(ctx, key, b, kind, genre, limit)
	}
	s.mu.Unlock()

	select {
	case <-b.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if b.err != nil {
		return nil, b.err
	}
	return cloneMedia(b.results), nil
}

// run builds one row under its own context (keeping the caller's trace
// values but not its cancellation), caches the result and releases every
// caller waiting on b.
func (s *Service) run(ctx context.Context, key string, b *build, kind Kind, genre string, limit int) {
	buildCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), buildTimeout)
	defer cancel()

	results, source, err := s.fetch(buildCtx, kind, genre, limit)
	switch {
	case err != nil:
	case buildCtx.Err() != nil:
		// The build ran out of time: serve what there is, but let the next
		// request build the row again rather than caching a partial ranking.
		s.logger.WarnContext(buildCtx, "Popular row build timed out, not cached", telemetry.WithTraceContext(buildCtx,
			"kind", string(kind), "genre", genre, "source", source)...)
	default:
		s.lists.set(key, results, listTTL)
		s.logger.InfoContext(buildCtx, "Ranked popular row", telemetry.WithTraceContext(buildCtx,
			"kind", string(kind), "genre", genre, "limit", limit, "count", len(results), "source", source)...)
	}
	b.results, b.err = results, err
	s.mu.Lock()
	delete(s.inflight, key)
	s.mu.Unlock()
	close(b.done)
}

// Row names one popular row.
type Row struct {
	Kind  Kind
	Genre string
	Limit int
}

// DefaultRows are the rows the UI shows: the unfiltered Home rows and the
// eight curated genres on the Shows and Movies pages.
func DefaultRows() []Row {
	var rows []Row
	for _, kind := range []Kind{KindSeries, KindMovie} {
		for _, genre := range []string{
			"", "Action", "Comedy", "Drama", "Sci-Fi", "Horror", "Thriller", "Anime", "Documentary",
		} {
			rows = append(rows, Row{Kind: kind, Genre: genre, Limit: DefaultLimit})
		}
	}
	return rows
}

// warmConcurrency is how many rows Warm builds at once.
const warmConcurrency = 3

// Warm builds rows in the background so the first requests after startup are
// served from the cache. It returns at once; the work stops when ctx ends.
func (s *Service) Warm(ctx context.Context, rows []Row) {
	go func() {
		sem := make(chan struct{}, warmConcurrency)
		var wg sync.WaitGroup
		for _, row := range rows {
			wg.Add(1)
			go func(row Row) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()
				if _, err := s.Rank(ctx, row.Kind, row.Genre, row.Limit); err != nil {
					s.logger.WarnContext(ctx, "Warming popular row failed",
						"kind", string(row.Kind), "genre", row.Genre, "error", err)
				}
			}(row)
		}
		wg.Wait()
		s.logger.InfoContext(ctx, "Popular rows warmed", "rows", len(rows))
	}()
}

func (s *Service) fetch(ctx context.Context, kind Kind, genre string, limit int) ([]tvdb.Media, string, error) {
	if s.primary != nil {
		results, err := s.primary.Rank(ctx, kind, genre, limit)
		switch {
		case err != nil:
			s.logger.WarnContext(ctx, "Primary popularity source failed, using fallback",
				telemetry.WithTraceContext(ctx, "source", s.primary.Name(), "kind", string(kind), "genre", genre,
					"error", err)...)
		case len(results) == 0:
			s.logger.WarnContext(ctx, "Primary popularity source returned nothing, using fallback",
				telemetry.WithTraceContext(ctx, "source", s.primary.Name(), "kind", string(kind), "genre", genre)...)
		default:
			return results, s.primary.Name(), nil
		}
	}
	results, err := s.fallback.Rank(ctx, kind, genre, limit)
	if err != nil {
		return nil, s.fallback.Name(), err
	}
	return results, s.fallback.Name(), nil
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

func cloneMedia(in []tvdb.Media) []tvdb.Media {
	out := make([]tvdb.Media, len(in))
	copy(out, in)
	return out
}

// artworkURL makes a TVDB artwork path absolute; absolute URLs pass through.
func artworkURL(path string) string {
	if path == "" || strings.HasPrefix(path, "http") {
		return path
	}
	return "https://artworks.thetvdb.com" + path
}

// ttlCache is a small in-memory cache with per-entry expiry.
type ttlCache[V any] struct {
	mu      sync.Mutex
	entries map[string]ttlEntry[V]
	now     func() time.Time
}

type ttlEntry[V any] struct {
	value   V
	expires time.Time
}

// pruneAbove is the entry count past which a set sweeps expired entries.
const pruneAbove = 5000

func newTTLCache[V any]() *ttlCache[V] {
	return &ttlCache[V]{entries: map[string]ttlEntry[V]{}, now: time.Now}
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || c.now().After(entry.expires) {
		var zero V
		return zero, false
	}
	return entry.value, true
}

func (c *ttlCache[V]) set(key string, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.entries) >= pruneAbove {
		for k, e := range c.entries {
			if now.After(e.expires) {
				delete(c.entries, k)
			}
		}
	}
	c.entries[key] = ttlEntry[V]{value: value, expires: now.Add(ttl)}
}
