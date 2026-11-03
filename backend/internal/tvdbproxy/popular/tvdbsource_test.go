package popular

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// today is the fixed clock the TVDB source tests run under.
var today = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

type stubGenres map[string]int

func (g stubGenres) LookupID(name string) (int, bool) {
	id, ok := g[strings.ToLower(name)]
	return id, ok
}

func newTVDBUnderTest(tvdbAPI *fakeTVDB) *TVDBSource {
	src := NewTVDBSource(tvdbAPI, stubGenres{"drama": 12, "anime": 27, "sci-fi": 24}, slog.Default())
	src.now = func() time.Time { return today }
	return src
}

// filterBody builds a /filter response from compact records.
func filterBody(t *testing.T, recs ...map[string]any) string {
	body, err := json.Marshal(map[string]any{"status": "success", "data": recs})
	require.NoError(t, err)
	return string(body)
}

func seriesRec(id int, name string, score float64, first, last string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "slug": "s", "image": "/banners/" + name + ".jpg", "score": score,
		"year": first[:4], "firstAired": first, "lastAired": last, "originalLanguage": "eng",
		"originalCountry": "usa", "status": map[string]any{"id": 1, "name": "Continuing"},
	}
}

func movieRec(id int, name string, score float64, year, status string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "slug": "m", "image": "/banners/movies/" + name + ".jpg", "score": score,
		"year": year, "runtime": 100, "status": map[string]any{"id": 5, "name": status},
	}
}

func seriesURI(country, lang string, genre int, extra string) string {
	uri := fmt.Sprintf("/series/filter?country=%s&lang=%s&sort=score&sortType=desc", country, lang)
	if genre != 0 {
		uri += fmt.Sprintf("&genre=%d", genre)
	}
	return uri + extra
}

func movieURI(country, lang string, genre int, extra string) string {
	uri := fmt.Sprintf("/movies/filter?country=%s&lang=%s&sort=score&sortType=desc", country, lang)
	if genre != 0 {
		uri += fmt.Sprintf("&genre=%d", genre)
	}
	return uri + extra
}

func TestTVDBSeriesQueriesAndRanking(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	empty := filterBody(t)
	// Lifetime scores: the long-runner has 13x the new hit's score.
	tvdbAPI.bodies[seriesURI("usa", "eng", 12, "&year=2026")] = filterBody(t,
		seriesRec(1, "New Hit", 374000, "2026-01-18", "2026-02-22"),
		seriesRec(2, "Not Yet Aired", 900000, "2026-12-25", "2026-12-25"),
	)
	tvdbAPI.bodies[seriesURI("usa", "eng", 12, "&year=2025")] = filterBody(t,
		seriesRec(3, "Last Year's Hit", 425000, "2025-01-09", "2026-03-01"),
	)
	tvdbAPI.bodies[seriesURI("usa", "eng", 12, "&status=1")] = filterBody(t,
		seriesRec(4, "Airing Long-Runner", 5500000, "2005-03-27", "2026-11-19"),
		seriesRec(5, "Resting Long-Runner", 4400000, "2011-12-04", "2025-04-10"),
		seriesRec(3, "Last Year's Hit", 425000, "2025-01-09", "2026-03-01"),
	)
	tvdbAPI.bodies[seriesURI("gbr", "eng", 12, "&year=2026")] = filterBody(t,
		seriesRec(6, "UK Hit", 200000, "2026-03-13", "2026-04-17"),
	)
	tvdbAPI.bodies[seriesURI("gbr", "eng", 12, "&year=2025")] = empty
	tvdbAPI.bodies[seriesURI("gbr", "eng", 12, "&status=1")] = empty

	src := newTVDBUnderTest(tvdbAPI)
	results, err := src.Rank(context.Background(), KindSeries, "Drama", 20)
	require.NoError(t, err)

	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	// New Hit 374000*1; Last Year's Hit 425000*0.5=212500; UK Hit 200000;
	// Airing Long-Runner 5500000*0.02=110000; Resting Long-Runner ~0.
	assert.Equal(t, []string{"New Hit", "Last Year's Hit", "UK Hit", "Airing Long-Runner", "Resting Long-Runner"}, names)
	assert.NotContains(t, names, "Not Yet Aired")
	assert.Equal(t, 6, tvdbAPI.count("/series/filter"), "two regions, two years and continuing each")
	assert.Equal(t, 0, tvdbAPI.count("/series/1"), "English titles need no translation lookup")

	got := results[0]
	assert.Equal(t, "1", got.Id)
	assert.Equal(t, "series", got.Category)
	assert.Equal(t, "https://artworks.thetvdb.com/banners/New Hit.jpg", got.ImageUrl)
	assert.Equal(t, "2026", got.Year)
	assert.Equal(t, float64(374000), got.Score)
	assert.False(t, got.Anime)
}

