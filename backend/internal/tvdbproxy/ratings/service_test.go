package ratings

import (
	"context"
	"errors"
	"log/slog"
	"sort"
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

// stubTable is an IMDbTable with a fixed set of rows.
type stubTable map[string]tvdb.Rating

func (t stubTable) Rating(imdbID string) (tvdb.Rating, bool) {
	rating, ok := t[imdbID]
	return rating, ok
}

// stubResolver maps "mediaType:id" to an IMDb id and counts asks.
type stubResolver struct {
	mu    sync.Mutex
	ids   map[string]string
	calls int
}

func (r *stubResolver) ImdbID(_ context.Context, mediaType, id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.ids[mediaType+":"+id]
}

var (
	imdbEight = []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8}}
	tomato97  = tvdb.Rating{Source: tvdb.RatingSourceRottenTomatoes, Value: 97}
)

func newTestService(opts Options) (*Service, *time.Time) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	s := NewService(opts, slog.Default())
	s.now = func() time.Time { return now }
	s.omdb.now = s.now
	return s, &now
}

func TestAttachFillsRatingsOncePerTitle(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, _ := newTestService(Options{Source: source})

	media := []tvdb.Media{
		{Id: "a", ImdbID: "tt1"},
		{Id: "b"},
		{Id: "c", ImdbID: "tt1"},
		{Id: "d", ImdbID: "tt2"},
		{Id: "e", ImdbID: "tt3", Ratings: []tvdb.Rating{{Source: "x", Value: 1}}},
	}
	s.Attach(context.Background(), media)

	assert.Equal(t, imdbEight, media[0].Ratings)
	assert.Nil(t, media[1].Ratings, "no IMDb id and no resolver, nothing to look up")
	assert.Equal(t, imdbEight, media[2].Ratings, "the second item with the same id shares the answer")
	assert.Nil(t, media[3].Ratings, "the source has nothing for tt2")
	assert.Equal(t, []tvdb.Rating{{Source: "x", Value: 1}}, media[4].Ratings, "already rated items are left alone")
	assert.Equal(t, 1, source.count("tt1"))
	assert.Equal(t, 1, source.count("tt2"))
	assert.Equal(t, 0, source.count("tt3"))
}

func TestAttachResolvesTVDBIDsFirst(t *testing.T) {
	resolver := &stubResolver{ids: map[string]string{"series:10": "tt1", "movie:20": "tt2"}}
	table := stubTable{"tt1": imdbEight[0], "tt2": {Source: tvdb.RatingSourceIMDb, Value: 6.5, Votes: 12}}
	s, _ := newTestService(Options{IMDb: table, Resolver: resolver})

	media := []tvdb.Media{
		{Id: "10", Category: "series"},
		{Id: "20", Category: "movie"},
		{Id: "10", Category: "series"},
		{Id: "30", Category: "movie"},
		{Id: "40", Category: "movie", ImdbID: "tt2"},
	}
	s.Attach(context.Background(), media)

	assert.Equal(t, "tt1", media[0].ImdbID, "the resolved id is kept on the item")
	assert.Equal(t, imdbEight, media[0].Ratings)
	assert.Equal(t, 6.5, media[1].Ratings[0].Value)
	assert.Equal(t, imdbEight, media[2].Ratings)
	assert.Empty(t, media[3].ImdbID)
	assert.Nil(t, media[3].Ratings, "TVDB lists no IMDb id for 30")
	assert.Equal(t, 6.5, media[4].Ratings[0].Value, "an item that came with its id is not resolved")
	assert.Equal(t, 3, resolver.calls, "one resolution per distinct title")
}

func TestLookupMergesTableAndSource(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 7.9}, tomato97}
	table := stubTable{"tt1": {Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}}
	s, _ := newTestService(Options{IMDb: table, Source: source})

	got := s.Lookup(context.Background(), "tt1")

	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}, tomato97}, got,
		"the table's IMDb figure leads, the source adds the rest")
}

