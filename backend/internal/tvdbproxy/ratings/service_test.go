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
	s.Attach(context.Background(), media, DefaultWait)

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
	s.Attach(context.Background(), media, DefaultWait)

	assert.Equal(t, "tt1", media[0].ImdbID, "the resolved id is kept on the item")
	assert.Equal(t, imdbEight, media[0].Ratings)
	assert.Equal(t, 6.5, media[1].Ratings[0].Value)
	assert.Equal(t, imdbEight, media[2].Ratings)
	assert.Empty(t, media[3].ImdbID)
	assert.Nil(t, media[3].Ratings, "TVDB lists no IMDb id for 30")
	assert.Equal(t, 6.5, media[4].Ratings[0].Value, "an item that came with its id is not resolved")
	assert.Equal(t, 3, resolver.calls, "one resolution per distinct title")
}

// stubFinder maps "name|year" to an IMDb id.
type stubFinder struct {
	mu    sync.Mutex
	ids   map[string]string
	calls int
}

func (f *stubFinder) FindIMDbID(_ context.Context, name, year string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	id, ok := f.ids[name+"|"+year]
	return id, ok
}

func TestAttachFindsIDsForFilmsTVDBLacks(t *testing.T) {
	resolver := &stubResolver{ids: map[string]string{"movie:10": "tt1"}}
	finder := &stubFinder{ids: map[string]string{"Mononoke the Movie|2025": "tt2"}}
	table := stubTable{"tt1": imdbEight[0], "tt2": {Source: tvdb.RatingSourceIMDb, Value: 7.1, Votes: 300}}
	s, _ := newTestService(Options{IMDb: table, Resolver: resolver, Finder: finder})

	media := []tvdb.Media{
		{Id: "10", Category: "movie", Name: "Known"},
		{Id: "20", Category: "movie", Name: "Mononoke the Movie", Year: "2025"},
		{Id: "30", Category: "movie", Name: "Unknown Film", Year: "2025"},
		{Id: "40", Category: "series", Name: "Mononoke the Movie", Year: "2025"},
	}
	s.Attach(context.Background(), media, DefaultWait)

	assert.Equal(t, "tt1", media[0].ImdbID, "TVDB's own id comes first")
	assert.Equal(t, "tt2", media[1].ImdbID, "a film TVDB has no id for is found by name and year")
	assert.Equal(t, 7.1, media[1].Ratings[0].Value)
	assert.Empty(t, media[2].ImdbID)
	assert.Empty(t, media[3].ImdbID, "series are not searched")
	assert.Equal(t, 2, finder.calls, "one search per distinct film title")

	s.Attach(context.Background(), []tvdb.Media{{Id: "20", Category: "movie", Name: "Mononoke the Movie", Year: "2025"}}, DefaultWait)
	assert.Equal(t, 2, finder.calls, "the answer is cached")
}

func TestLookupMergesTableAndSource(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 7.9}, tomato97}
	table := stubTable{"tt1": {Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}}
	s, _ := newTestService(Options{IMDb: table, Source: source})

	got := s.Lookup(context.Background(), "tt1", DefaultWait)

	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}, tomato97}, got,
		"the table's IMDb figure leads, the source adds the rest")
}

func TestLookupTableOnlyAndSourceOnly(t *testing.T) {
	table := stubTable{"tt1": {Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}}
	s, _ := newTestService(Options{IMDb: table})
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}}, s.Lookup(context.Background(), "tt1", DefaultWait))
	assert.Nil(t, s.Lookup(context.Background(), "tt2", DefaultWait))

	source := newStubSource()
	source.answers["tt1"] = []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 7.9}, tomato97}
	s, _ = newTestService(Options{Source: source})
	assert.Equal(t, source.answers["tt1"], s.Lookup(context.Background(), "tt1", DefaultWait), "without a table the source's IMDb figure stands")
}

