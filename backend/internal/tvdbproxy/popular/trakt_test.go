package popular

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTVDB answers Get from canned bodies keyed by URI and records every call.
type fakeTVDB struct {
	mu     sync.Mutex
	bodies map[string]string
	errors map[string]error
	calls  []string
}

func newFakeTVDB() *fakeTVDB {
	return &fakeTVDB{bodies: map[string]string{}, errors: map[string]error{}}
}

func (f *fakeTVDB) Get(_ context.Context, uri string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, uri)
	if err, ok := f.errors[uri]; ok {
		return nil, err
	}
	body, ok := f.bodies[uri]
	if !ok {
		return nil, fmt.Errorf("TVDB API error: status 404 (%s)", uri)
	}
	return []byte(body), nil
}

func (f *fakeTVDB) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, call := range f.calls {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeTVDB) series(id int, name string, extra string) {
	f.bodies[fmt.Sprintf("/series/%d", id)] = fmt.Sprintf(
		`{"data":{"id":%d,"name":%q,"slug":"slug-%d","image":"/banners/%d.jpg","year":"2020","score":1000,`+
			`"status":{"id":1,"name":"Continuing"},"originalLanguage":"eng","originalCountry":"usa",`+
			`"averageRuntime":45%s}}`, id, name, id, id, extra)
}

func (f *fakeTVDB) movie(imdb string, id int, name string) {
	f.bodies["/search/remoteid/"+imdb] = fmt.Sprintf(
		`{"data":[{"people":{"id":1}},{"movie":{"id":%d,"name":%q,"slug":"slug-%d",`+
			`"image":"https://artworks.thetvdb.com/banners/movies/%d.jpg","year":"2024","score":500,`+
			`"status":{"id":5,"name":"Released"},"runtime":120}}]}`, id, name, id, id)
}

// fakeTrakt is a Trakt API answering one canned body per path and recording requests.
type fakeTrakt struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	requests []*http.Request
	bodies   map[string]string
	status   int
}

func newFakeTrakt(t *testing.T) *fakeTrakt {
	f := &fakeTrakt{t: t, bodies: map[string]string{}, status: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r)
		body, ok := f.bodies[r.URL.Path]
		status := f.status
		f.mu.Unlock()
		if status != http.StatusOK {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(status)
			return
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTrakt) last() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(f.t, f.requests)
	return f.requests[len(f.requests)-1]
}

func showEntry(watchers, tvdbID int, imdb, title string, genres ...string) map[string]any {
	return map[string]any{
		"watcher_count": watchers,
		"play_count":    watchers * 3,
		"show": map[string]any{
			"title": title, "year": 2025, "overview": "about " + title, "runtime": 50,
			"certification": "TV-MA", "network": "HBO", "country": "us", "genres": genres,
			"ids": map[string]any{"trakt": tvdbID, "slug": "s", "tvdb": tvdbID, "imdb": imdb, "tmdb": 1},
		},
	}
}

func movieEntry(watchers int, imdb, title string, genres ...string) map[string]any {
	return map[string]any{
		"watcher_count": watchers,
		"movie": map[string]any{
			"title": title, "year": 2024, "overview": "about " + title, "runtime": 130, "tagline": "tag",
			"certification": "PG-13", "country": "us", "genres": genres,
			"ids": map[string]any{"trakt": 1, "slug": "m", "imdb": imdb, "tmdb": 2},
		},
	}
}

func mustJSON(t *testing.T, v any) string {
	body, err := json.Marshal(v)
	require.NoError(t, err)
	return string(body)
}

func newTraktUnderTest(t *testing.T, trakt *fakeTrakt, tvdbAPI *fakeTVDB, opts TraktOptions) *TraktSource {
	opts.Host = trakt.srv.URL
	opts.ClientID = "client-id"
	return NewTraktSource(opts, trakt.srv.Client(), tvdbAPI, slog.Default())
}

