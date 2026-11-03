package popular

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Attention measures how much a title is being looked at right now. The
// TVDB source uses it to order its recent-release pool by current interest
// and to pull in titles that are trending but that TVDB users have not
// scored yet.
type Attention interface {
	// Measure returns each title's attention over the last week, in the
	// order given; a title with no article is reported as not found.
	Measure(ctx context.Context, kind Kind, titles []Title) []Measurement
	// ArticleViews returns the attention of a known article.
	ArticleViews(ctx context.Context, article string) (int, bool)
	// Trending lists film or TV articles among the most read of the last
	// day, with the release year when the article title carries one.
	Trending(ctx context.Context, kind Kind) []TrendingArticle
}

// Title names a work to measure.
type Title struct {
	Name string
	Year string
}

// Measurement is one title's attention.
type Measurement struct {
	Article string
	Views   int
	Found   bool
}

// TrendingArticle is one entry of Wikipedia's daily most-read list that
// names a film or a TV series.
type TrendingArticle struct {
	Article string // Wikipedia article title, e.g. "Obsession (2025 film)"
	Title   string // the work's title, e.g. "Obsession"
	Year    string // release year from the article title, "" when absent
	Views   int    // views on the day of the list
}

const (
	// DefaultWikipediaAPIHost serves the MediaWiki action API.
	DefaultWikipediaAPIHost = "https://en.wikipedia.org"
	// DefaultWikimediaRESTHost serves the pageviews API.
	DefaultWikimediaRESTHost = "https://wikimedia.org"

	wikipediaTimeout = 15 * time.Second
	// restConcurrency and restInterval pace pageviews requests: the analytics
	// API answers bursts with 429, so at most two run at once and they start
	// no closer together than restInterval.
	restConcurrency = 2
	restInterval    = 100 * time.Millisecond
	// actionConcurrency bounds MediaWiki action API requests (title lookups
	// and searches): the API asks clients to make them in series.
	actionConcurrency = 1
	// retryWait is how long a rate-limited request waits before its one
	// retry when the answer names no Retry-After.
	retryWait = 2 * time.Second
	// maxRetryWait caps a Retry-After.
	maxRetryWait = 10 * time.Second
	// titlesPerQuery is how many article titles one action API query may check.
	titlesPerQuery = 50
	// viewsWindowDays is the span pageviews are summed over.
	viewsWindowDays = 7
	// viewsTTL is how long a title's pageviews are kept; the daily figures
	// move slowly, and the rows are rebuilt hourly anyway.
	viewsTTL = 6 * time.Hour
	// articleTTL is how long a resolved (or unresolved) article title is kept.
	articleTTL = 24 * time.Hour
	// trendingTTL is how long a day's most-read list is kept.
	trendingTTL = 6 * time.Hour
	// trendingMax caps how many trending titles are harvested per kind.
	trendingMax = 40
)

// WikipediaOptions configures the Wikipedia attention source.
type WikipediaOptions struct {
	// APIHost serves /w/api.php; empty means DefaultWikipediaAPIHost.
	APIHost string
	// RESTHost serves /api/rest_v1/metrics/pageviews; empty means DefaultWikimediaRESTHost.
	RESTHost string
}

// Wikipedia measures attention as English Wikipedia pageviews: a title is
// resolved to its article with the action API (bulk title lookup, then
// search), and the article's views over the last week are summed from the
// pageviews API.
type Wikipedia struct {
	opts      WikipediaOptions
	http      *http.Client
	articles  *ttlCache[string]            // (kind, name, year) -> article, "" for a miss
	views     *ttlCache[int]               // article -> views, -1 for a miss
	trending  *ttlCache[[]TrendingArticle] // kind -> harvested list
	restSem   chan struct{}
	actionSem chan struct{}
	paceMu    sync.Mutex
	nextREST  time.Time
	sleep     func(time.Duration)
	now       func() time.Time
	logger    *slog.Logger
}