func TestLookupCachesHitsAndMisses(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, now := newTestService(Options{Source: source})

	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt1", DefaultWait))
	assert.Nil(t, s.Lookup(context.Background(), "tt2", DefaultWait))
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt1", DefaultWait))
	assert.Nil(t, s.Lookup(context.Background(), "tt2", DefaultWait))
	assert.Equal(t, 1, source.count("tt1"), "a hit is cached")
	assert.Equal(t, 1, source.count("tt2"), "a miss is cached too")

	*now = now.Add(missTTL + time.Second)
	assert.Nil(t, s.Lookup(context.Background(), "tt2", DefaultWait))
	assert.Equal(t, 2, source.count("tt2"), "a miss expires after missTTL")
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt1", DefaultWait))
	assert.Equal(t, 1, source.count("tt1"), "a hit outlives missTTL")

	*now = now.Add(ratingTTL)
	s.Lookup(context.Background(), "tt1", DefaultWait)
	assert.Equal(t, 2, source.count("tt1"), "a hit expires after ratingTTL")
}

func TestLookupEmptyIDAsksNothing(t *testing.T) {
	source := newStubSource()
	s, _ := newTestService(Options{Source: source})

	assert.Nil(t, s.Lookup(context.Background(), "", DefaultWait))
	assert.Empty(t, source.calls)
}

func TestLookupPausesWhenTheKeyIsRefused(t *testing.T) {
	source := newStubSource()
	source.err = ErrRateLimited
	table := stubTable{"tt2": imdbEight[0]}
	s, now := newTestService(Options{IMDb: table, Source: source})

	assert.Nil(t, s.Lookup(context.Background(), "tt1", DefaultWait))
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt2", DefaultWait), "the table still answers while paused")
	assert.Equal(t, 1, source.count("tt1"))
	assert.Equal(t, 0, source.count("tt2"), "no source lookups while paused")

	source.err = nil
	source.answers["tt2"] = []tvdb.Rating{tomato97}
	*now = now.Add(pauseFor + time.Second)
	assert.Equal(t, []tvdb.Rating{imdbEight[0], tomato97}, s.Lookup(context.Background(), "tt2", DefaultWait),
		"lookups resume after the pause")
}

func TestLookupRetriesAfterASourceError(t *testing.T) {
	source := newStubSource()
	source.err = errors.New("connection reset")
	s, _ := newTestService(Options{Source: source})

	assert.Nil(t, s.Lookup(context.Background(), "tt1", DefaultWait))
	source.err = nil
	source.answers["tt1"] = imdbEight
	assert.Equal(t, imdbEight, s.Lookup(context.Background(), "tt1", DefaultWait))
	assert.Equal(t, 2, source.count("tt1"), "a failed call is not cached")
}

func TestAttachGivesUpAfterTheBudgetAndCachesLater(t *testing.T) {
	source := newStubSource()
	source.answers["tt1"] = imdbEight
	s, _ := newTestService(Options{Source: source})

	source.gate.Lock() // the lookup blocks until released
	media := []tvdb.Media{{Id: "a", ImdbID: "tt1"}}
	started := time.Now()
	s.Attach(context.Background(), media, 30*time.Millisecond)
	assert.Less(t, time.Since(started), time.Second)
	assert.Nil(t, media[0].Ratings, "the slow answer is left off")

	source.gate.Unlock()
	require.Eventually(t, func() bool {
		_, ok := s.omdb.peek("tt1")
		return ok
	}, time.Second, 5*time.Millisecond, "the detached lookup still lands in the cache")

	s.Attach(context.Background(), media, DefaultWait)
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
			results[i] = s.Lookup(context.Background(), "tt1", DefaultWait)
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

// stubScores is a RecordSource answering from a table and recording each batch.
type stubScores struct {
	mu      sync.Mutex
	answers map[string][]tvdb.Rating
	records map[string]Record
	err     error
	batches [][]string
}

func (s *stubScores) Records(_ context.Context, ids []string) (map[string]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches = append(s.batches, append([]string(nil), ids...))
	if s.err != nil {
		return nil, s.err
	}
	out := map[string]Record{}
	for _, id := range ids {
		if ratings, ok := s.answers[id]; ok {
			out[id] = Record{Ratings: ratings}
		}
		if record, ok := s.records[id]; ok {
			out[id] = record
		}
	}
	return out, nil
}

// stubProse is a ProseSource answering from a table of article titles and
// a table of "name|year" → article for searches.
type stubProse struct {
	mu       sync.Mutex
	answers  map[string][]tvdb.Rating
	articles map[string]string
	err      error
	asked    [][]string
	searched []string
}

func (p *stubProse) Search(_ context.Context, name, year string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.searched = append(p.searched, name+"|"+year)
	article, ok := p.articles[name+"|"+year]
	return article, ok
}

func (p *stubProse) Scores(_ context.Context, titles []string) (map[string][]tvdb.Rating, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, append([]string(nil), titles...))
	if p.err != nil {
		return nil, p.err
	}
	out := map[string][]tvdb.Rating{}
	for _, title := range titles {
		if ratings, ok := p.answers[title]; ok {
			out[title] = ratings
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

	got := s.LookupMany(context.Background(), []string{"tt1", "tt2", "tt3", "tt1", ""}, DefaultWait)

	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8, Votes: 500}, tomato97, meta84}, got["tt1"],
		"table IMDb, the source's current Tomatometer over Wikidata's, Wikidata's Metascore")
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceRottenTomatoes, Value: 70}}, got["tt2"],
		"Wikidata alone still gives a Tomatometer")
	_, ok := got["tt3"]
	assert.False(t, ok)
	require.Equal(t, 1, wikidata.batchCount(), "one query for the whole batch")
	assert.Equal(t, []string{"tt1", "tt2", "tt3"}, wikidata.batches[0], "deduplicated, no empty id")

	s.LookupMany(context.Background(), []string{"tt1", "tt2", "tt3"}, DefaultWait)
	assert.Equal(t, 1, wikidata.batchCount(), "hits and misses are cached")
}

