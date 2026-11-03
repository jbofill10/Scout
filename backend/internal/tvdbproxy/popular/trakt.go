package popular

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	// DefaultTraktHost is Trakt's public API.
	DefaultTraktHost = "https://api.trakt.tv"
	// TraktListWatched ranks by unique watchers over a period.
	TraktListWatched = "watched"
	// TraktListTrending ranks by who is watching right now.
	TraktListTrending = "trending"
	// DefaultTraktPeriod is the watched period used when none is configured.
	DefaultTraktPeriod = "weekly"

	traktTimeout = 20 * time.Second
	// traktOverfetch is how many extra hits are asked for, so titles TVDB
	// does not know (or that lack a TVDB/IMDb id) do not shorten the row.
	traktOverfetch = 10
	// traktMaxPage is the most hits one Trakt page may hold.
	traktMaxPage = 100
	userAgent    = "Scout/1.0 (+https://github.com/jbofill10/scout)"
)

var traktPeriods = map[string]bool{"daily": true, "weekly": true, "monthly": true, "all": true}

// TraktOptions configures the Trakt source.
type TraktOptions struct {
	// Host is the API base URL; empty means DefaultTraktHost.
	Host string
	// ClientID is the Trakt application's client id, sent as trakt-api-key.
	ClientID string
	// List is TraktListWatched (default) or TraktListTrending.
	List string
	// Period applies to the watched list: daily, weekly (default), monthly or all.
	Period string
}

// Normalize fills defaults and replaces values Trakt would reject, reporting
// each replacement through logger.
func (o TraktOptions) Normalize(logger *slog.Logger) TraktOptions {
	if o.Host == "" {
		o.Host = DefaultTraktHost
	}
	o.Host = strings.TrimRight(o.Host, "/")
	o.List = strings.ToLower(strings.TrimSpace(o.List))
	switch o.List {
	case "":
		o.List = TraktListWatched
	case TraktListWatched, TraktListTrending:
	default:
		logger.Warn("Invalid TRAKT_LIST, using default", "value", o.List, "default", TraktListWatched)
		o.List = TraktListWatched
	}
	o.Period = strings.ToLower(strings.TrimSpace(o.Period))
	switch {
	case o.Period == "":
		o.Period = DefaultTraktPeriod
	case !traktPeriods[o.Period]:
		logger.Warn("Invalid TRAKT_PERIOD, using default", "value", o.Period, "default", DefaultTraktPeriod)
		o.Period = DefaultTraktPeriod
	}
	return o
}

// TraktSource ranks rows by Trakt's most watched or trending list and
// resolves every hit to its TVDB record: shows through the TVDB id Trakt
// carries, movies through their IMDb id and TVDB's remote-id lookup.
type TraktSource struct {
	opts    TraktOptions
	http    *http.Client
	records *records
	logger  *slog.Logger
}

// NewTraktHTTPClient returns the traced HTTP client used for Trakt requests.
func NewTraktHTTPClient() *http.Client {
	return &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: traktTimeout}
}

// NewTraktSource returns a source for opts that resolves records through tvdbClient.
func NewTraktSource(
	opts TraktOptions, httpClient *http.Client, tvdbClient TVDBGetter, logger *slog.Logger,
) *TraktSource {
	return &TraktSource{
		opts:    opts.Normalize(logger),
		http:    httpClient,
		records: newRecords(tvdbClient, logger),
		logger:  logger,
	}
}

// Name implements Source.
func (t *TraktSource) Name() string {
	if t.opts.List == TraktListTrending {
		return "trakt trending"
	}
	return "trakt watched " + t.opts.Period
}