// NewWikipediaHTTPClient returns the traced HTTP client used for Wikimedia requests.
func NewWikipediaHTTPClient() *http.Client {
	return &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: wikipediaTimeout}
}

// NewWikipedia returns an attention source backed by the Wikimedia APIs.
func NewWikipedia(opts WikipediaOptions, httpClient *http.Client, logger *slog.Logger) *Wikipedia {
	if opts.APIHost == "" {
		opts.APIHost = DefaultWikipediaAPIHost
	}
	if opts.RESTHost == "" {
		opts.RESTHost = DefaultWikimediaRESTHost
	}
	opts.APIHost = strings.TrimRight(opts.APIHost, "/")
	opts.RESTHost = strings.TrimRight(opts.RESTHost, "/")
	return &Wikipedia{
		opts:      opts,
		http:      httpClient,
		articles:  newTTLCache[string](),
		views:     newTTLCache[int](),
		trending:  newTTLCache[[]TrendingArticle](),
		restSem:   make(chan struct{}, restConcurrency),
		actionSem: make(chan struct{}, actionConcurrency),
		sleep:     time.Sleep,
		now:       time.Now,
		logger:    logger,
	}
}

// Measure implements Attention. Titles are resolved to articles in bulk:
// the disambiguated titles a work commonly has ("Moana (2026 film)",
// "Fallout (TV series)") are checked fifty at a time with one action API
// query that follows redirects and flags disambiguation pages; titles that
// have none fall back to the search API, one at a time. The bare title is
// never guessed: "Superman" is the character, not the film.
func (w *Wikipedia) Measure(ctx context.Context, kind Kind, titles []Title) []Measurement {
	articles := w.resolve(ctx, kind, titles)
	out := make([]Measurement, len(titles))
	var wg sync.WaitGroup
	for i, article := range articles {
		if article == "" {
			continue
		}
		wg.Add(1)
		go func(i int, article string) {
			defer wg.Done()
			views, found, _ := w.pageviews(ctx, article)
			out[i] = Measurement{Article: article, Views: views, Found: found}
		}(i, article)
	}
	wg.Wait()
	return out
}

// ArticleViews implements Attention.
func (w *Wikipedia) ArticleViews(ctx context.Context, article string) (int, bool) {
	views, found, _ := w.pageviews(ctx, article)
	return views, found
}

// resolve returns each title's article, "" when it has none.
func (w *Wikipedia) resolve(ctx context.Context, kind Kind, titles []Title) []string {
	keys := make([]string, len(titles))
	names := make([]string, len(titles))
	articles := make([]string, len(titles))
	var pending []int
	for i, title := range titles {
		names[i] = cleanTitle(title.Name)
		keys[i] = string(kind) + "|" + strings.ToLower(names[i]) + "|" + title.Year
		if names[i] == "" {
			continue
		}
		if hit, ok := w.articles.get(keys[i]); ok {
			articles[i] = hit
			continue
		}
		pending = append(pending, i)
	}
	if len(pending) == 0 {
		return articles
	}

	// Pass 1: the guessed titles, in bulk.
	guesses := map[int][]string{}
	var all []string
	for _, i := range pending {
		guesses[i] = articleGuesses(kind, names[i], titles[i].Year)
		all = append(all, guesses[i]...)
	}
	existing, err := w.lookupTitles(ctx, all)
	if err != nil {
		w.logger.WarnContext(ctx, "Wikipedia title lookup failed", telemetry.WithTraceContext(ctx,
			"kind", string(kind), "titles", len(all), "error", err)...)
		return articles
	}
	var unresolved []int
	for _, i := range pending {
		for _, guess := range guesses[i] {
			if article, ok := existing[guess]; ok {
				articles[i] = article
				break
			}
		}
		if articles[i] != "" {
			w.articles.set(keys[i], articles[i], articleTTL)
		} else {
			unresolved = append(unresolved, i)
		}
	}

	// Pass 2: search, one title at a time.
	for _, i := range unresolved {
		hits, err := w.search(ctx, kind, names[i], titles[i].Year)
		if err != nil {
			w.logger.WarnContext(ctx, "Wikipedia search failed", telemetry.WithTraceContext(ctx,
				"title", names[i], "error", err)...)
			continue
		}
		for _, hit := range hits {
			if articleMatches(hit, names[i]) {
				articles[i] = hit
				break
			}
		}
		w.articles.set(keys[i], articles[i], articleTTL)
	}
	return articles
}

