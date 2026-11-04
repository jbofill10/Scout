// Package ratings attaches critic and audience scores to media: IMDb's
// rating from IMDb's public daily dataset (no key needed), and, when an
// OMDb key is configured, the Rotten Tomatoes Tomatometer and Metacritic's
// Metascore. Both are keyed by IMDb id; media that only carries a TVDB id
// (library items, person-credit search hits) is resolved to one first.
// OMDb answers, hit or miss, are cached for days and concurrent asks for one
// title share a lookup, since a free key allows 1,000 calls a day.
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
	// ratingTTL is how long a title's OMDb ratings are served before being
	// looked up again; scores move slowly once a title is out.
	ratingTTL = 7 * 24 * time.Hour
	// missTTL is how long "OMDb has no record" is remembered.
	missTTL = 24 * time.Hour
	// pauseFor is how long OMDb lookups stop after it refuses the key,
	// whether its daily allowance is used up or it is wrong.
	pauseFor = time.Hour
	// lookupConcurrency bounds the OMDb calls in flight at once.
	lookupConcurrency = 4
	// lookupTimeout bounds one OMDb call. The call runs detached from the
	// request that asked, so a caller giving up does not waste the answer.
	lookupTimeout = 15 * time.Second
	// waitBudget is how long a caller waits for an answer not cached yet.
	// Ratings are decoration: past the budget the response goes out
	// without them and the next request finds them cached.
	waitBudget = 4 * time.Second
)

// Source looks up a title's ratings by IMDb id. nil, nil means the source
// has no record; ErrRateLimited and ErrUnauthorized mean the key is refused.
type Source interface {
	Lookup(ctx context.Context, imdbID string) ([]tvdb.Rating, error)
}

// IMDbTable answers IMDb ratings from a local table; Dataset implements it.
type IMDbTable interface {
	Rating(imdbID string) (tvdb.Rating, bool)
}

// IDResolver finds the IMDb id behind a TVDB id; Resolver implements it.
type IDResolver interface {
	ImdbID(ctx context.Context, mediaType, id string) string
}

// Options are the parts a Service draws on; any may be nil.
type Options struct {
	// IMDb answers the IMDb rating locally.
	IMDb IMDbTable
	// Source is the keyed source (OMDb) for the Tomatometer and Metascore.
	Source Source
	// Resolver fills in IMDb ids for media that arrives with only a TVDB id.
	Resolver IDResolver
}

// Service answers rating lookups: the IMDb rating from the table, the rest
// from the source through a cache with bounded concurrency and a bounded wait.
type Service struct {
	opts   Options
	logger *slog.Logger
	omdb   *asyncCache[[]tvdb.Rating]
	now    func() time.Time
	budget time.Duration

	mu          sync.Mutex
	pausedUntil time.Time
}

// NewService returns a service drawing on opts.
func NewService(opts Options, logger *slog.Logger) *Service {
	return &Service{
		opts:   opts,
		logger: logger,
		omdb:   newAsyncCache[[]tvdb.Rating](lookupConcurrency, lookupTimeout),
		now:    time.Now,
		budget: waitBudget,
	}
}

// Attach fills Ratings on every item that has no ratings yet. Items without
// an IMDb id are resolved from their TVDB id first (and keep the id); items
// sharing an id share one lookup. It returns once every lookup has answered
// or the wait budget is spent, whichever is first.
func (s *Service) Attach(ctx context.Context, media []tvdb.Media) {
	ctx, cancel := context.WithTimeout(ctx, s.budget)
	defer cancel()

	s.resolveIDs(ctx, media)

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

// resolveIDs fills ImdbID on items that have a TVDB id but no IMDb id, one
// resolution per distinct title.
func (s *Service) resolveIDs(ctx context.Context, media []tvdb.Media) {
	if s.opts.Resolver == nil {
		return
	}
	type title struct{ mediaType, id string }
	byTitle := map[title][]int{}
	for i, item := range media {
		if item.ImdbID != "" || item.Id == "" || len(item.Ratings) > 0 {
			continue
		}
		key := title{item.Category, item.Id}
		byTitle[key] = append(byTitle[key], i)
	}
	if len(byTitle) == 0 {
		return
	}
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results = make(map[title]string, len(byTitle))
	)
	for key := range byTitle {
		wg.Add(1)
		go func(key title) {
			defer wg.Done()
			imdbID := s.opts.Resolver.ImdbID(ctx, key.mediaType, key.id)
			mu.Lock()
			results[key] = imdbID
			mu.Unlock()
		}(key)
	}
	wg.Wait()
	for key, indexes := range byTitle {
		for _, i := range indexes {
			media[i].ImdbID = results[key]
		}
	}
}

// Lookup returns the ratings for an IMDb id, IMDb first, then Rotten
// Tomatoes, then Metacritic: the IMDb rating from the table, the rest from
// the source (from its cache, or waiting at most the wait budget for a new
// lookup). nil means none are known yet, or the title has none.
func (s *Service) Lookup(ctx context.Context, imdbID string) []tvdb.Rating {
	if imdbID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.budget)
	defer cancel()

	var ratings []tvdb.Rating
	if s.opts.IMDb != nil {
		if rating, ok := s.opts.IMDb.Rating(imdbID); ok {
			ratings = append(ratings, rating)
		}
	}
	for _, rating := range s.fromSource(ctx, imdbID) {
		if rating.Source == tvdb.RatingSourceIMDb && len(ratings) > 0 {
			continue // the table's figure is fresher and always carries votes
		}
		ratings = append(ratings, rating)
	}
	return ratings
}

// fromSource returns the source's ratings for an id through the cache,
// unless the source is paused or absent.
func (s *Service) fromSource(ctx context.Context, imdbID string) []tvdb.Rating {
	if s.opts.Source == nil {
		return nil
	}
	if cached, ok := s.omdb.peek(imdbID); ok {
		return cached
	}
	s.mu.Lock()
	paused := s.now().Before(s.pausedUntil)
	s.mu.Unlock()
	if paused {
		return nil
	}
	ratings, _ := s.omdb.get(ctx, imdbID, func(ctx context.Context) ([]tvdb.Rating, time.Duration) {
		ratings, err := s.opts.Source.Lookup(ctx, imdbID)
		switch {
		case errors.Is(err, ErrRateLimited), errors.Is(err, ErrUnauthorized):
			s.pause(ctx, err)
			return nil, 0
		case err != nil:
			// A failed call is simply retried by the next asker.
			s.logger.WarnContext(ctx, "Rating lookup failed",
				telemetry.WithTraceContext(ctx, "imdb_id", imdbID, "error", err)...)
			return nil, 0
		case len(ratings) == 0:
			return nil, missTTL
		default:
			return ratings, ratingTTL
		}
	})
	return ratings
}

// pause stops source lookups for pauseFor, logging once per pause.
func (s *Service) pause(ctx context.Context, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if now.After(s.pausedUntil) {
		s.logger.WarnContext(ctx, "Rating source refused the key, pausing lookups",
			telemetry.WithTraceContext(ctx, "error", err, "pause", pauseFor.String())...)
	}
	s.pausedUntil = now.Add(pauseFor)
}