// Rank implements Source. Trakt's order is kept; hits TVDB cannot resolve
// are dropped.
func (t *TraktSource) Rank(ctx context.Context, kind Kind, genre string, limit int) ([]tvdb.Media, error) {
	limit = clampLimit(limit)
	entries, err := t.list(ctx, kind, genre, limit+traktOverfetch)
	if err != nil {
		return nil, err
	}

	resolved := make([]*tvdb.Media, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Add(1)
		go func(i int, entry traktEntry) {
			defer wg.Done()
			if media, ok := t.resolve(ctx, kind, entry); ok {
				resolved[i] = &media
			}
		}(i, entry)
	}
	wg.Wait()

	results := []tvdb.Media{}
	for _, media := range resolved {
		if media == nil {
			continue
		}
		results = append(results, *media)
		if len(results) == limit {
			break
		}
	}
	t.logger.InfoContext(ctx, "Trakt row resolved", telemetry.WithTraceContext(ctx,
		"kind", string(kind), "genre", genre, "trakt_hits", len(entries), "resolved", len(results))...)
	return results, nil
}

// traktEntry is one hit of a watched or trending list. Watched lists carry
// watcher_count; trending lists carry watchers.
type traktEntry struct {
	Watchers     int        `json:"watchers"`
	WatcherCount int        `json:"watcher_count"`
	PlayCount    int        `json:"play_count"`
	Show         *traktItem `json:"show"`
	Movie        *traktItem `json:"movie"`
}

// traktItem is a show or movie with extended=full fields.
type traktItem struct {
	Title         string   `json:"title"`
	Year          int      `json:"year"`
	IDs           traktIDs `json:"ids"`
	Tagline       string   `json:"tagline"`
	Overview      string   `json:"overview"`
	Runtime       int      `json:"runtime"`
	Certification string   `json:"certification"`
	Network       string   `json:"network"`
	Country       string   `json:"country"`
	Status        string   `json:"status"`
	Genres        []string `json:"genres"`
	Language      string   `json:"language"`
}

type traktIDs struct {
	Trakt int    `json:"trakt"`
	Slug  string `json:"slug"`
	TVDB  int    `json:"tvdb"`
	IMDB  string `json:"imdb"`
	TMDB  int    `json:"tmdb"`
}

// list fetches one page of the configured list for kind and genre.
func (t *TraktSource) list(ctx context.Context, kind Kind, genre string, n int) ([]traktEntry, error) {
	if n > traktMaxPage {
		n = traktMaxPage
	}
	collection := "shows"
	if kind == KindMovie {
		collection = "movies"
	}
	path := "/" + collection + "/trending"
	if t.opts.List == TraktListWatched {
		path = "/" + collection + "/watched/" + t.opts.Period
	}
	query := url.Values{}
	query.Set("limit", strconv.Itoa(n))
	query.Set("extended", "full")
	if genre != "" {
		query.Set("genres", TraktGenreSlug(genre))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.opts.Host+path+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", t.opts.ClientID)

	resp, err := t.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trakt %s: %w", path, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.logger.InfoContext(ctx, "Error closing Trakt response body", "error", cerr)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("trakt %s: read body: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("trakt %s: rate limited, retry after %s", path, resp.Header.Get("Retry-After"))
		}
		return nil, fmt.Errorf("trakt %s: status %d", path, resp.StatusCode)
	}
	var entries []traktEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("trakt %s: decode: %w", path, err)
	}
	return entries, nil
}

// resolve turns a Trakt hit into Scout's Media, keyed by TVDB id.
func (t *TraktSource) resolve(ctx context.Context, kind Kind, entry traktEntry) (tvdb.Media, bool) {
	var (
		item   *traktItem
		record *baseRecord
		err    error
	)
	switch kind {
	case KindSeries:
		item = entry.Show
		if item == nil || item.IDs.TVDB == 0 {
			return tvdb.Media{}, false
		}
		record, err = t.records.series(ctx, item.IDs.TVDB)
	case KindMovie:
		item = entry.Movie
		if item == nil || item.IDs.IMDB == "" {
			return tvdb.Media{}, false
		}
		record, err = t.records.movieByIMDb(ctx, item.IDs.IMDB)
	default:
		return tvdb.Media{}, false
	}
	if err != nil {
		t.logger.WarnContext(ctx, "Could not resolve Trakt hit on TVDB", telemetry.WithTraceContext(ctx,
			"kind", string(kind), "title", item.Title, "tvdb_id", item.IDs.TVDB, "imdb_id", item.IDs.IMDB,
			"error", err)...)
		return tvdb.Media{}, false
	}
	if record == nil {
		t.logger.InfoContext(ctx, "TVDB has no record for Trakt hit", telemetry.WithTraceContext(ctx,
			"kind", string(kind), "title", item.Title, "tvdb_id", item.IDs.TVDB, "imdb_id", item.IDs.IMDB)...)
		return tvdb.Media{}, false
	}
	media := mediaFromTrakt(kind, item, record)
	media.Popularity = t.popularity(entry)
	return media, true
}