func TestTVDBAnimeUsesJapanAndEnglishNames(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	empty := filterBody(t)
	jp := seriesRec(81797, "ワンピース", 3236299, "1999-10-20", "2026-11-08")
	jp["originalLanguage"] = "jpn"
	jp["originalCountry"] = "jpn"
	tvdbAPI.bodies[seriesURI("jpn", "jpn", 27, "&status=1")] = filterBody(t, jp)
	tvdbAPI.bodies[seriesURI("jpn", "jpn", 27, "&year=2026")] = empty
	tvdbAPI.bodies[seriesURI("jpn", "jpn", 27, "&year=2025")] = empty
	tvdbAPI.bodies[seriesURI("usa", "eng", 27, "&year=2026")] = filterBody(t,
		seriesRec(9, "Western Toon", 1000, "2026-02-01", "2026-03-01"))
	tvdbAPI.bodies[seriesURI("usa", "eng", 27, "&year=2025")] = empty
	tvdbAPI.bodies[seriesURI("usa", "eng", 27, "&status=1")] = empty
	tvdbAPI.bodies["/series/81797/translations/eng"] = `{"data":{"name":"One Piece","language":"eng"}}`

	src := newTVDBUnderTest(tvdbAPI)
	results, err := src.Rank(context.Background(), KindSeries, "Anime", 20)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "One Piece", results[0].Name)
	assert.Equal(t, "ワンピース", results[0].OriginalName)
	assert.True(t, results[0].Anime)
	assert.Equal(t, "Western Toon", results[1].Name)
	assert.Equal(t, 0, tvdbAPI.count("/series/filter?country=gbr"), "anime rows do not query the UK")

	_, err = src.Rank(context.Background(), KindSeries, "Anime", 20)
	require.NoError(t, err)
	assert.Equal(t, 1, tvdbAPI.count("/series/81797/translations/eng"), "English names are cached")
}

func TestTVDBMoviesRanking(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	tvdbAPI.bodies[movieURI("usa", "eng", 0, "&year=2026")] = filterBody(t,
		movieRec(1, "Unreleased Tentpole", 70000, "2026", "Filming / Post-Production"),
		movieRec(2, "This Year's Best", 14000, "2026", "Released"),
		movieRec(3, "This Year's Second", 7000, "2026", "Released"),
		movieRec(8, "Future Year", 99999, "2027", "Released"),
	)
	tvdbAPI.bodies[movieURI("usa", "eng", 0, "&year=2025")] = filterBody(t,
		movieRec(4, "Last Year's Best", 80000, "2025", "Released"),
		movieRec(5, "Last Year's Second", 20000, "2025", "Released"),
	)
	tvdbAPI.bodies[movieURI("gbr", "eng", 0, "&year=2026")] = filterBody(t,
		movieRec(2, "This Year's Best", 14000, "2026", "Released"),
		movieRec(6, "Re-release From 2023", 800000, "2023", "Released"),
	)
	tvdbAPI.bodies[movieURI("gbr", "eng", 0, "&year=2025")] = filterBody(t,
		movieRec(7, "Last Year's UK", 40000, "2025", "Released"))

	src := newTVDBUnderTest(tvdbAPI)
	results, err := src.Rank(context.Background(), KindMovie, "", 20)
	require.NoError(t, err)

	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	// Relative to the best of each year, then 0.5 per year of age:
	// This Year's Best 1.0; This Year's Second 0.5; Last Year's Best 0.5 (tie
	// broken by raw score); Last Year's UK 0.25; Last Year's Second 0.125.
	assert.Equal(t, []string{
		"This Year's Best", "Last Year's Best", "This Year's Second", "Last Year's UK", "Last Year's Second",
	}, names)
	assert.NotContains(t, names, "Unreleased Tentpole")
	assert.NotContains(t, names, "Future Year")
	assert.NotContains(t, names, "Re-release From 2023", "older than the two-year window")
	assert.Equal(t, 4, tvdbAPI.count("/movies/filter"), "two regions, two years each")
	assert.Equal(t, "movie", results[0].Category)
	assert.Equal(t, 100, results[0].Runtime)
	assert.Equal(t, "Released", results[0].Status.Name)
}