func TestLookupManyChunksWikidataQueries(t *testing.T) {
	wikidata := &stubScores{answers: map[string][]tvdb.Rating{}}
	s, _ := newTestService(Options{Wikidata: wikidata})
	ids := make([]string, WikidataChunk+5)
	for i := range ids {
		ids[i] = "tt" + strconv.Itoa(1000+i)
	}

	s.LookupMany(context.Background(), ids, DefaultWait)

	require.Equal(t, 2, wikidata.batchCount())
	sizes := []int{len(wikidata.batches[0]), len(wikidata.batches[1])}
	sort.Ints(sizes)
	assert.Equal(t, []int{5, WikidataChunk}, sizes, "chunks run concurrently, so their order varies")
}

func TestLookupManyRetriesWikidataAfterAnError(t *testing.T) {
	wikidata := &stubScores{err: errors.New("429")}
	s, _ := newTestService(Options{Wikidata: wikidata})

	assert.Empty(t, s.LookupMany(context.Background(), []string{"tt1"}, DefaultWait))
	wikidata.mu.Lock()
	wikidata.err = nil
	wikidata.answers = map[string][]tvdb.Rating{"tt1": {tomato97}}
	wikidata.mu.Unlock()
	assert.Equal(t, []tvdb.Rating{tomato97}, s.LookupMany(context.Background(), []string{"tt1"}, DefaultWait)["tt1"])
	assert.Equal(t, 2, wikidata.batchCount(), "a failed query is not cached")
}

func TestLookupManyReadsProseForUnscoredFilms(t *testing.T) {
	meta59 := tvdb.Rating{Source: tvdb.RatingSourceMetacritic, Value: 59}
	wikidata := &stubScores{records: map[string]Record{
		"tt1": {Article: "Clayface (film)", Film: true},
		"tt2": {Article: "Some Show", Film: false},
		"tt3": {Article: "Old Film", Film: true, Ratings: []tvdb.Rating{tomato97}},
		"tt4": {Film: true},
		"tt5": {Article: "Also Clayface", Film: true, Ratings: []tvdb.Rating{meta59}},
	}}
	prose := &stubProse{answers: map[string][]tvdb.Rating{
		"Clayface (film)": {{Source: tvdb.RatingSourceRottenTomatoes, Value: 77}, meta59},
		"Some Show":       {{Source: tvdb.RatingSourceRottenTomatoes, Value: 10}},
		"Also Clayface":   {{Source: tvdb.RatingSourceRottenTomatoes, Value: 80}, {Source: tvdb.RatingSourceMetacritic, Value: 1}},
	}}
	s, _ := newTestService(Options{Wikidata: wikidata, Prose: prose})

	got := s.LookupMany(context.Background(), []string{"tt1", "tt2", "tt3", "tt4", "tt5"}, DefaultWait)

	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceRottenTomatoes, Value: 77}, meta59}, got["tt1"],
		"a film Wikidata has not scored gets both scores from its article")
	assert.Nil(t, got["tt2"], "series articles are not read")
	assert.Equal(t, []tvdb.Rating{tomato97}, got["tt3"], "a film Wikidata scored is not re-read")
	assert.Nil(t, got["tt4"], "no article, nothing to read")
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceRottenTomatoes, Value: 80}, meta59}, got["tt5"],
		"prose fills the missing Tomatometer; Wikidata's Metascore stands")
	require.Len(t, prose.asked, 1)
	assert.Equal(t, []string{"Also Clayface", "Clayface (film)"}, prose.asked[0], "one article fetch per chunk")

	s.LookupMany(context.Background(), []string{"tt1"}, DefaultWait)
	assert.Len(t, prose.asked, 1, "cached with the rest")
}