func TestLookupTableOnlyAndSourceOnly(t *testing.T) {
	table := stubTable{"tt1": {Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}}
	s, _ := newTestService(Options{IMDb: table})
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}}, s.Lookup(context.Background(), "tt1"))
	assert.Nil(t, s.Lookup(context.Background(), "tt2"))

	source := newStubSource()
	source.answers["tt1"] = []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 7.9}, tomato97}
	s, _ = newTestService(Options{Source: source})
	assert.Equal(t, source.answers["tt1"], s.Lookup(context.Background(), "tt1"), "without a table the source's IMDb figure stands")
}

func TestLookupCachesHitsAndMisses(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, now := newTestService(Options{Source: source})

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
	s, _ := newTestService(Options{Source: source})

	assert.Nil(t, s.Lookup(context.Background(), ""))
	assert.Empty(t, source.calls)
}

func TestLookupPausesWhenTheKeyIsRefused(t *testing.T) {
	source := newStubSource()
	source.err = ErrRateLimited
	table := stubTable{"tt2": imdbEight[0]}
	s, now := newTestService(Options{IMDb: table, Source: source})

	assert.Nil(t, s.Lookup(context.Background(), "tt1"))
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt2"), "the table still answers while paused")
	assert.Equal(t, 1, source.count("tt1"))
	assert.Equal(t, 0, source.count("tt2"), "no source lookups while paused")

	source.err = nil
	source.answers["tt2"] = []tvdb.Rating{tomato97}
	*now = now.Add(pauseFor + time.Second)
	assert.Equal(t, []tvdb.Rating{imdbEight[0], tomato97}, s.Lookup(context.Background(), "tt2"),
		"lookups resume after the pause")
}

func TestLookupRetriesAfterASourceError(t *testing.T) {
	source := newStubSource()
	source.err = errors.New("connection reset")
	s, _ := newTestService(Options{Source: source})

	assert.Nil(t, s.Lookup(context.Background(), "tt1"))
	source.err = nil
	source.answers["tt1"] = imdbEight
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt1"))
	assert.Equal(t, 2, source.count("tt1"), "a failed call is not cached")
}

func TestAttachGivesUpAfterTheBudgetAndCachesLater(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, _ := newTestService(Options{Source: source})
	s.budget = 30 * time.Millisecond

	source.gate.Lock() // the lookup blocks until released
	media := []tvdb.Media{{Id: "a", ImdbID: "tt1"}}
	started := time.Now()
	s.Attach(context.Background(), media)
	assert.Less(t, time.Since(started), time.Second)
	assert.Nil(t, media[0].Ratings, "the slow answer is left off")

	source.gate.Unlock()
	require.Eventually(t, func() bool {
		_, ok := s.omdb.peek("tt1")
		return ok
	}, time.Second, 5*time.Millisecond, "the detached lookup still lands in the cache")

	s.Attach(context.Background(), media)
	assert.Equal(t, imdbEight, media[0].Ratings)
	assert.Equal(t, 1, source.count("tt1"))
}

func TestConcurrentLookupsShareOneCall(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, _ := newTestService(Options{Source: source})

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
		s.omdb.mu.Lock()
		defer s.omdb.mu.Unlock()
		return len(s.omdb.inflight) == 1
	}, time.Second, time.Millisecond)
	source.gate.Unlock()
	wg.Wait()

	for _, got := range results {
		assert.Equal(t, imdbEight, got)
	}
	assert.Equal(t, 1, source.count("tt1"))
}

// stubScores is a ScoreSource answering from a table and recording each batch.
type stubScores struct {
	mu      sync.Mutex
	answers map[string][]tvdb.Rating
	err     error
	batches [][]string
}