func TestTVDBLimitAndUnknownGenre(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	var recs []map[string]any
	for i := 1; i <= 30; i++ {
		recs = append(recs, movieRec(i, fmt.Sprintf("m%d", i), float64(1000-i), "2026", "Released"))
	}
	for _, region := range englishRegions {
		for _, year := range []string{"2026", "2025"} {
			tvdbAPI.bodies[movieURI(region.country, region.lang, 24, "&year="+year)] = filterBody(t, recs...)
		}
	}
	src := newTVDBUnderTest(tvdbAPI)

	results, err := src.Rank(context.Background(), KindMovie, "Sci-Fi", 5)
	require.NoError(t, err)
	assert.Len(t, results, 5)
	assert.Equal(t, "m1", results[0].Name)

	_, err = src.Rank(context.Background(), KindMovie, "Nope", 5)
	require.ErrorIs(t, err, ErrUnknownGenre)
}

func TestTVDBToleratesPartialFailures(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	for _, region := range englishRegions {
		for _, year := range []string{"2026", "2025"} {
			tvdbAPI.errors[movieURI(region.country, region.lang, 0, "&year="+year)] = errors.New("boom")
		}
	}
	tvdbAPI.bodies[movieURI("usa", "eng", 0, "&year=2026")] = filterBody(t,
		movieRec(1, "Only One", 10, "2026", "Released"))
	delete(tvdbAPI.errors, movieURI("usa", "eng", 0, "&year=2026"))

	src := newTVDBUnderTest(tvdbAPI)
	results, err := src.Rank(context.Background(), KindMovie, "", 20)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Only One", results[0].Name)

	// Every query failing is an error.
	tvdbAPI.errors[movieURI("usa", "eng", 0, "&year=2026")] = errors.New("boom")
	_, err = src.Rank(context.Background(), KindMovie, "", 20)
	require.Error(t, err)
}

func TestRankSeriesEdgeCases(t *testing.T) {
	recs := []baseRecord{
		{ID: 1, Name: "no dates", Score: 100},
		{ID: 2, Name: "upcoming", Score: 100, FirstAired: "2026-01-01", Status: statusNamed("Upcoming")},
		{ID: 3, Name: "ok", Score: 100, FirstAired: "2026-01-01", LastAired: "bad-date"},
	}
	ranked := rankSeries(recs, today)
	require.Len(t, ranked, 1)
	assert.Equal(t, "ok", ranked[0].Name)
}

func TestRankMoviesUnknownStatusCountsAsReleased(t *testing.T) {
	ranked := rankMovies([]baseRecord{{ID: 1, Name: "x", Score: 5, Year: "2026"}}, today)
	require.Len(t, ranked, 1)
	ranked = rankMovies([]baseRecord{{ID: 1, Name: "x", Score: 5, Year: "nope"}}, today)
	assert.Empty(t, ranked)
}

func statusNamed(name string) tvdb.Status { return tvdb.Status{Name: name} }