// articleGuesses lists the disambiguated article titles a work commonly has.
func articleGuesses(kind Kind, name, year string) []string {
	if kind == KindMovie {
		guesses := []string{name + " (film)"}
		if year != "" {
			guesses = append([]string{name + " (" + year + " film)"}, guesses...)
		}
		return guesses
	}
	guesses := []string{name + " (TV series)", name + " (American TV series)"}
	if year != "" {
		guesses = append([]string{name + " (" + year + " TV series)"}, guesses...)
	}
	return guesses
}

// lookupTitles asks the action API which of the titles exist, fifty per
// request, following redirects. It returns each existing title mapped to
// its canonical article; disambiguation pages are left out.
func (w *Wikipedia) lookupTitles(ctx context.Context, titles []string) (map[string]string, error) {
	existing := map[string]string{}
	for from := 0; from < len(titles); from += titlesPerQuery {
		to := from + titlesPerQuery
		if to > len(titles) {
			to = len(titles)
		}
		query := url.Values{}
		query.Set("action", "query")
		query.Set("titles", strings.Join(titles[from:to], "|"))
		query.Set("redirects", "1")
		query.Set("prop", "pageprops")
		query.Set("ppprop", "disambiguation")
		query.Set("format", "json")
		query.Set("formatversion", "2")
		var response struct {
			Query struct {
				Normalized []struct {
					From string `json:"from"`
					To   string `json:"to"`
				} `json:"normalized"`
				Redirects []struct {
					From string `json:"from"`
					To   string `json:"to"`
				} `json:"redirects"`
				Pages []struct {
					Title     string          `json:"title"`
					Missing   bool            `json:"missing"`
					PageProps json.RawMessage `json:"pageprops"`
				} `json:"pages"`
			} `json:"query"`
		}
		if err := w.actionGet(ctx, w.opts.APIHost+"/w/api.php?"+query.Encode(), &response); err != nil {
			return nil, err
		}
		usable := map[string]bool{} // canonical title -> exists and is not a disambiguation page
		for _, page := range response.Query.Pages {
			usable[page.Title] = !page.Missing && !strings.Contains(string(page.PageProps), "disambiguation")
		}
		normalized := map[string]string{}
		for _, n := range response.Query.Normalized {
			normalized[n.From] = n.To
		}
		redirected := map[string]string{}
		for _, r := range response.Query.Redirects {
			redirected[r.From] = r.To
		}
		for _, title := range titles[from:to] {
			canonical := title
			if to, ok := normalized[canonical]; ok {
				canonical = to
			}
			if to, ok := redirected[canonical]; ok {
				canonical = to
			}
			if usable[canonical] {
				existing[title] = canonical
			}
		}
	}
	return existing, nil
}

// search asks the search API for the work and returns the hit titles.
func (w *Wikipedia) search(ctx context.Context, kind Kind, name, year string) ([]string, error) {
	noun := "film"
	if kind == KindSeries {
		noun = "television series"
	}
	query := url.Values{}
	query.Set("action", "query")
	query.Set("list", "search")
	query.Set("srsearch", strings.TrimSpace(name+" "+year+" "+noun))
	query.Set("srlimit", "5")
	query.Set("srprop", "")
	query.Set("format", "json")
	var response struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := w.actionGet(ctx, w.opts.APIHost+"/w/api.php?"+query.Encode(), &response); err != nil {
		return nil, err
	}
	titles := make([]string, 0, len(response.Query.Search))
	for _, hit := range response.Query.Search {
		titles = append(titles, hit.Title)
	}
	return titles, nil
}