func (s *stubScores) Scores(_ context.Context, ids []string) (map[string][]tvdb.Rating, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches = append(s.batches, append([]string(nil), ids...))
	if s.err != nil {
		return nil, s.err
	}
	out := map[string][]tvdb.Rating{}
	for _, id := range ids {
		if ratings, ok := s.answers[id]; ok {
			out[id] = ratings
		}
	}
	return out, nil
}

func (s *stubScores) batchCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.batches)
}

func TestLookupManyMergesTableWikidataAndSource(t *testing.T) {
	meta84 := tvdb.Rating{Source: tvdb.RatingSourceMetacritic, Value: 84}
	wikidata := &stubScores{answers: map[string][]tvdb.Rating{
		"tt1": {{Source: tvdb.RatingSourceRottenTomatoes, Value: 91}, meta84},
		"tt2": {{Source: tvdb.RatingSourceRottenTomatoes, Value: 70}},
	}}
	source := newStubSource()
	source.answers["tt1"] = []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 7.9}, tomato97}
	table := stubTable{"tt1": {Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}}
	s, _ := newTestService(Options{IMDb: table, Wikidata: wikidata, Source: source})

	got := s.LookupMany(context.Background(), []string{"tt1", "tt2", "tt3", "tt1", ""})

	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}, tomato97, meta84}, got["tt1"],
		"table IMDb, the source's current Tomatometer over Wikidata's, Wikidata's Metascore")
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceRottenTomatoes, Value: 70}}, got["tt2"],
		"Wikidata alone still gives a Tomatometer")
	_, ok := got["tt3"]
	assert.False(t, ok)
	require.Equal(t, 1, wikidata.batchCount(), "one query for the whole batch")
	assert.Equal(t, []string{"tt1", "tt2", "tt3"}, wikidata.batches[0], "deduplicated, no empty id")

	s.LookupMany(context.Background(), []string{"tt1", "tt2", "tt3"})
	assert.Equal(t, 1, wikidata.batchCount(), "hits and misses are cached")
}

func TestLookupManyChunksWikidataQueries(t *testing.T) {
	wikidata := &stubScores{answers: map[string][]tvdb.Rating{}}
	s, _ := newTestService(Options{Wikidata: wikidata})
	ids := make([]string, WikidataChunk+5)
	for i := range ids {
		ids[i] = "tt" + strconv.Itoa(1000+i)
	}

	s.LookupMany(context.Background(), ids)

	require.Equal(t, 2, wikidata.batchCount())
	sizes := []int{len(wikidata.batches[0]), len(wikidata.batches[1])}
	sort.Ints(sizes)
	assert.Equal(t, []int{5, WikidataChunk}, sizes, "chunks run concurrently, so their order varies")
}

func TestLookupManyRetriesWikidataAfterAnError(t *testing.T) {
	wikidata := &stubScores{err: errors.New("429")}
	s, _ := newTestService(Options{Wikidata: wikidata})

	assert.Empty(t, s.LookupMany(context.Background(), []string{"tt1"}))
	wikidata.mu.Lock()
	wikidata.err = nil
	wikidata.answers = map[string][]tvdb.Rating{"tt1": {tomato97}}
	wikidata.mu.Unlock()
	assert.Equal(t, []tvdb.Rating{tomato97}, s.LookupMany(context.Background(), []string{"tt1"})["tt1"])
	assert.Equal(t, 2, wikidata.batchCount(), "a failed query is not cached")
}

func TestAttachUsesWikidataForMovies(t *testing.T) {
	wikidata := &stubScores{answers: map[string][]tvdb.Rating{"tt1": {tomato97}}}
	table := stubTable{"tt1": imdbEight[0]}
	s, _ := newTestService(Options{IMDb: table, Wikidata: wikidata})

	media := []tvdb.Media{{Id: "a", Category: "movie", ImdbID: "tt1"}}
	s.Attach(context.Background(), media)

	assert.Equal(t, []tvdb.Rating{imdbEight[0], tomato97}, media[0].Ratings)
}