func TestTVDBAnimeMoviesGetEnglishNames(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	empty := filterBody(t)
	// Movie filter records have no originalLanguage: the query's language decides.
	chainsaw := movieRec(400, "チェンソーマン レゼ篇", 900, "2025", "Released")
	chainsaw["nameTranslations"] = []string{"jpn", "eng"}
	untranslated := movieRec(401, "翻訳なし", 800, "2025", "Released")
	untranslated["nameTranslations"] = []string{"jpn"}
	tvdbAPI.bodies[movieURI("jpn", "jpn", 27, "&year=2025")] = filterBody(t, chainsaw, untranslated)
	tvdbAPI.bodies[movieURI("jpn", "jpn", 27, "&year=2026")] = empty
	for _, year := range []string{"2026", "2025"} {
		tvdbAPI.bodies[movieURI("usa", "eng", 27, "&year="+year)] = empty
	}
	tvdbAPI.bodies["/movies/400/translations/eng"] = `{"data":{"name":"Chainsaw Man - The Movie: Reze Arc"}}`

	src := newTVDBUnderTest(tvdbAPI)
	results, err := src.Rank(context.Background(), KindMovie, "Anime", 20)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "Chainsaw Man - The Movie: Reze Arc", results[0].Name)
	assert.Equal(t, "チェンソーマン レゼ篇", results[0].OriginalName)
	assert.Equal(t, "jpn", results[0].OriginalLanguage)
	assert.True(t, results[0].Anime)
	assert.Equal(t, "翻訳なし", results[1].Name, "kept under its own name when TVDB lists no English one")
	assert.Equal(t, 0, tvdbAPI.count("/movies/401/"), "no lookup when nameTranslations lacks eng")
}

// stubAttention answers Views from a table of article views and Trending from a list.
type stubAttention struct {
	mu       sync.Mutex
	articles map[string]string // "kind|name|year" -> article
	views    map[string]int    // article -> views
	trending map[Kind][]TrendingArticle
	asked    []string
}

func (a *stubAttention) Measure(_ context.Context, kind Kind, titles []Title) []Measurement {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Measurement, len(titles))
	for i, title := range titles {
		key := string(kind) + "|" + title.Name + "|" + title.Year
		a.asked = append(a.asked, key)
		if article, ok := a.articles[key]; ok {
			out[i] = Measurement{Article: article, Views: a.views[article], Found: true}
		}
	}
	return out
}

func (a *stubAttention) ArticleViews(_ context.Context, article string) (int, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	views, ok := a.views[article]
	return views, ok
}

func (a *stubAttention) Trending(_ context.Context, kind Kind) []TrendingArticle {
	return a.trending[kind]
}