// popularity describes a hit's place on the configured Trakt list.
func (t *TraktSource) popularity(entry traktEntry) *tvdb.Popularity {
	if t.opts.List == TraktListTrending {
		return &tvdb.Popularity{Source: "trakt", Value: entry.Watchers, Detail: "watching now"}
	}
	return &tvdb.Popularity{Source: "trakt", Value: entry.WatcherCount, Detail: "watchers, " + t.opts.Period}
}

// mediaFromTrakt merges Trakt's English-language details with TVDB's
// identity, poster and status. The name is Trakt's title so anime and other
// non-English titles read as the UI expects; TVDB's name is kept as the
// original.
func mediaFromTrakt(kind Kind, item *traktItem, record *baseRecord) tvdb.Media {
	name := item.Title
	if name == "" {
		name = record.Name
	}
	year := record.Year
	if item.Year > 0 {
		year = strconv.Itoa(item.Year)
	}
	runtime := item.Runtime
	if runtime == 0 {
		runtime = record.Runtime
		if runtime == 0 {
			runtime = record.AverageRuntime
		}
	}
	genres := make([]string, 0, len(item.Genres))
	for _, slug := range item.Genres {
		genres = append(genres, traktGenreName(slug))
	}
	return tvdb.Media{
		Id:               strconv.Itoa(record.ID),
		Name:             name,
		Category:         string(kind),
		Anime:            traktIsAnime(kind, item.Genres),
		Score:            record.Score,
		Slug:             record.Slug,
		ImageUrl:         artworkURL(record.Image),
		OriginalName:     record.Name,
		Status:           record.Status,
		Overview:         item.Overview,
		Tagline:          item.Tagline,
		Year:             year,
		Aliases:          []string{},
		Genres:           genres,
		Runtime:          runtime,
		ContentRating:    item.Certification,
		OriginalCountry:  record.OriginalCountry,
		OriginalLanguage: record.OriginalLanguage,
		ImdbID:           item.IDs.IMDB,
		Network:          item.Network,
		Metadata:         tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}},
	}
}

// traktIsAnime applies Scout's anime rule to Trakt genre slugs: "anime"
// marks anything, and "animation" additionally marks movies.
func traktIsAnime(kind Kind, genres []string) bool {
	for _, genre := range genres {
		if genre == "anime" || (kind == KindMovie && genre == "animation") {
			return true
		}
	}
	return false
}

// traktGenreSlugs maps the genre names Scout's UI uses (TVDB's names) to
// Trakt's slugs where plain slugification would not match.
var traktGenreSlugs = map[string]string{
	"sci-fi":          "science-fiction",
	"science fiction": "science-fiction",
	"home and garden": "home-and-garden",
	"talk show":       "talk-show",
	"game show":       "game-show",
	"awards show":     "awards-show",
	"mini-series":     "mini-series",
	"sport":           "sporting-event",
	"children":        "children",
	"indie":           "indie",
}

// TraktGenreSlug converts a TVDB genre name to the Trakt genre slug.
func TraktGenreSlug(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if slug, ok := traktGenreSlugs[key]; ok {
		return slug
	}
	return strings.ReplaceAll(strings.ReplaceAll(key, " & ", "-"), " ", "-")
}

// traktGenreName turns a Trakt slug back into a display name.
func traktGenreName(slug string) string {
	words := strings.Split(slug, "-")
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}
