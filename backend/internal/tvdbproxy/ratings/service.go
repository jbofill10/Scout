// Package ratings attaches critic and audience scores to media: the
// Rotten Tomatoes Tomatometer, IMDb's rating and Metacritic's Metascore,
// looked up by IMDb id. TVDB carries none of these, so they come from a
// separate source (OMDb) with a small daily allowance; every answer, hit or
// miss, is cached for days and concurrent asks for one title share a lookup.
package ratings

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

const (
	// ratingTTL is how long a title's ratings are served before being
	// looked up again; scores move slowly once a title is out.
	ratingTTL = 7 * 24 * time.Hour
	// missTTL is how long "the source has no record" is remembered.
	missTTL = 24 * time.Hour
	// pauseFor is how long lookups stop after the source refuses the key,
	// whether its daily allowance is used up or it is wrong.
	pauseFor = time.Hour
	// lookupConcurrency bounds the source calls in flight at once.
	lookupConcurrency = 4
	// lookupTimeout bounds one source call. The call runs detached from the
	// request that asked, so a caller giving up does not waste the answer.
	lookupTimeout = 15 * time.Second
	// waitBudget is how long a caller waits for a lookup that is not cached
	// yet. Ratings are decoration: past the budget the response goes out
	// without them and the next request finds them cached.
	waitBudget = 4 * time.Second
	// pruneAbove is the cache size past which a set sweeps expired entries.
	pruneAbove = 5000
)

// Source looks up a title's ratings by IMDb id. nil, nil means the source
// has no record; ErrRateLimited and ErrUnauthorized mean the key is refused.
type Source interface {
	Lookup(ctx context.Context, imdbID string) ([]tvdb.Rating, error)
}

// Service answers rating lookups from a cache, filling misses from the
// source with bounded concurrency and a bounded wait.
type Service struct {
	source Source
	logger *slog.Logger
	sem    chan struct{}
	now    func() time.Time
	budget time.Duration

	mu          sync.Mutex
	cache       map[string]entry
	inflight    map[string]*lookup
	pausedUntil time.Time
}

type entry struct {
	ratings []tvdb.Rating
	expires time.Time
}

// lookup is one source call in progress, shared by every caller asking for
// the same title.
type lookup struct {
	done    chan struct{}
	ratings []tvdb.Rating
}

// NewService returns a service answering from source.
func NewService(source Source, logger *slog.Logger) *Service {
	return &Service{
		source:   source,
		logger:   logger,
		sem:      make(chan struct{}, lookupConcurrency),
		now:      time.Now,
		budget:   waitBudget,
		cache:    map[string]entry{},
		inflight: map[string]*lookup{},
	}
}

// Attach fills Ratings on every item that has an IMDb id and no ratings yet.
// Items sharing an id share one lookup. It returns once every lookup has
// answered or the wait budget is spent, whichever is first.
func (s *Service) Attach(ctx context.Context, media []tvdb.Media) {
	ctx, cancel := context.WithTimeout(ctx, s.budget)
	defer cancel()

	byID := map[string][]int{}
	for i, item := range media {
		if item.ImdbID == "" || len(item.Ratings) > 0 {
			continue
		}
		byID[item.ImdbID] = append(byID[item.ImdbID], i)
	}
	if len(byID) == 0 {
		return
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results = make(map[string][]tvdb.Rating, len(byID))
	)
	for id := range byID {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			ratings := s.Lookup(ctx, id)
			mu.Lock()
			results[id] = ratings
			mu.Unlock()
		}(id)
	}
	wg.Wait()

	for id, indexes := range byID {
		ratings := results[id]
		if len(ratings) == 0 {
			continue
		}
		for _, i := range indexes {
			media[i].Ratings = ratings
		}
	}
}

// Lookup returns the ratings for an IMDb id: from the cache, or from the
// source when the id is new, waiting at most the wait budget (or until ctx
// ends). nil means none are known yet, or the title has none.
func (s *Service) Lookup(ctx context.Context, imdbID string) []tvdb.Rating {
	if imdbID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.budget)
	defer cancel()

	s.mu.Lock()
	now := s.now()
	if hit, ok := s.cache[imdbID]; ok && now.Before(hit.expires) {
		s.mu.Unlock()
		return hit.ratings
	}
	if now.Before(s.pausedUntil) {
		s.mu.Unlock()
		return nil
	}
	l, ok := s.inflight[imdbID]
	if !ok {
		l = &lookup{done: make(chan struct{})}
		s.inflight[imdbID] = l
		go s.fetch(context.WithoutCancel(ctx), imdbID, l)
	}
	s.mu.Unlock()

	select {
	case <-l.done:
		return l.ratings
	case <-ctx.Done():
		return nil
	}
}

// fetch runs one source call under the concurrency bound, caches the answer
// and releases every caller waiting on l.
func (s *Service) fetch(ctx context.Context, imdbID string, l *lookup) {
	defer func() {
		s.mu.Lock()
		delete(s.inflight, imdbID)
		s.mu.Unlock()
		close(l.done)
	}()

	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-s.sem }()

	ratings, err := s.source.Lookup(ctx, imdbID)
	s.record(ctx, imdbID, l, ratings, err)
}

// record caches one source answer and hands it to l's waiters.
func (s *Service) record(ctx context.Context, imdbID string, l *lookup, ratings []tvdb.Rating, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	switch {
	case errors.Is(err, ErrRateLimited), errors.Is(err, ErrUnauthorized):
		if now.After(s.pausedUntil) {
			s.logger.WarnContext(ctx, "Rating source refused the key, pausing lookups",
				telemetry.WithTraceContext(ctx, "error", err, "pause", pauseFor.String())...)
		}
		s.pausedUntil = now.Add(pauseFor)
	case err != nil:
		// A failed call is simply retried by the next asker.
		s.logger.WarnContext(ctx, "Rating lookup failed",
			telemetry.WithTraceContext(ctx, "imdb_id", imdbID, "error", err)...)
	case len(ratings) == 0:
		s.set(now, imdbID, nil, missTTL)
	default:
		l.ratings = ratings
		s.set(now, imdbID, ratings, ratingTTL)
	}
}

// set stores one answer; the caller holds s.mu.
func (s *Service) set(now time.Time, imdbID string, ratings []tvdb.Rating, ttl time.Duration) {
	if len(s.cache) >= pruneAbove {
		for key, e := range s.cache {
			if now.After(e.expires) {
				delete(s.cache, key)
			}
		}
	}
	s.cache[imdbID] = entry{ratings: ratings, expires: now.Add(ttl)}
}
