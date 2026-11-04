// Package ratings attaches critic and audience scores to media. Three
// sources, all keyed by IMDb id: IMDb's public daily dataset for the IMDb
// rating (no key), Wikidata's recorded review scores for the Rotten
// Tomatoes Tomatometer and Metascore (no key, lags on new releases), and
// OMDb for a current Tomatometer and Metascore when a key is configured.
// Media that only carries a TVDB id (library items, person-credit search
// hits) is resolved to an IMDb id first. Answers are cached for days and
// concurrent asks share one lookup, since a free OMDb key allows 1,000
// calls a day and Wikidata's query service is shared by everyone.
package ratings

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

const (
	// ratingTTL is how long a title's OMDb or Wikidata scores are served
	// before being looked up again; scores move slowly once a title is out.
	ratingTTL = 7 * 24 * time.Hour
	// missTTL is how long "the source has no record" is remembered.
	missTTL = 24 * time.Hour
	// pauseFor is how long OMDb lookups stop after it refuses the key,
	// whether its daily allowance is used up or it is wrong.
	pauseFor = time.Hour
	// lookupConcurrency bounds the OMDb calls in flight at once.
	lookupConcurrency = 4
	// lookupTimeout bounds one OMDb call. The call runs detached from the
	// request that asked, so a caller giving up does not waste the answer.
	lookupTimeout = 15 * time.Second
	// chunkConcurrency bounds the Wikidata queries in flight at once.
	chunkConcurrency = 2
	// chunkTimeout bounds one Wikidata query.
	chunkTimeout = 30 * time.Second
	// chunkTTL is how long a finished query is remembered, only so that
	// concurrent asks for one chunk share it; the per-title answers it
	// produced live for ratingTTL.
	chunkTTL = time.Minute
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

// ScoreSource looks up many titles' scores at once; Wikidata implements it.
// Ids the source has nothing for are absent from the map.
type ScoreSource interface {
	Scores(ctx context.Context, imdbIDs []string) (map[string][]tvdb.Rating, error)
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
	// Wikidata answers the Tomatometer and Metascore without a key.
	Wikidata ScoreSource
	// Source is the keyed source (OMDb) for a current Tomatometer and Metascore.
	Source Source
	// Resolver fills in IMDb ids for media that arrives with only a TVDB id.
	Resolver IDResolver
}

// Service answers rating lookups: the IMDb rating from the table, the rest
// from Wikidata and the source through caches with bounded concurrency and
// a bounded wait.
type Service struct {
	opts   Options
	logger *slog.Logger
	omdb   *asyncCache[[]tvdb.Rating]
	chunks *asyncCache[struct{}]
	now    func() time.Time
	budget time.Duration

	mu          sync.Mutex
	pausedUntil time.Time
	wikidata    map[string]cacheEntry[[]tvdb.Rating]
}

// NewService returns a service drawing on opts.
func NewService(opts Options, logger *slog.Logger) *Service {
	return &Service{
		opts:     opts,
		logger:   logger,
		omdb:     newAsyncCache[[]tvdb.Rating](lookupConcurrency, lookupTimeout),
		chunks:   newAsyncCache[struct{}](chunkConcurrency, chunkTimeout),
		now:      time.Now,
		budget:   waitBudget,
		wikidata: map[string]cacheEntry[[]tvdb.Rating]{},
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
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}

	results := s.LookupMany(ctx, ids)
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

// Lookup returns the ratings for one IMDb id; see LookupMany.
func (s *Service) Lookup(ctx context.Context, imdbID string) []tvdb.Rating {
	if imdbID == "" {
		return nil
	}
	return s.LookupMany(ctx, []string{imdbID})[imdbID]
}

// LookupMany returns the ratings for each IMDb id, IMDb first, then Rotten
// Tomatoes, then Metacritic: the IMDb rating from the table, the rest from
// the source where it has them and Wikidata otherwise, each from its cache
// or waiting at most the wait budget for a new lookup. Ids with nothing
// known yet, or nothing at all, are absent.
func (s *Service) LookupMany(ctx context.Context, imdbIDs []string) map[string][]tvdb.Rating {
	ctx, cancel := context.WithTimeout(ctx, s.budget)
	defer cancel()

	ids := uniqueIDs(imdbIDs)
	if len(ids) == 0 {
		return map[string][]tvdb.Rating{}
	}

	var (
		recorded map[string][]tvdb.Rating
		sourced  = map[string][]tvdb.Rating{}
		mu       sync.Mutex
		wg       sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		recorded = s.fromWikidata(ctx, ids)
	}()
	if s.opts.Source != nil {
		for _, id := range ids {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				ratings := s.fromSource(ctx, id)
				mu.Lock()
				sourced[id] = ratings
				mu.Unlock()
			}(id)
		}
	}
	wg.Wait()

	results := make(map[string][]tvdb.Rating, len(ids))
	for _, id := range ids {
		if ratings := s.merge(id, sourced[id], recorded[id]); len(ratings) > 0 {
			results[id] = ratings
		}
	}
	return results
}

// merge orders one title's scores: the table's IMDb figure (the source's
// when there is no table), then the Tomatometer and Metascore, the source's
// figure winning over Wikidata's older one.
func (s *Service) merge(imdbID string, sourced, recorded []tvdb.Rating) []tvdb.Rating {
	pick := func(source string) (tvdb.Rating, bool) {
		for _, list := range [][]tvdb.Rating{sourced, recorded} {
			for _, rating := range list {
				if rating.Source == source {
					return rating, true
				}
			}
		}
		return tvdb.Rating{}, false
	}
	var ratings []tvdb.Rating
	if s.opts.IMDb != nil {
		if rating, ok := s.opts.IMDb.Rating(imdbID); ok {
			ratings = append(ratings, rating)
		}
	}
	if len(ratings) == 0 {
		if rating, ok := pick(tvdb.RatingSourceIMDb); ok {
			ratings = append(ratings, rating)
		}
	}
	for _, source := range []string{tvdb.RatingSourceRottenTomatoes, tvdb.RatingSourceMetacritic} {
		if rating, ok := pick(source); ok {
			ratings = append(ratings, rating)
		}
	}
	return ratings
}

// fromWikidata returns Wikidata's scores for the ids: cached answers at
// once, the rest through one query per chunk of ids, run detached and
// shared by every caller asking for that chunk.
func (s *Service) fromWikidata(ctx context.Context, ids []string) map[string][]tvdb.Rating {
	if s.opts.Wikidata == nil {
		return nil
	}
	s.mu.Lock()
	now := s.now()
	var need []string
	for _, id := range ids {
		if e, ok := s.wikidata[id]; !ok || !now.Before(e.expires) {
			need = append(need, id)
		}
	}
	s.mu.Unlock()

	var wg sync.WaitGroup
	for start := 0; start < len(need); start += WikidataChunk {
		chunk := need[start:min(start+WikidataChunk, len(need))]
		wg.Add(1)
		go func(chunk []string) {
			defer wg.Done()
			s.chunks.get(ctx, strings.Join(chunk, ","), func(ctx context.Context) (struct{}, time.Duration) {
				scores, err := s.opts.Wikidata.Scores(ctx, chunk)
				if err != nil {
					// A failed query is simply retried by the next asker.
					s.logger.WarnContext(ctx, "Wikidata scores lookup failed",
						telemetry.WithTraceContext(ctx, "count", len(chunk), "error", err)...)
					return struct{}{}, 0
				}
				s.mu.Lock()
				defer s.mu.Unlock()
				now := s.now()
				if len(s.wikidata) >= pruneAbove {
					for key, e := range s.wikidata {
						if now.After(e.expires) {
							delete(s.wikidata, key)
						}
					}
				}
				for _, id := range chunk {
					ratings, ttl := scores[id], missTTL
					if len(ratings) > 0 {
						ttl = ratingTTL
					}
					s.wikidata[id] = cacheEntry[[]tvdb.Rating]{value: ratings, expires: now.Add(ttl)}
				}
				return struct{}{}, chunkTTL
			})
		}(chunk)
	}
	wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	now = s.now()
	results := make(map[string][]tvdb.Rating, len(ids))
	for _, id := range ids {
		if e, ok := s.wikidata[id]; ok && now.Before(e.expires) && len(e.value) > 0 {
			results[id] = e.value
		}
	}
	return results
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

// uniqueIDs drops empty and repeated ids, keeping a stable order.
func uniqueIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