func TestTVDBAttentionReordersPoolAndHarvestsTrending(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	empty := filterBody(t)
	tvdbAPI.bodies[movieURI("usa", "eng", 0, "&year=2026")] = filterBody(t,
		movieRec(1, "Big Score Low Interest", 90000, "2026", "Released"),
		movieRec(2, "Mid Score High Interest", 40000, "2026", "Released"),
		movieRec(3, "No Article", 30000, "2026", "Released"),
	)
	tvdbAPI.bodies[movieURI("usa", "eng", 0, "&year=2025")] = empty
	tvdbAPI.bodies[movieURI("gbr", "eng", 0, "&year=2026")] = empty
	tvdbAPI.bodies[movieURI("gbr", "eng", 0, "&year=2025")] = empty
	// Trending articles: one new title, one already in the pool, one unreleased, one unknown to TVDB.
	tvdbAPI.bodies["/search?limit=5&query=Obsession&type=movie&year=2025"] = `{"data":[
		{"id":"movie-7","name":"Obsession","translations":{"eng":"Obsession"}}]}`
	tvdbAPI.bodies["/movies/7/extended"] = `{"data":{"id":7,"name":"Obsession","slug":"obsession","image":"/o.jpg",
		"year":"2025","score":500,"status":{"id":5,"name":"Released"},"originalLanguage":"eng",
		"genres":[{"id":11,"name":"Horror"}],"remoteIds":[{"id":"tt7","sourceName":"IMDB"}]}}`
	tvdbAPI.bodies["/search?limit=5&query=Mid+Score+High+Interest&type=movie&year=2026"] = `{"data":[
		{"id":"movie-2","name":"Mid Score High Interest","translations":{"eng":"Mid Score High Interest"}}]}`
	tvdbAPI.bodies["/movies/2/extended"] = `{"data":{"id":2,"name":"Mid Score High Interest","year":"2026",
		"status":{"id":5,"name":"Released"}}}`
	tvdbAPI.bodies["/search?limit=5&query=Doomsday&type=movie&year=2026"] = `{"data":[
		{"id":"movie-9","name":"Doomsday","translations":{"eng":"Doomsday"}}]}`
	tvdbAPI.bodies["/movies/9/extended"] = `{"data":{"id":9,"name":"Doomsday","year":"2026",
		"status":{"id":3,"name":"Filming / Post-Production"}}}`
	tvdbAPI.bodies["/search?limit=5&query=Unknown&type=movie"] = `{"data":[]}`
	// A year-less trending article resolves to the most recent namesake, and
	// an old namesake alone is not current.
	tvdbAPI.bodies["/search?limit=5&query=Sigma&type=movie"] = `{"data":[
		{"id":"movie-20","name":"Sigma","year":"2005","translations":{"eng":"Sigma"}},
		{"id":"movie-21","name":"Sigma","year":"2026","translations":{"eng":"Sigma"}}]}`
	tvdbAPI.bodies["/movies/21/extended"] = `{"data":{"id":21,"name":"Sigma","year":"2026",
		"status":{"id":5,"name":"Released"}}}`
	tvdbAPI.bodies["/search?limit=5&query=The+Shining&type=movie&year=1980"] = `{"data":[
		{"id":"movie-30","name":"The Shining","year":"1980","translations":{"eng":"The Shining"}}]}`
	tvdbAPI.bodies["/movies/30/extended"] = `{"data":{"id":30,"name":"The Shining","year":"1980",
		"status":{"id":5,"name":"Released"}}}`

	attention := &stubAttention{
		articles: map[string]string{
			"movie|Big Score Low Interest|2026":  "Big Score Low Interest (film)",
			"movie|Mid Score High Interest|2026": "Mid Score High Interest (film)",
		},
		views: map[string]int{
			"Big Score Low Interest (film)":  1000,
			"Mid Score High Interest (film)": 50000,
			"Obsession (2025 film)":          91000,
			"Sigma (film)":                   60000,
			"The Shining (1980 film)":        99000,
		},
		trending: map[Kind][]TrendingArticle{KindMovie: {
			{Article: "Obsession (2025 film)", Title: "Obsession", Year: "2025", Views: 20000},
			{Article: "Mid Score High Interest (film)", Title: "Mid Score High Interest", Year: "2026", Views: 9000},
			{Article: "Doomsday (2026 film)", Title: "Doomsday", Year: "2026", Views: 80000},
			{Article: "Unknown (film)", Title: "Unknown", Views: 70000},
			{Article: "Sigma (film)", Title: "Sigma", Views: 10000},
			{Article: "The Shining (1980 film)", Title: "The Shining", Year: "1980", Views: 30000},
		}},
	}
	src := newTVDBUnderTest(tvdbAPI)
	src.SetAttention(attention)
	assert.Contains(t, src.Name(), "attention")

	results, err := src.Rank(context.Background(), KindMovie, "", 20)
	require.NoError(t, err)
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	assert.Equal(t, []string{"Obsession", "Sigma", "Mid Score High Interest", "Big Score Low Interest", "No Article"},
		names)
	assert.Equal(t, "tt7", results[0].ImdbID, "harvested titles carry their IMDb id")
	assert.Equal(t, "https://artworks.thetvdb.com/o.jpg", results[0].ImageUrl)
	assert.Equal(t,
		&tvdb.Popularity{Source: "wikipedia", Value: 91000, Detail: "Obsession (2025 film), views over 7 days"},
		results[0].Popularity)
	assert.Equal(t, "wikipedia", results[3].Popularity.Source)
	assert.Equal(t, 1000, results[3].Popularity.Value)
	assert.Equal(t, &tvdb.Popularity{Source: "tvdb", Value: 30000, Detail: "lifetime score"}, results[4].Popularity,
		"a title with no article reports TVDB's score")
	assert.Equal(t, "2026", results[1].Year, "a year-less trending title resolves to the most recent namesake")
	assert.NotContains(t, names, "Doomsday", "an unreleased trending movie is not offered")
	assert.NotContains(t, names, "The Shining", "a classic trending this week is outside the release window")
}