func TestTraktShowsRequestAndMapping(t *testing.T) {
	trakt := newFakeTrakt(t)
	trakt.bodies["/shows/watched/weekly"] = mustJSON(t, []any{
		showEntry(500, 121361, "tt0944947", "Game of Thrones", "drama", "fantasy"),
		showEntry(400, 81797, "tt0388629", "One Piece", "anime", "action"),
	})
	tvdbAPI := newFakeTVDB()
	tvdbAPI.series(121361, "Game of Thrones", "")
	tvdbAPI.series(81797, "ワンピース", `,"originalLanguage":"jpn","originalCountry":"jpn"`)

	src := newTraktUnderTest(t, trakt, tvdbAPI, TraktOptions{})
	results, err := src.Rank(context.Background(), KindSeries, "Drama", 20)
	require.NoError(t, err)

	req := trakt.last()
	assert.Equal(t, "/shows/watched/weekly", req.URL.Path)
	assert.Equal(t, "2", req.Header.Get("trakt-api-version"))
	assert.Equal(t, "client-id", req.Header.Get("trakt-api-key"))
	assert.NotEmpty(t, req.Header.Get("User-Agent"))
	assert.Equal(t, "drama", req.URL.Query().Get("genres"))
	assert.Equal(t, "full", req.URL.Query().Get("extended"))
	assert.Equal(t, "30", req.URL.Query().Get("limit"), "asks for the limit plus slack")

	require.Len(t, results, 2)
	got := results[0]
	assert.Equal(t, "121361", got.Id)
	assert.Equal(t, "Game of Thrones", got.Name)
	assert.Equal(t, "series", got.Category)
	assert.Equal(t, "https://artworks.thetvdb.com/banners/121361.jpg", got.ImageUrl)
	assert.Equal(t, "about Game of Thrones", got.Overview)
	assert.Equal(t, "2025", got.Year, "Trakt's year wins over TVDB's")
	assert.Equal(t, 50, got.Runtime)
	assert.Equal(t, "TV-MA", got.ContentRating)
	assert.Equal(t, "HBO", got.Network)
	assert.Equal(t, "tt0944947", got.ImdbID)
	assert.Equal(t, []string{"Drama", "Fantasy"}, got.Genres)
	assert.Equal(t, "Continuing", got.Status.Name)
	assert.Equal(t, float64(1000), got.Score)
	assert.Equal(t, &tvdb.Popularity{Source: "trakt", Value: 500, Detail: "watchers, weekly"}, got.Popularity)
	assert.False(t, got.Anime)
	assert.NotNil(t, got.Aliases)
	assert.NotNil(t, got.Metadata.Episodes)

	anime := results[1]
	assert.Equal(t, "One Piece", anime.Name, "Trakt's English title is the display name")
	assert.Equal(t, "ワンピース", anime.OriginalName)
	assert.True(t, anime.Anime)
	assert.Equal(t, "jpn", anime.OriginalLanguage)
}

func TestTraktMoviesResolveThroughIMDb(t *testing.T) {
	trakt := newFakeTrakt(t)
	trakt.bodies["/movies/watched/monthly"] = mustJSON(t, []any{
		movieEntry(900, "tt0848228", "The Avengers", "action", "science-fiction"),
		movieEntry(800, "tt0245429", "Spirited Away", "animation", "fantasy"),
	})
	tvdbAPI := newFakeTVDB()
	tvdbAPI.movie("tt0848228", 31, "The Avengers")
	tvdbAPI.movie("tt0245429", 1550, "千と千尋の神隠し")

	src := newTraktUnderTest(t, trakt, tvdbAPI, TraktOptions{Period: "monthly"})
	results, err := src.Rank(context.Background(), KindMovie, "Sci-Fi", 20)
	require.NoError(t, err)

	req := trakt.last()
	assert.Equal(t, "/movies/watched/monthly", req.URL.Path)
	assert.Equal(t, "science-fiction", req.URL.Query().Get("genres"))

	require.Len(t, results, 2)
	assert.Equal(t, "31", results[0].Id)
	assert.Equal(t, "movie", results[0].Category)
	assert.Equal(t, "The Avengers", results[0].Name)
	assert.Equal(t, "tag", results[0].Tagline)
	assert.Equal(t, []string{"Action", "Science Fiction"}, results[0].Genres)
	assert.Equal(t, "https://artworks.thetvdb.com/banners/movies/31.jpg", results[0].ImageUrl)
	assert.False(t, results[0].Anime)
	assert.Equal(t, "Spirited Away", results[1].Name)
	assert.True(t, results[1].Anime, "an animated movie counts as anime, as in search")
	assert.Equal(t, "Released", results[1].Status.Name)
}

