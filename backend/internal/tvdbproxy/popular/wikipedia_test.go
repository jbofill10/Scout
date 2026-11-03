package popular

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWikimedia serves the MediaWiki search API and the pageviews REST API
// from canned data, recording every path.
type fakeWikimedia struct {
	t         *testing.T
	srv       *httptest.Server
	mu        sync.Mutex
	paths     []string
	queries   []string            // every action API query string
	pages     map[string]string   // existing article -> "" or "disambiguation"
	redirects map[string]string   // redirect title -> target
	search    map[string][]string // srsearch -> hit titles
	views     map[string][]int    // article -> daily views
	top       map[string][]any    // "2026/11/01" -> articles
	searchHT  int
	viewsHT   int
}

func newFakeWikimedia(t *testing.T) *fakeWikimedia {
	f := &fakeWikimedia{t: t, pages: map[string]string{}, redirects: map[string]string{}, search: map[string][]string{},
		views: map[string][]int{}, top: map[string][]any{}, searchHT: http.StatusOK, viewsHT: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// page registers an existing article with daily views.
func (f *fakeWikimedia) page(article string, daily ...int) {
	f.pages[article] = ""
	f.views[article] = daily
}

func (f *fakeWikimedia) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.paths = append(f.paths, r.URL.Path)
	if r.URL.Path == "/w/api.php" {
		f.queries = append(f.queries, r.URL.RawQuery)
	}
	searchHT, viewsHT := f.searchHT, f.viewsHT
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/w/api.php" && r.URL.Query().Get("titles") != "":
		if searchHT != http.StatusOK {
			w.WriteHeader(searchHT)
			return
		}
		assert.Equal(f.t, "1", r.URL.Query().Get("redirects"))
		assert.Equal(f.t, "2", r.URL.Query().Get("formatversion"))
		var pages, redirects []map[string]any
		seen := map[string]bool{}
		for _, title := range strings.Split(r.URL.Query().Get("titles"), "|") {
			canonical := title
			if to, ok := f.redirects[title]; ok {
				redirects = append(redirects, map[string]any{"from": title, "to": to})
				canonical = to
			}
			if seen[canonical] {
				continue
			}
			seen[canonical] = true
			props, exists := f.pages[canonical]
			page := map[string]any{"title": canonical}
			if !exists {
				page["missing"] = true
			} else if props != "" {
				page["pageprops"] = map[string]string{props: ""}
			}
			pages = append(pages, page)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": pages, "redirects": redirects}})
	case r.URL.Path == "/w/api.php":
		if searchHT != http.StatusOK {
			w.WriteHeader(searchHT)
			return
		}
		assert.Equal(f.t, "query", r.URL.Query().Get("action"))
		assert.Equal(f.t, "search", r.URL.Query().Get("list"))
		hits := []map[string]string{}
		for _, title := range f.search[r.URL.Query().Get("srsearch")] {
			hits = append(hits, map[string]string{"title": title})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"search": hits}})
	case strings.Contains(r.URL.Path, "/pageviews/per-article/"):
		if viewsHT != http.StatusOK {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(viewsHT)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		// .../user/{article}/daily/{start}/{end}
		article := strings.ReplaceAll(parts[len(parts)-4], "_", " ")
		daily, ok := f.views[article]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		items := []map[string]int{}
		for _, v := range daily {
			items = append(items, map[string]int{"views": v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	case strings.Contains(r.URL.Path, "/pageviews/top/"):
		day := strings.TrimPrefix(r.URL.Path, "/api/rest_v1/metrics/pageviews/top/en.wikipedia/all-access/")
		articles, ok := f.top[day]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"articles": articles}}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeWikimedia) count(substr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, p := range f.paths {
		if strings.Contains(p, substr) {
			n++
		}
	}
	return n
}

// countQueries counts action API requests whose query string holds substr.
func (f *fakeWikimedia) countQueries(substr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, q := range f.queries {
		if strings.Contains(q, substr) {
			n++
		}
	}
	return n
}

// measureOne is a convenience for single-title tests.
func measureOne(w *Wikipedia, kind Kind, name, year string) Measurement {
	return w.Measure(context.Background(), kind, []Title{{Name: name, Year: year}})[0]
}

func topEntry(article string, views int) any {
	return map[string]any{"article": article, "views": views}
}

func newWikipediaUnderTest(f *fakeWikimedia) *Wikipedia {
	w := NewWikipedia(WikipediaOptions{APIHost: f.srv.URL, RESTHost: f.srv.URL}, f.srv.Client(), slog.Default())
	w.now = func() time.Time { return today }
	w.sleep = func(time.Duration) {}
	return w
}

func TestWikipediaMeasureResolvesInBulkAndFollowsRedirects(t *testing.T) {
	f := newFakeWikimedia(t)
	f.page("Moana (2026 film)", 1000, 2000, 3000)
	f.redirects["Fallout (2024 TV series)"] = "Fallout (American TV series)"
	f.page("Fallout (American TV series)", 500, 500)
	f.pages["Moana (film)"] = "disambiguation"
	w := newWikipediaUnderTest(f)

	got := w.Measure(context.Background(), KindMovie,
		[]Title{{Name: "Moana", Year: "2026"}, {Name: "Nothing", Year: "2026"}})
	assert.Equal(t, Measurement{Article: "Moana (2026 film)", Views: 6000, Found: true}, got[0])
	assert.Equal(t, Measurement{}, got[1], "no article, no search hit")
	assert.Equal(t, 1, f.countQueries("titles="), "one bulk lookup for the whole row")
	assert.Equal(t, 1, f.countQueries("list=search"), "only the unresolved title is searched")

	got = w.Measure(context.Background(), KindSeries, []Title{{Name: "Fallout", Year: "2024"}})
	assert.Equal(t, Measurement{Article: "Fallout (American TV series)", Views: 1000, Found: true}, got[0],
		"a redirect is measured on its target, not on the redirect page")

	// Articles and views are cached: measuring again costs nothing.
	before := len(f.paths)
	w.Measure(context.Background(), KindMovie, []Title{{Name: "Moana", Year: "2026"}})
	w.Measure(context.Background(), KindSeries, []Title{{Name: "Fallout", Year: "2024"}})
	assert.Equal(t, before, len(f.paths))

	// The pageviews range is the seven days ending yesterday.
	f.mu.Lock()
	var pv string
	for _, p := range f.paths {
		if strings.Contains(p, "/per-article/") {
			pv = p
		}
	}
	f.mu.Unlock()
	assert.True(t, strings.HasSuffix(pv, "/daily/20261026/20261101"), pv)
}

func TestWikipediaMeasureSkipsDisambiguationAndFallsBackToSearch(t *testing.T) {
	f := newFakeWikimedia(t)
	// "Paradise (film)" is a disambiguation page; the real article has a form
	// the guesses do not cover, so the search finds it. TVDB's "(2025)"
	// disambiguator is dropped first.
	f.pages["Paradise (film)"] = "disambiguation"
	f.search["Paradise 2025 television series"] = []string{"Paradise Lost", "Paradise (2025 American TV series)"}
	f.page("Paradise (2025 American TV series)", 10)
	// Thunderbolts* has a plain article title and matches despite punctuation.
	f.search["Thunderbolts* 2025 film"] = []string{"Thunderbolts*"}
	f.page("Thunderbolts*", 5)
	// A search whose hits are other works is a miss.
	f.search["Scarlet 2025 film"] = []string{"Scarlett Johansson", "Scarlet (comics)"}
	w := newWikipediaUnderTest(f)

	got := measureOne(w, KindSeries, "Paradise (2025)", "2025")
	assert.Equal(t, Measurement{Article: "Paradise (2025 American TV series)", Views: 10, Found: true}, got)

	got = measureOne(w, KindMovie, "Thunderbolts*", "2025")
	assert.Equal(t, "Thunderbolts*", got.Article)

	assert.False(t, measureOne(w, KindMovie, "Scarlet", "2025").Found)
	assert.False(t, measureOne(w, KindMovie, "Scarlet", "2025").Found)
	assert.Equal(t, 3, f.countQueries("list=search"), "misses are cached too")
}

func TestWikipediaRateLimitsAreRetriedOnce(t *testing.T) {
	f := newFakeWikimedia(t)
	f.search["Scream 7 2026 film"] = []string{"Scream 7"}
	f.page("Scream 7", 7)
	f.searchHT = http.StatusTooManyRequests
	f.viewsHT = http.StatusTooManyRequests
	w := newWikipediaUnderTest(f)
	var (
		mu    sync.Mutex
		slept []time.Duration
	)
	w.sleep = func(d time.Duration) {
		mu.Lock()
		slept = append(slept, d)
		mu.Unlock()
		// Each API recovers after its own retry pause.
		f.mu.Lock()
		if d == retryWait {
			f.searchHT = http.StatusOK
		} else {
			f.viewsHT = http.StatusOK
		}
		f.mu.Unlock()
	}

	got := measureOne(w, KindMovie, "Scream 7", "2026")
	assert.Equal(t, Measurement{Article: "Scream 7", Views: 7, Found: true}, got)
	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, slept, retryWait, "the action API retry waits the default")
	assert.Contains(t, slept, time.Second, "the REST retry honours Retry-After")
}

func TestWikipediaPacesRESTRequests(t *testing.T) {
	f := newFakeWikimedia(t)
	f.page("A (film)", 1)
	f.page("B (film)", 2)
	f.page("C (film)", 3)
	w := newWikipediaUnderTest(f)
	var slept atomic.Int64
	w.sleep = func(d time.Duration) { slept.Add(int64(d)) }

	w.Measure(context.Background(), KindMovie, []Title{{Name: "A"}, {Name: "B"}, {Name: "C"}})
	assert.GreaterOrEqual(t, time.Duration(slept.Load()), 2*restInterval, "three REST requests are spaced out")
}

func TestArticleGuesses(t *testing.T) {
	assert.Equal(t, []string{"Moana (2026 film)", "Moana (film)"}, articleGuesses(KindMovie, "Moana", "2026"))
	assert.Equal(t, []string{"F1 (film)"}, articleGuesses(KindMovie, "F1", ""))
	assert.Equal(t, []string{"Fallout (2024 TV series)", "Fallout (TV series)", "Fallout (American TV series)"},
		articleGuesses(KindSeries, "Fallout", "2024"))
}

func TestWikipediaArticleViewsMissIsCached(t *testing.T) {
	f := newFakeWikimedia(t)
	w := newWikipediaUnderTest(f)
	_, ok := w.ArticleViews(context.Background(), "Nothing Here")
	assert.False(t, ok)
	_, ok = w.ArticleViews(context.Background(), "Nothing Here")
	assert.False(t, ok)
	assert.Equal(t, 1, f.count("/per-article/"))
}

func TestWikipediaLookupFailureIsNotCached(t *testing.T) {
	f := newFakeWikimedia(t)
	f.searchHT = http.StatusServiceUnavailable
	w := newWikipediaUnderTest(f)
	assert.False(t, measureOne(w, KindMovie, "Toy Story 5", "2026").Found)
	f.mu.Lock()
	f.searchHT = http.StatusOK
	f.search["Toy Story 5 2026 film"] = []string{"Toy Story 5"}
	f.mu.Unlock()
	f.page("Toy Story 5", 1)
	assert.True(t, measureOne(w, KindMovie, "Toy Story 5", "2026").Found,
		"a transient failure is retried on the next measurement")
}

func TestWikipediaTrendingParsesFilmAndSeriesArticles(t *testing.T) {
	f := newFakeWikimedia(t)
	f.top["2026/11/01"] = []any{
		topEntry("Main_Page", 5000000),
		topEntry("Obsession_(2025_film)", 91786),
		topEntry("Clayface_(film)", 83109),
		topEntry("Nocturne_(TV_series)", 59441),
		topEntry("Fallout_(American_TV_series)", 20000),
		topEntry("Stranger_Things_season_5", 18000),
		topEntry("Wednesday_(season_2)", 17000),
		topEntry("List_of_Marvel_Cinematic_Universe_films", 16000),
		topEntry("Wicker_(2026_American_film)", 15000),
		topEntry("Deaths_in_2026", 14000),
	}
	w := newWikipediaUnderTest(f)

	movies := w.Trending(context.Background(), KindMovie)
	assert.Equal(t, []TrendingArticle{
		{Article: "Obsession (2025 film)", Title: "Obsession", Year: "2025", Views: 91786},
		{Article: "Clayface (film)", Title: "Clayface", Views: 83109},
		{Article: "Wicker (2026 American film)", Title: "Wicker", Year: "2026", Views: 15000},
	}, movies)

	series := w.Trending(context.Background(), KindSeries)
	assert.Equal(t, []TrendingArticle{
		{Article: "Nocturne (TV series)", Title: "Nocturne", Views: 59441},
		{Article: "Fallout (American TV series)", Title: "Fallout", Views: 20000},
		{Article: "Stranger Things season 5", Title: "Stranger Things", Views: 18000},
		{Article: "Wednesday (season 2)", Title: "Wednesday", Views: 17000},
	}, series)

	w.Trending(context.Background(), KindSeries)
	assert.Equal(t, 2, f.count("/pageviews/top/"), "one fetch per kind, then cached")
}

func TestWikipediaTrendingFallsBackToTheDayBefore(t *testing.T) {
	f := newFakeWikimedia(t)
	f.top["2026/10/31"] = []any{topEntry("Verity_(film)", 56051)}
	w := newWikipediaUnderTest(f)

	movies := w.Trending(context.Background(), KindMovie)
	require.Len(t, movies, 1)
	assert.Equal(t, "Verity", movies[0].Title)
	assert.Equal(t, 2, f.count("/pageviews/top/"))
}

func TestArticleMatches(t *testing.T) {
	assert.True(t, articleMatches("F1 (film)", "F1"))
	assert.True(t, articleMatches("Chainsaw Man – The Movie: Reze Arc", "Chainsaw Man – The Movie: Reze Arc"))
	assert.True(t, articleMatches("A Knight of the Seven Kingdoms (TV series)", "A Knight of the Seven Kingdoms"))
	assert.False(t, articleMatches("Moana 2", "Moana"))
	assert.False(t, articleMatches("Michael Pitt", "The Pitt"))
	assert.Equal(t, "His & Hers", cleanTitle("His & Hers (2026)"))
	assert.Equal(t, "Scream 7", cleanTitle("Scream 7"))
}