// pageviews sums the article's views over the window. found is false when
// Wikipedia has no such article; failed is true when the lookup itself
// failed and nothing is known.
func (w *Wikipedia) pageviews(ctx context.Context, article string) (views int, found, failed bool) {
	if hit, ok := w.views.get(article); ok {
		return hit, hit >= 0, false
	}
	end := w.now().UTC().AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -(viewsWindowDays - 1))
	path := fmt.Sprintf("/api/rest_v1/metrics/pageviews/per-article/en.wikipedia/all-access/user/%s/daily/%s/%s",
		url.PathEscape(strings.ReplaceAll(article, " ", "_")), start.Format("20060102"), end.Format("20060102"))
	var response struct {
		Items []struct {
			Views int `json:"views"`
		} `json:"items"`
	}
	status, err := w.restGet(ctx, w.opts.RESTHost+path, &response)
	if err != nil {
		if status == http.StatusNotFound {
			w.views.set(article, -1, viewsTTL)
			return 0, false, false
		}
		w.logger.WarnContext(ctx, "Pageviews lookup failed", telemetry.WithTraceContext(ctx,
			"article", article, "error", err)...)
		return 0, false, true
	}
	total := 0
	for _, item := range response.Items {
		total += item.Views
	}
	w.views.set(article, total, viewsTTL)
	return total, true, false
}

// Trending implements Attention: the film or TV articles among yesterday's
// most-read English Wikipedia articles (the day before when yesterday's list
// is not published yet).
func (w *Wikipedia) Trending(ctx context.Context, kind Kind) []TrendingArticle {
	if hit, ok := w.trending.get(string(kind)); ok {
		return hit
	}
	var articles []TrendingArticle
	for daysBack := 1; daysBack <= 2; daysBack++ {
		day := w.now().UTC().AddDate(0, 0, -daysBack)
		path := "/api/rest_v1/metrics/pageviews/top/en.wikipedia/all-access/" + day.Format("2006/01/02")
		var response struct {
			Items []struct {
				Articles []struct {
					Article string `json:"article"`
					Views   int    `json:"views"`
				} `json:"articles"`
			} `json:"items"`
		}
		status, err := w.restGet(ctx, w.opts.RESTHost+path, &response)
		if err != nil {
			if status != http.StatusNotFound {
				w.logger.WarnContext(ctx, "Most-read list failed", telemetry.WithTraceContext(ctx,
					"day", day.Format("2006-01-02"), "error", err)...)
				return nil
			}
			continue
		}
		for _, item := range response.Items {
			for _, entry := range item.Articles {
				if trending, ok := parseTrendingArticle(kind, strings.ReplaceAll(entry.Article, "_", " ")); ok {
					trending.Views = entry.Views
					articles = append(articles, trending)
					if len(articles) == trendingMax {
						break
					}
				}
			}
		}
		break
	}
	w.trending.set(string(kind), articles, trendingTTL)
	return articles
}

var (
	// disambiguationYear strips TVDB's "(2025)" suffix on names that need
	// telling apart, e.g. "Paradise (2025)".
	disambiguationYear = regexp.MustCompile(`\s*\(\d{4}\)$`)
	nonAlphanumeric    = regexp.MustCompile(`[^a-z0-9]+`)
	// filmArticle matches "Title (film)", "Title (2025 film)" and "Title (2025 American film)".
	filmArticle = regexp.MustCompile(`^(.+?) \((?:(\d{4}) )?(?:[A-Za-z-]+ )*film\)$`)
	// seriesArticle matches "Title (TV series)", "Title (American TV series)" and "Title (2025 TV series)".
	seriesArticle = regexp.MustCompile(`^(.+?) \((?:(\d{4}) )?(?:[A-Za-z-]+ )*TV series\)$`)
	// seasonArticle matches "Title season 5" and "Title (season 5)".
	seasonArticle = regexp.MustCompile(`^(.+?) \(?season \d+\)?$`)
)

// cleanTitle removes a TVDB year disambiguator from a name.
func cleanTitle(name string) string {
	return strings.TrimSpace(disambiguationYear.ReplaceAllString(name, ""))
}