func TestTraktTrendingPathAndOrder(t *testing.T) {
	trakt := newFakeTrakt(t)
	trakt.bodies["/shows/trending"] = mustJSON(t, []any{
		map[string]any{"watchers": 9, "show": map[string]any{"title": "B", "ids": map[string]any{"tvdb": 2}}},
		map[string]any{"watchers": 5, "show": map[string]any{"title": "A", "ids": map[string]any{"tvdb": 1}}},
	})
	tvdbAPI := newFakeTVDB()
	tvdbAPI.series(1, "A", "")
	tvdbAPI.series(2, "B", "")

	src := newTraktUnderTest(t, trakt, tvdbAPI, TraktOptions{List: "trending"})
	assert.Equal(t, "trakt trending", src.Name())
	results, err := src.Rank(context.Background(), KindSeries, "", 20)
	require.NoError(t, err)

	assert.Equal(t, "/shows/trending", trakt.last().URL.Path)
	assert.Empty(t, trakt.last().URL.Query().Get("genres"))
	require.Len(t, results, 2)
	assert.Equal(t, &tvdb.Popularity{Source: "trakt", Value: 9, Detail: "watching now"}, results[0].Popularity)
	assert.Equal(t, []string{"2", "1"}, []string{results[0].Id, results[1].Id}, "Trakt's order is kept")
}

func TestTraktSkipsUnresolvableHitsAndHonoursLimit(t *testing.T) {
	trakt := newFakeTrakt(t)
	trakt.bodies["/shows/watched/weekly"] = mustJSON(t, []any{
		showEntry(9, 0, "", "no tvdb id"),
		showEntry(8, 404, "", "unknown to tvdb"),
		showEntry(7, 500, "", "tvdb error"),
		showEntry(6, 1, "", "one"),
		showEntry(5, 2, "", "two"),
		showEntry(4, 3, "", "three"),
	})
	tvdbAPI := newFakeTVDB()
	tvdbAPI.bodies["/series/404"] = `{"data":null}`
	tvdbAPI.errors["/series/500"] = errors.New("TVDB API error: status 500")
	tvdbAPI.series(1, "one", "")
	tvdbAPI.series(2, "two", "")
	tvdbAPI.series(3, "three", "")

	src := newTraktUnderTest(t, trakt, tvdbAPI, TraktOptions{})
	results, err := src.Rank(context.Background(), KindSeries, "", 2)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "one", results[0].Name)
	assert.Equal(t, "two", results[1].Name)

	// Records and definite misses are cached; only the failed lookup is retried.
	before := tvdbAPI.count("/series/")
	_, err = src.Rank(context.Background(), KindSeries, "", 2)
	require.NoError(t, err)
	assert.Equal(t, before+1, tvdbAPI.count("/series/"), "hits and misses are remembered, the error is retried")
	assert.Equal(t, 2, tvdbAPI.count("/series/500"))
}