func TestAttachUsesWikidataForMovies(t *testing.T) {
	wikidata := &stubScores{answers: map[string][]tvdb.Rating{"tt1": {tomato97}}}
	table := stubTable{"tt1": imdbEight[0]}
	s, _ := newTestService(Options{IMDb: table, Wikidata: wikidata})

	media := []tvdb.Media{{Id: "a", Category: "movie", ImdbID: "tt1"}}
	s.Attach(context.Background(), media, DefaultWait)

	assert.Equal(t, []tvdb.Rating{imdbEight[0], tomato97}, media[0].Ratings)
}

func TestAttachSearchesArticlesForFilmsWikidataLacks(t *testing.T) {
	wikidata := &stubScores{records: map[string]Record{"tt1": {Article: "Known Film", Film: true}}}
	prose := &stubProse{
		answers:  map[string][]tvdb.Rating{"Known Film": {tomato97}, "Last Take (film)": {{Source: tvdb.RatingSourceRottenTomatoes, Value: 86}}},
		articles: map[string]string{"Last Take|2025": "Last Take (film)"},
	}
	s, _ := newTestService(Options{Wikidata: wikidata, Prose: prose})

	media := []tvdb.Media{
		{Id: "a", Category: "movie", ImdbID: "tt1", Name: "Known Film", Year: "2020"},
		{Id: "b", Category: "movie", ImdbID: "tt2", Name: "Last Take", Year: "2025"},
		{Id: "c", Category: "movie", ImdbID: "tt3", Name: "Unknown Doc", Year: "2025"},
		{Id: "d", Category: "series", ImdbID: "tt4", Name: "Some Show", Year: "2025"},
	}
	s.Attach(context.Background(), media, DefaultWait)

	assert.Equal(t, []tvdb.Rating{tomato97}, media[0].Ratings)
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceRottenTomatoes, Value: 86}}, media[1].Ratings,
		"a film Wikidata has no item for is found by name and year")
	assert.Nil(t, media[2].Ratings)
	assert.Nil(t, media[3].Ratings)
	assert.ElementsMatch(t, []string{"Last Take|2025", "Unknown Doc|2025"}, prose.searched,
		"only films without a Wikidata item are searched; series are not")

	s.LookupMany(context.Background(), []string{"tt2"}, DefaultWait)
	assert.Len(t, prose.searched, 2, "the answer is cached with the chunk")
}

func TestLookupManyKeepsWikidataScoresWhenProseFails(t *testing.T) {
	meta59 := tvdb.Rating{Source: tvdb.RatingSourceMetacritic, Value: 59}
	wikidata := &stubScores{records: map[string]Record{
		"tt1": {Article: "New Film", Film: true},
		"tt5": {Article: "Other Film", Film: true, Ratings: []tvdb.Rating{meta59}},
	}}
	prose := &stubProse{err: errors.New("503")}
	s, _ := newTestService(Options{Wikidata: wikidata, Prose: prose})

	got := s.LookupMany(context.Background(), []string{"tt1", "tt5"}, DefaultWait)

	assert.Nil(t, got["tt1"])
	assert.Equal(t, []tvdb.Rating{meta59}, got["tt5"], "Wikidata's own scores still stand")
	assert.Len(t, prose.asked, 1)
}