// normalizeTitle lowercases and strips punctuation so "Thunderbolts*" and
// "Thunderbolts" compare equal.
func normalizeTitle(title string) string {
	return strings.Trim(nonAlphanumeric.ReplaceAllString(strings.ToLower(title), " "), " ")
}

// articleMatches reports whether an article title names the work: the title
// before any parenthesis must equal the work's title once normalised.
func articleMatches(article, name string) bool {
	base := article
	if i := strings.Index(article, " ("); i > 0 {
		base = article[:i]
	}
	return normalizeTitle(base) == normalizeTitle(name)
}

// parseTrendingArticle reads a work's title and year out of an article title
// of the right kind; other articles report false.
func parseTrendingArticle(kind Kind, article string) (TrendingArticle, bool) {
	if strings.HasPrefix(article, "List of ") {
		return TrendingArticle{}, false
	}
	var patterns []*regexp.Regexp
	if kind == KindMovie {
		patterns = []*regexp.Regexp{filmArticle}
	} else {
		patterns = []*regexp.Regexp{seriesArticle, seasonArticle}
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(article)
		if match == nil {
			continue
		}
		trending := TrendingArticle{Article: article, Title: match[1]}
		if len(match) > 2 {
			trending.Year = match[2]
		}
		return trending, true
	}
	return TrendingArticle{}, false
}

// actionGet performs one action API request, in series with the others,
// retrying once when rate limited.
func (w *Wikipedia) actionGet(ctx context.Context, target string, into any) error {
	status, err := w.getJSON(ctx, w.actionSem, target, into)
	if status == http.StatusTooManyRequests {
		w.sleep(retryWait)
		status, err = w.getJSON(ctx, w.actionSem, target, into)
	}
	if err != nil {
		return fmt.Errorf("status %d: %w", status, err)
	}
	return nil
}

// restGet performs one paced REST API request, retrying once when rate
// limited (after Retry-After when given). It returns the final status code.
func (w *Wikipedia) restGet(ctx context.Context, target string, into any) (int, error) {
	w.pace()
	status, err := w.getJSON(ctx, w.restSem, target, into)
	if status == http.StatusTooManyRequests {
		w.sleep(retryAfter(err))
		w.pace()
		status, err = w.getJSON(ctx, w.restSem, target, into)
	}
	return status, err
}

// pace spaces REST requests restInterval apart across all callers.
func (w *Wikipedia) pace() {
	w.paceMu.Lock()
	now := w.now()
	if w.nextREST.Before(now) {
		w.nextREST = now
	}
	wait := w.nextREST.Sub(now)
	w.nextREST = w.nextREST.Add(restInterval)
	w.paceMu.Unlock()
	if wait > 0 {
		w.sleep(wait)
	}
}

// retryAfter reads the wait a rate-limit error carries, within bounds.
func retryAfter(err error) time.Duration {
	var limited *rateLimitedError
	if errors.As(err, &limited) && limited.retryAfter > 0 {
		if limited.retryAfter > maxRetryWait {
			return maxRetryWait
		}
		return limited.retryAfter
	}
	return retryWait
}

// rateLimitedError is a 429 answer with its Retry-After, when given.
type rateLimitedError struct {
	retryAfter time.Duration
}

func (e *rateLimitedError) Error() string { return "status 429" }

// getJSON performs a GET bounded by sem and decodes the JSON body. A non-2xx
// status is returned as an error together with the status code.
func (w *Wikipedia) getJSON(ctx context.Context, sem chan struct{}, target string, into any) (int, error) {
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	defer func() { <-sem }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := w.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			w.logger.InfoContext(ctx, "Error closing Wikimedia response body", "error", cerr)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		seconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return resp.StatusCode, &rateLimitedError{retryAfter: time.Duration(seconds) * time.Second}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, into); err != nil {
		return resp.StatusCode, fmt.Errorf("decode: %w", err)
	}
	return resp.StatusCode, nil
}