func TestTVDBAttentionHarvestRespectsGenreAndAnimeNames(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	empty := filterBody(t)
	jp := seriesRec(81797, "ワンピース", 3236299, "1999-10-20", "2026-11-08")
	jp["originalLanguage"] = "jpn"
	for _, region := range animeRegions {
		for _, extra := range []string{"&year=2026", "&year=2025", "&status=1"} {
			tvdbAPI.bodies[seriesURI(region.country, region.lang, 27, extra)] = empty
		}
	}
	tvdbAPI.bodies[seriesURI("jpn", "jpn", 27, "&status=1")] = filterBody(t, jp)
	tvdbAPI.bodies["/series/81797/translations/eng"] = `{"data":{"name":"One Piece"}}`
	// Trending: a drama (wrong genre) and an anime (right genre).
	tvdbAPI.bodies["/search?limit=5&query=Nocturne&type=series"] = `{"data":[
		{"id":"series-5","name":"Nocturne","translations":{"eng":"Nocturne"}}]}`
	tvdbAPI.bodies["/series/5/extended"] = `{"data":{"id":5,"name":"Nocturne","year":"2026","firstAired":"2026-10-01",
		"status":{"id":1,"name":"Continuing"},"genres":[{"id":12,"name":"Drama"}]}}`
	tvdbAPI.bodies["/search?limit=5&query=Gachiakuta&type=series"] = `{"data":[
		{"id":"series-6","name":"ガチアクタ","translations":{"eng":"Gachiakuta"}}]}`
	tvdbAPI.bodies["/series/6/extended"] = `{"data":{"id":6,"name":"ガチアクタ","year":"2025","firstAired":"2025-07-06",
		"status":{"id":1,"name":"Continuing"},"originalLanguage":"jpn","nameTranslations":["jpn","eng"],
		"genres":[{"id":27,"name":"Anime"}]}}`
	tvdbAPI.bodies["/series/6/translations/eng"] = `{"data":{"name":"Gachiakuta"}}`

	attention := &stubAttention{
		articles: map[string]string{"series|One Piece|1999": "One Piece"},
		views:    map[string]int{"One Piece": 30000, "Gachiakuta": 45000, "Nocturne (TV series)": 60000},
		trending: map[Kind][]TrendingArticle{KindSeries: {
			{Article: "Nocturne (TV series)", Title: "Nocturne", Views: 59441},
			{Article: "Gachiakuta", Title: "Gachiakuta", Views: 40000},
		}},
	}
	src := newTVDBUnderTest(tvdbAPI)
	src.SetAttention(attention)

	results, err := src.Rank(context.Background(), KindSeries, "Anime", 20)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "Gachiakuta", results[0].Name)
	assert.Equal(t, "One Piece", results[1].Name)
	assert.True(t, results[0].Anime)
	assert.Contains(t, attention.asked, "series|One Piece|1999", "attention is looked up under the English name")
}

type stubAnime struct {
	trends map[Kind][]AnimeTrend
	err    error
}

func (a *stubAnime) Trending(_ context.Context, kind Kind) ([]AnimeTrend, error) {
	return a.trends[kind], a.err
}