func TestTraktMovieWithoutIMDbIsSkipped(t *testing.T) {
	trakt := newFakeTrakt(t)
	trakt.bodies["/movies/watched/weekly"] = mustJSON(t, []any{
		map[string]any{"watcher_count": 3, "movie": map[string]any{"title": "no imdb", "ids": map[string]any{"tmdb": 5}}},
		map[string]any{"watcher_count": 2, "movie": map[string]any{"title": "no movie record",
			"ids": map[string]any{"imdb": "tt1"}}},
	})
	tvdbAPI := newFakeTVDB()
	tvdbAPI.bodies["/search/remoteid/tt1"] = `{"data":[{"series":{"id":7,"name":"a show"}}]}`

	src := newTraktUnderTest(t, trakt, tvdbAPI, TraktOptions{})
	results, err := src.Rank(context.Background(), KindMovie, "", 20)
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Equal(t, 1, tvdbAPI.count("/search/remoteid/"))
}

func TestTraktErrorsSurface(t *testing.T) {
	trakt := newFakeTrakt(t)
	trakt.status = http.StatusTooManyRequests
	src := newTraktUnderTest(t, trakt, newFakeTVDB(), TraktOptions{})

	_, err := src.Rank(context.Background(), KindSeries, "", 20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
	assert.Contains(t, err.Error(), "7")

	trakt.status = http.StatusOK
	trakt.bodies["/shows/watched/weekly"] = `not json`
	_, err = src.Rank(context.Background(), KindSeries, "", 20)
	require.Error(t, err)
}

func TestTraktOptionsNormalize(t *testing.T) {
	opts := TraktOptions{}.Normalize(slog.Default())
	assert.Equal(t, DefaultTraktHost, opts.Host)
	assert.Equal(t, TraktListWatched, opts.List)
	assert.Equal(t, DefaultTraktPeriod, opts.Period)

	opts = TraktOptions{Host: "https://x/", List: "Trending", Period: "hourly"}.Normalize(slog.Default())
	assert.Equal(t, "https://x", opts.Host)
	assert.Equal(t, TraktListTrending, opts.List)
	assert.Equal(t, DefaultTraktPeriod, opts.Period, "an unknown period falls back to weekly")

	opts = TraktOptions{List: "bogus", Period: "ALL"}.Normalize(slog.Default())
	assert.Equal(t, TraktListWatched, opts.List)
	assert.Equal(t, "all", opts.Period)
}

func TestTraktGenreSlug(t *testing.T) {
	cases := map[string]string{
		"Action": "action", "Comedy": "comedy", "Drama": "drama", "Sci-Fi": "science-fiction",
		"Science Fiction": "science-fiction", "Anime": "anime", "Documentary": "documentary",
		"Talk Show": "talk-show", "Home and Garden": "home-and-garden", "Martial Arts": "martial-arts",
	}
	for name, want := range cases {
		assert.Equal(t, want, TraktGenreSlug(name), name)
	}
	assert.Equal(t, "Science Fiction", traktGenreName("science-fiction"))
	assert.Equal(t, "Anime", traktGenreName("anime"))
}

func TestTraktLimitIsCappedAtOnePage(t *testing.T) {
	trakt := newFakeTrakt(t)
	trakt.bodies["/shows/watched/weekly"] = `[]`
	src := newTraktUnderTest(t, trakt, newFakeTVDB(), TraktOptions{})

	_, err := src.Rank(context.Background(), KindSeries, "", 500)
	require.NoError(t, err)
	limit := trakt.last().URL.Query().Get("limit")
	assert.Equal(t, "60", limit, "MaxLimit plus slack")
	_, err = url.Parse(trakt.last().URL.String())
	require.NoError(t, err)
}

func TestTraktCancelledContext(t *testing.T) {
	trakt := newFakeTrakt(t)
	trakt.bodies["/shows/watched/weekly"] = `[]`
	src := newTraktUnderTest(t, trakt, newFakeTVDB(), TraktOptions{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.Rank(ctx, KindSeries, "", 20)
	require.ErrorIs(t, err, context.Canceled)
}
