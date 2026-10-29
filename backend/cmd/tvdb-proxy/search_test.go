package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchFixture is a fake TVDB that answers /search, /search/remoteid,
// /people/{id}/extended and the per-series episode endpoint from canned data,
// recording every request so tests can assert on what the proxy asked for.
type searchFixture struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	requests []*url.URL
	// search hits keyed by "type|query"; a missing key answers with no hits
	hits map[string][]map[string]any
	// people extended records keyed by person id
	people map[string]map[string]any
	// remote id hits keyed by id
	remote map[string][]map[string]any
	// series scores keyed by series id, for the episodes endpoint
	scores map[string]float64
	// peopleSearchFails makes type=person searches answer 500
	peopleSearchFails bool
}

func newSearchFixture(t *testing.T) *searchFixture {
	f := &searchFixture{
		t:      t,
		hits:   map[string][]map[string]any{},
		people: map[string]map[string]any{},
		remote: map[string][]map[string]any{},
		scores: map[string]float64{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	peopleSearchCache.clear()
	peopleCreditsCache.clear()
	t.Cleanup(func() {
		peopleSearchCache.clear()
		peopleCreditsCache.clear()
	})
	useFakeTVDB(t, &fakeTVDB{srv: f.srv}, "token")
	return f
}

func (f *searchFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL)
	f.mu.Unlock()

	write := func(v any) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(f.t, json.NewEncoder(w).Encode(v))
	}
	path := r.URL.Path
	switch {
	case path == "/search":
		typ := r.URL.Query().Get("type")
		if typ == "person" && f.peopleSearchFails {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		hits := f.hits[typ+"|"+r.URL.Query().Get("query")]
		if hits == nil {
			hits = []map[string]any{}
		}
		write(map[string]any{"data": hits})
	case strings.HasPrefix(path, "/search/remoteid/"):
		hits := f.remote[strings.TrimPrefix(path, "/search/remoteid/")]
		if hits == nil {
			hits = []map[string]any{}
		}
		write(map[string]any{"data": hits})
	case strings.HasPrefix(path, "/people/"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/people/"), "/extended")
		person, ok := f.people[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		write(map[string]any{"data": person})
	case strings.HasPrefix(path, "/series/") && strings.HasSuffix(path, "/episodes/official/eng"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/series/"), "/episodes/official/eng")
		write(map[string]any{"data": map[string]any{
			"score":    f.scores[id],
			"episodes": []map[string]any{{"id": 1, "seasonNumber": 1, "number": 1, "name": "Pilot"}},
		}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// searchRequests returns the /search calls made, as their query values.
func (f *searchFixture) searchRequests() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []url.Values
	for _, u := range f.requests {
		if u.Path == "/search" {
			out = append(out, u.Query())
		}
	}
	return out
}

func (f *searchFixture) requestPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	paths := make([]string, 0, len(f.requests))
	for _, u := range f.requests {
		paths = append(paths, u.Path)
	}
	return paths
}

func movieHit(id, name, year string, extra map[string]any) map[string]any {
	hit := map[string]any{
		"id": "movie-" + id, "tvdb_id": id, "type": "movie", "name": name, "year": year,
		"translations": map[string]string{"eng": name},
		"image_url":    "/banners/movies/" + id + ".jpg",
	}
	for k, v := range extra {
		hit[k] = v
	}
	return hit
}

func seriesHit(id, name, year string, extra map[string]any) map[string]any {
	hit := map[string]any{
		"id": "series-" + id, "tvdb_id": id, "type": "series", "name": name, "year": year,
		"translations": map[string]string{"eng": name},
	}
	for k, v := range extra {
		hit[k] = v
	}
	return hit
}

func personHit(id, name string) map[string]any {
	return map[string]any{"id": "person-" + id, "tvdb_id": id, "type": "person", "name": name}
}

func movieCredit(movieID int, name, year, role string) map[string]any {
	return map[string]any{
		"peopleType": role, "movieId": movieID, "seriesId": nil,
		"movie": map[string]any{"name": name, "year": year, "image": "/banners/movies/" + name + ".jpg"},
	}
}

func seriesCredit(seriesID int, name, year, role string) map[string]any {
	return map[string]any{
		"peopleType": role, "seriesId": seriesID, "movieId": nil,
		"series": map[string]any{"name": name, "year": year, "image": "/banners/series/" + name + ".jpg"},
	}
}

func names(results []tvdb.Media) []string {
	out := make([]string, len(results))
	for idx, media := range results {
		out[idx] = media.Name
	}
	return out
}

var now = time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)

func query(raw string) search.Query {
	return search.Parse(raw, now)
}

func TestSearchMedia_YearFilterIsPassedToTVDBAndExactTitleRanksFirst(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["movie|X"] = []map[string]any{
		movieHit("1", "Rubius X", "2022", nil),
		movieHit("2", "X", "2022", nil),
		movieHit("3", "Adonis X", "2022", nil),
	}

	results, err := searchMedia(context.Background(), query("X (2022)"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"X", "Rubius X", "Adonis X"}, names(results))

	calls := f.searchRequests()
	require.Len(t, calls, 2, "one title search plus one people lookup")
	assert.Equal(t, "X", calls[0].Get("query"))
	assert.Equal(t, "movie", calls[0].Get("type"))
	assert.Equal(t, "2022", calls[0].Get("year"))
	assert.Equal(t, "person", calls[1].Get("type"))
}

func TestSearchMedia_EncodesTitleProperly(t *testing.T) {
	f := newSearchFixture(t)

	_, err := searchMedia(context.Background(), query("Mission: Impossible & more"), "movie")

	require.NoError(t, err)
	calls := f.searchRequests()
	require.NotEmpty(t, calls)
	assert.Equal(t, "Mission: Impossible & more", calls[0].Get("query"), "spaces and symbols survive the round trip")
}

func TestSearchMedia_TrailingYearAlsoSearchesVerbatimTitle(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["movie|Class of"] = []map[string]any{movieHit("1", "Class of 1984", "1982", nil)}
	f.hits["movie|Class of 1984"] = []map[string]any{
		movieHit("1", "Class of 1984", "1982", nil),
		movieHit("2", "Class of 1999", "1990", nil),
	}

	results, err := searchMedia(context.Background(), query("Class of 1984"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"Class of 1984", "Class of 1999"}, names(results), "merged and de-duplicated")

	calls := f.searchRequests()
	require.GreaterOrEqual(t, len(calls), 2)
	assert.Equal(t, "1984", calls[0].Get("year"))
	assert.Equal(t, "Class of 1984", calls[1].Get("query"))
	assert.Empty(t, calls[1].Get("year"), "the verbatim search carries no year filter")
}

func TestSearchMedia_ExactPersonNameAddsCreditsAfterExactTitles(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["movie|Mia Goth"] = []map[string]any{movieHit("9", "Mia Goth: A Portrait", "2023", nil)}
	f.hits["person|Mia Goth"] = []map[string]any{personHit("8159739", "Mia Goth"), personHit("1", "Mia Gothier")}
	f.people["8159739"] = map[string]any{
		"id": 8159739, "name": "Mia Goth",
		"characters": []map[string]any{
			movieCredit(259, "Suspiria", "2018", "Actor"),
			movieCredit(154561, "X", "2022", "Actor"),
			movieCredit(154561, "X", "2022", "Producer"), // same title, second role
			movieCredit(338580, "Pearl", "2022", "Writer"),
			seriesCredit(100, "Some Show", "2020", "Actor"),
		},
	}

	results, err := searchMedia(context.Background(), query("Mia Goth"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"X", "Suspiria", "Mia Goth: A Portrait"}, names(results),
		"acting credits (newest first) come before the loose title hit; writer-only credits are skipped")
	assert.Equal(t, "Mia Goth · Actor", results[0].MatchedVia)
	assert.Equal(t, "154561", results[0].Id)
	assert.Equal(t, "https://artworks.thetvdb.com/banners/movies/X.jpg", results[0].ImageUrl)
	assert.Equal(t, "movie", results[0].Category)
	assert.Empty(t, results[2].MatchedVia)
	assert.Contains(t, f.requestPaths(), "/people/8159739/extended")
	assert.NotContains(t, f.requestPaths(), "/people/1/extended", "only the exact name is expanded")
}

func TestSearchMedia_InexactPersonNameIsNotExpanded(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["movie|Pearl"] = []map[string]any{movieHit("1", "Pearl", "2022", nil)}
	f.hits["person|Pearl"] = []map[string]any{personHit("5", "Pearl Bailey")}

	results, err := searchMedia(context.Background(), query("Pearl"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"Pearl"}, names(results))
	assert.NotContains(t, f.requestPaths(), "/people/5/extended")
}

func TestSearchMedia_ExplicitActorReturnsOnlyCreditsNarrowedByTitleAndYear(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["movie|X"] = []map[string]any{movieHit("99", "Malcolm X", "1992", nil)}
	f.hits["person|mia goth"] = []map[string]any{personHit("7", "Mia Goth")}
	f.people["7"] = map[string]any{
		"id": 7, "name": "Mia Goth",
		"characters": []map[string]any{
			movieCredit(1, "X", "2022", "Actor"),
			movieCredit(2, "MaXXXine", "2024", "Actor"),
			movieCredit(3, "Pearl", "2022", "Actor"),
			movieCredit(4, "X-Men Origins", "2022", "Guest Star"),
			movieCredit(5, "Nix", "2022", "Producer"),
		},
	}

	results, err := searchMedia(context.Background(), query("X actor:mia goth year:2022"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"X", "X-Men Origins"}, names(results),
		"title text narrows credits by substring, the year by equality, guest spots rank last")
	for _, call := range f.searchRequests() {
		assert.Equal(t, "person", call.Get("type"), "no title search is made for an explicit person")
	}
}

func TestSearchMedia_ExplicitPersonTakesTopHitEvenWhenNameDiffers(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["person|goth"] = []map[string]any{personHit("3", "Van Goth"), personHit("7", "Mia Goth")}
	f.people["3"] = map[string]any{"id": 3, "name": "Van Goth", "characters": []map[string]any{
		movieCredit(1, "Sunflowers", "1999", "Actor"),
	}}

	results, err := searchMedia(context.Background(), query("actor:goth"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"Sunflowers"}, names(results))
	assert.Equal(t, "Van Goth · Actor", results[0].MatchedVia)
	assert.NotContains(t, f.requestPaths(), "/people/7/extended")
}

func TestSearchMedia_PersonFilterKeepsEveryRole(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["person|Ti West"] = []map[string]any{personHit("555922", "Ti West")}
	f.people["555922"] = map[string]any{"id": 555922, "name": "Ti West", "characters": []map[string]any{
		movieCredit(1, "You're Next", "2011", "Actor"),
		movieCredit(2, "X", "2022", "Director"),
		movieCredit(3, "Pearl", "2022", "Writer"),
	}}

	results, err := searchMedia(context.Background(), query("person:Ti West"), "movie")

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"You're Next", "X", "Pearl"}, names(results))
}

func TestSearchMedia_DirectorOnMoviesUsesTVDBFilter(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["movie|X"] = []map[string]any{movieHit("1", "X", "2022", map[string]any{"director": "Ti West"})}

	results, err := searchMedia(context.Background(), query("X director:Ti West"), "movie")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Ti West", results[0].Director)
	calls := f.searchRequests()
	assert.Equal(t, "Ti West", calls[0].Get("director"))
	assert.NotContains(t, f.requestPaths(), "/people/555922/extended")
}

func TestSearchMedia_DirectorOnSeriesUsesCredits(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["person|David Lynch"] = []map[string]any{personHit("1", "David Lynch")}
	f.people["1"] = map[string]any{"id": 1, "name": "David Lynch", "characters": []map[string]any{
		seriesCredit(10, "Twin Peaks", "1990", "Director"),
		seriesCredit(11, "Louie", "2010", "Guest Star"),
		movieCredit(12, "Blue Velvet", "1986", "Director"),
	}}
	f.scores["10"] = 500

	results, err := searchMedia(context.Background(), query("director:David Lynch"), "series")

	require.NoError(t, err)
	assert.Equal(t, []string{"Twin Peaks"}, names(results))
	assert.Equal(t, "David Lynch · Director", results[0].MatchedVia)
	assert.Len(t, results[0].Metadata.Episodes, 1, "series credits get their episodes like any series hit")
	assert.Equal(t, float64(500), results[0].Score)
	for _, call := range f.searchRequests() {
		assert.Empty(t, call.Get("director"), "TVDB's director filter does not work for series")
	}
}

func TestSearchMedia_DirectorWithoutTitleUsesCreditsForMovies(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["person|Ti West"] = []map[string]any{personHit("555922", "Ti West")}
	f.people["555922"] = map[string]any{"id": 555922, "name": "Ti West", "characters": []map[string]any{
		movieCredit(1, "You're Next", "2011", "Actor"),
		movieCredit(2, "X", "2022", "Director"),
		movieCredit(3, "Pearl", "2022", "Director"),
		movieCredit(4, "MaXXXine", "2024", "Director"),
	}}

	results, err := searchMedia(context.Background(), query("director:Ti West"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"MaXXXine", "X", "Pearl"}, names(results), "directing credits only, newest first")
}

func TestSearchMedia_StudioFiltersMovieHitsLocally(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["movie|X"] = []map[string]any{
		movieHit("1", "Malcolm X", "1992", map[string]any{"studios": []string{"Warner Bros."}}),
		movieHit("2", "X", "2022", map[string]any{"studios": []string{"A24", "Bron Studios"}}),
		movieHit("3", "Pearl", "2022", map[string]any{"studios": []string{"a24 films"}}),
		movieHit("4", "Saw X", "2023", nil),
	}

	results, err := searchMedia(context.Background(), query("X studio:A24"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"X", "Pearl"}, names(results))
	assert.Equal(t, []string{"A24", "Bron Studios"}, results[0].Studios)
	assert.Empty(t, f.searchRequests()[0].Get("company"), "TVDB's company filter does not cover studios")
}

func TestSearchMedia_NetworkIsPassedThroughForSeries(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["series|Succession"] = []map[string]any{
		seriesHit("1", "Succession", "2018", map[string]any{"network": "HBO"}),
	}

	results, err := searchMedia(context.Background(), query("Succession network:HBO"), "series")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "HBO", results[0].Network)
	assert.Equal(t, "HBO", f.searchRequests()[0].Get("network"))
}

func TestSearchMedia_CountryAndLanguagePassThrough(t *testing.T) {
	f := newSearchFixture(t)

	_, err := searchMedia(context.Background(), query("Dark country:deu lang:deu"), "series")

	require.NoError(t, err)
	call := f.searchRequests()[0]
	assert.Equal(t, "deu", call.Get("country"))
	assert.Equal(t, "deu", call.Get("language"))
}

func TestSearchMedia_SeriesHitsGetEpisodesAndSortByScoreWithinTier(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["series|Office"] = []map[string]any{
		seriesHit("1", "The Office (UK)", "2001", nil),
		seriesHit("2", "The Office (US)", "2005", nil),
		seriesHit("3", "Office", "2010", nil),
	}
	f.scores["1"] = 100
	f.scores["2"] = 900
	f.scores["3"] = 1

	results, err := searchMedia(context.Background(), query("Office"), "series")

	require.NoError(t, err)
	assert.Equal(t, []string{"Office", "The Office (US)", "The Office (UK)"}, names(results),
		"the exact match leads, then the rest by popularity")
	for _, media := range results {
		assert.Len(t, media.Metadata.Episodes, 1)
	}
}

func TestSearchMedia_ImdbIDUsesRemoteLookup(t *testing.T) {
	f := newSearchFixture(t)
	f.remote["tt13560574"] = []map[string]any{{
		"movie": map[string]any{"id": 154561, "name": "X", "year": "2022", "slug": "x-2022", "image": "/x.jpg", "score": 5},
	}}

	results, err := searchMedia(context.Background(), query("https://www.imdb.com/title/tt13560574/"), "movie")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "154561", results[0].Id)
	assert.Equal(t, "X", results[0].Name)
	assert.Equal(t, "tt13560574", results[0].ImdbID)
	assert.Equal(t, "https://artworks.thetvdb.com/x.jpg", results[0].ImageUrl)
	assert.Equal(t, []string{"/search/remoteid/tt13560574"}, f.requestPaths())
}

func TestSearchMedia_ImdbIDOfOtherTypeReturnsNothing(t *testing.T) {
	f := newSearchFixture(t)
	f.remote["tt13560574"] = []map[string]any{{
		"movie": map[string]any{"id": 154561, "name": "X", "year": "2022"},
	}}

	results, err := searchMedia(context.Background(), query("tt13560574"), "series")

	require.NoError(t, err)
	assert.Empty(t, results)
	assert.NotNil(t, results, "serialises as [] rather than null")
}

func TestSearchMedia_FiltersWithoutTitleReturnNothing(t *testing.T) {
	f := newSearchFixture(t)

	results, err := searchMedia(context.Background(), query("year:2022"), "movie")

	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Empty(t, f.requestPaths(), "nothing to ask TVDB")
}

func TestSearchMedia_PeopleLookupFailureKeepsTitleHits(t *testing.T) {
	f := newSearchFixture(t)
	f.peopleSearchFails = true
	f.hits["movie|X"] = []map[string]any{movieHit("1", "X", "2022", nil)}

	results, err := searchMedia(context.Background(), query("X"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"X"}, names(results))
}

func TestSearchMedia_TitleSearchFailureIsAnError(t *testing.T) {
	f := newSearchFixture(t)
	f.srv.Close()

	_, err := searchMedia(context.Background(), query("X"), "movie")

	assert.Error(t, err)
}

func TestSearchMedia_PeopleLookupsAreCached(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["person|Mia Goth"] = []map[string]any{personHit("7", "Mia Goth")}
	f.people["7"] = map[string]any{"id": 7, "name": "Mia Goth", "characters": []map[string]any{
		movieCredit(1, "X", "2022", "Actor"),
	}}

	_, err := searchMedia(context.Background(), query("actor:Mia Goth"), "movie")
	require.NoError(t, err)
	_, err = searchMedia(context.Background(), query("actor:mia goth"), "movie")
	require.NoError(t, err)

	paths := f.requestPaths()
	assert.Equal(t, 1, countOf(paths, "/search"), "second search reuses the cached people hits")
	assert.Equal(t, 1, countOf(paths, "/people/7/extended"))
}

func TestSearchMedia_CapsResultsAndCreditsPerPerson(t *testing.T) {
	f := newSearchFixture(t)
	var credits []map[string]any
	for i := 0; i < maxCreditsPerPerson+10; i++ {
		credits = append(credits, movieCredit(i+1, "Film "+string(rune('A'+i%26)), "2000", "Actor"))
	}
	f.hits["person|Prolific Star"] = []map[string]any{personHit("1", "Prolific Star")}
	f.people["1"] = map[string]any{"id": 1, "name": "Prolific Star", "characters": credits}

	results, err := searchMedia(context.Background(), query("actor:Prolific Star"), "movie")

	require.NoError(t, err)
	assert.Len(t, results, maxCreditsPerPerson)
}

func featured(credit map[string]any) map[string]any {
	credit["isFeatured"] = true
	return credit
}

func TestSearchMedia_ImplicitLookupRanksMainActingWorkBeforeSideDirecting(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["person|Bryan Cranston"] = []map[string]any{personHit("255211", "Bryan Cranston")}
	f.people["255211"] = map[string]any{"id": 255211, "name": "Bryan Cranston", "characters": []map[string]any{
		seriesCredit(95011, "Modern Family", "2009", "Director"),
		seriesCredit(81189, "Breaking Bad", "2008", "Director"),
		featured(seriesCredit(81189, "Breaking Bad", "2008", "Actor")),
		featured(seriesCredit(73838, "Malcolm in the Middle", "2000", "Actor")),
		seriesCredit(300, "Seinfeld", "1989", "Guest Star"),
		seriesCredit(301, "Some Cameo", "2015", "Actor"),
	}}
	f.scores["95011"] = 900
	f.scores["81189"] = 800
	f.scores["73838"] = 100
	f.scores["301"] = 50

	results, err := searchMedia(context.Background(), query("Bryan Cranston"), "series")

	require.NoError(t, err)
	assert.Equal(t, []string{"Breaking Bad", "Malcolm in the Middle", "Some Cameo", "Modern Family"}, names(results),
		"main cast by popularity, then supporting roles, then episodes he directed; guest spots are not implied")
	assert.Equal(t, "Bryan Cranston · Actor", results[0].MatchedVia, "the strongest role on a title is the one shown")
}

func TestSearchMedia_ImplicitLookupOfADirectorLeadsWithTheirFilms(t *testing.T) {
	f := newSearchFixture(t)
	f.hits["person|Ti West"] = []map[string]any{personHit("555922", "Ti West")}
	f.people["555922"] = map[string]any{"id": 555922, "name": "Ti West", "characters": []map[string]any{
		featured(movieCredit(2933, "You're Next", "2011", "Actor")),
		movieCredit(154561, "X", "2022", "Director"),
		movieCredit(154561, "X", "2022", "Writer"),
		movieCredit(338580, "Pearl", "2022", "Director"),
		movieCredit(340093, "MaXXXine", "2024", "Director"),
	}}

	results, err := searchMedia(context.Background(), query("Ti West"), "movie")

	require.NoError(t, err)
	assert.Equal(t, []string{"MaXXXine", "X", "Pearl", "You're Next"}, names(results),
		"directing dominates his credits, so the cameo comes last")
	assert.Equal(t, "Ti West · Director", results[1].MatchedVia)
}

func countOf(items []string, want string) int {
	n := 0
	for _, item := range items {
		if item == want {
			n++
		}
	}
	return n
}

func TestSearchItemToMedia_MapsDirectorStudiosNetwork(t *testing.T) {
	media, ok := searchItemToMedia(tvdb.TVDBSearchItem{
		Id: "movie-154561", Category: "movie", Director: "Ti West", Studios: []string{"A24"},
		Country: "usa", PrimaryLanguage: "eng", Genres: []string{"Horror"},
	})

	require.True(t, ok)
	assert.Equal(t, "Ti West", media.Director)
	assert.Equal(t, []string{"A24"}, media.Studios)
	assert.Equal(t, "usa", media.OriginalCountry)
	assert.Equal(t, "eng", media.OriginalLanguage)
	assert.Equal(t, []string{"Horror"}, media.Genres)
	assert.NotNil(t, media.Metadata.Episodes)

	show, ok := searchItemToMedia(tvdb.TVDBSearchItem{Id: "series-1", Category: "series", Network: "HBO"})
	require.True(t, ok)
	assert.Equal(t, "HBO", show.Network)
}

func TestSearchItemToMedia_FallsBackToOriginalName(t *testing.T) {
	media, ok := searchItemToMedia(tvdb.TVDBSearchItem{Id: "movie-1", Category: "movie", OriginalName: "Les Misérables"})

	require.True(t, ok)
	assert.Equal(t, "Les Misérables", media.Name)
}