func TestTVDBAnimeRowLeadsWithAniListTrending(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	empty := filterBody(t)
	for _, region := range animeRegions {
		for _, extra := range []string{"&year=2026", "&year=2025", "&status=1"} {
			tvdbAPI.bodies[seriesURI(region.country, region.lang, 27, extra)] = empty
		}
	}
	pool := []map[string]any{}
	for i := 1; i <= 6; i++ {
		pool = append(pool, seriesRec(i, fmt.Sprintf("Pool %d", i), float64(1000-i), "2025-01-01", "2026-11-01"))
	}
	tvdbAPI.bodies[seriesURI("usa", "eng", 27, "&year=2025")] = filterBody(t, pool...)
	// Trending anime: one found by English title, one only by romaji, one not
	// yet aired, one unknown to TVDB, one that is already in the pool.
	tvdbAPI.bodies["/search?limit=5&query=Gachiakuta&type=series&year=2025"] = `{"data":[
		{"id":"series-100","name":"ガチアクタ","year":"2025","translations":{"eng":"Gachiakuta"}}]}`
	tvdbAPI.bodies["/series/100/extended"] = `{"data":{"id":100,"name":"ガチアクタ","year":"2025","firstAired":"2025-07-06",
		"status":{"id":1,"name":"Continuing"},"originalLanguage":"jpn","nameTranslations":["jpn","eng"]}}`
	tvdbAPI.bodies["/series/100/translations/eng"] = `{"data":{"name":"Gachiakuta"}}`
	tvdbAPI.bodies["/search?limit=5&query=Overgeared&type=series&year=2026"] = `{"data":[]}`
	tvdbAPI.bodies["/search?limit=5&query=Templar+Knight&type=series&year=2026"] = `{"data":[
		{"id":"series-101","name":"Templar Knight","year":"2026","translations":{"eng":"Templar Knight"}}]}`
	tvdbAPI.bodies["/series/101/extended"] = `{"data":{"id":101,"name":"Templar Knight","year":"2026",
		"firstAired":"2026-10-04","status":{"id":1,"name":"Continuing"},"originalLanguage":"eng"}}`
	tvdbAPI.bodies["/search?limit=5&query=Not+Yet&type=series&year=2027"] = `{"data":[
		{"id":"series-102","name":"Not Yet","year":"2027","translations":{"eng":"Not Yet"}}]}`
	tvdbAPI.bodies["/series/102/extended"] = `{"data":{"id":102,"name":"Not Yet","year":"2027",
		"firstAired":"2027-01-10","status":{"id":3,"name":"Upcoming"}}}`
	tvdbAPI.bodies["/search?limit=5&query=Nobody&type=series"] = `{"data":[]}`
	tvdbAPI.bodies["/search?limit=5&query=Pool+3&type=series&year=2025"] = `{"data":[
		{"id":"series-3","name":"Pool 3","year":"2025","translations":{"eng":"Pool 3"}}]}`
	tvdbAPI.bodies["/series/3/extended"] = `{"data":{"id":3,"name":"Pool 3","year":"2025","firstAired":"2025-01-01",
		"status":{"id":1,"name":"Continuing"},"originalLanguage":"eng"}}`

	src := newTVDBUnderTest(tvdbAPI)
	src.SetAnimeTrends(&stubAnime{trends: map[Kind][]AnimeTrend{KindSeries: {
		{Title: "Gachiakuta", Romaji: "Gachiakuta", Year: "2025", Trending: 300},
		{Title: "Overgeared", Romaji: "Templar Knight", Year: "2026", Trending: 280},
		{Title: "Not Yet", Year: "2027", Trending: 200},
		{Title: "Nobody", Trending: 150},
		{Title: "Pool 3", Year: "2025", Trending: 120},
	}}})

	results, err := src.Rank(context.Background(), KindSeries, "Anime", 6)
	require.NoError(t, err)
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	// Three trending titles lead (the cap for a six-row), then the pool minus the duplicate.
	assert.Equal(t, []string{"Gachiakuta", "Templar Knight", "Pool 3", "Pool 1", "Pool 2", "Pool 4"}, names)
	assert.Equal(t, &tvdb.Popularity{Source: "anilist", Value: 300, Detail: "AniList trending #1"},
		results[0].Popularity)
	assert.Equal(t, &tvdb.Popularity{Source: "anilist", Value: 280, Detail: "AniList trending #2"}, results[1].Popularity)
	assert.Equal(t, "tvdb", results[3].Popularity.Source, "unmeasured titles report TVDB's score")
	assert.Equal(t, 999, results[3].Popularity.Value)
	assert.True(t, results[0].Anime)
}

func TestTVDBAnimeRowSurvivesAniListFailure(t *testing.T) {
	tvdbAPI := newFakeTVDB()
	empty := filterBody(t)
	for _, region := range animeRegions {
		for _, extra := range []string{"&year=2026", "&year=2025", "&status=1"} {
			tvdbAPI.bodies[seriesURI(region.country, region.lang, 27, extra)] = empty
		}
	}
	tvdbAPI.bodies[seriesURI("usa", "eng", 27, "&year=2026")] = filterBody(t,
		seriesRec(9, "Western Toon", 1000, "2026-02-01", "2026-03-01"))
	src := newTVDBUnderTest(tvdbAPI)
	src.SetAnimeTrends(&stubAnime{err: errors.New("anilist down")})

	results, err := src.Rank(context.Background(), KindSeries, "Anime", 20)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Western Toon", results[0].Name)
}
