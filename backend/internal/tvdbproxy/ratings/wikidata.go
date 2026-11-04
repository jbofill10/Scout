package ratings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	// DefaultWikidataSPARQLURL is Wikidata's public query service.
	DefaultWikidataSPARQLURL = "https://query.wikidata.org/sparql"
	// WikidataChunk is how many titles one query asks about.
	WikidataChunk = 50

	wikidataTimeout = 30 * time.Second
	wikidataMaxBody = 4 << 20
	wikidataAgent   = "Scout/1.0 (+https://github.com/jbofill10/scout)"

	// Wikidata items the review scores are attributed to (P447).
	qidRottenTomatoes = "Q105584"
	qidMetacritic     = "Q150248"
)

var (
	wikidataIDPattern = regexp.MustCompile(`^tt\d{1,10}$`)
	percentPattern    = regexp.MustCompile(`^(\d{1,3})\s*%$`)
	outOf100Pattern   = regexp.MustCompile(`^(\d{1,3})\s*/\s*100$`)
)

// Record is what Wikidata knows about one title: the scores it records and
// where its English Wikipedia article is, for the prose fallback.
type Record struct {
	Ratings []tvdb.Rating
	// Article is the English Wikipedia article title, "" when there is none.
	Article string
	// Film says the item is a film (or a kind of film), which is where the
	// article's reception prose is read from.
	Film bool
}

// Wikidata reads the Rotten Tomatoes Tomatometer and the Metascore that
// Wikidata records on film and series items ("review score", P444, with
// "review score by" naming the critic site), keyed by IMDb id (P345), along
// with each item's English Wikipedia article. The data is CC0 and needs no
// key; bots keep it current for established titles but it lags on new
// releases, which the article prose and OMDb cover.
type Wikidata struct {
	url    string
	http   *http.Client
	logger *slog.Logger
}

// NewWikidataHTTPClient returns the traced HTTP client used for the query service.
func NewWikidataHTTPClient() *http.Client {
	return &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: wikidataTimeout}
}

// NewWikidata returns a source querying the SPARQL endpoint at url (empty
// means DefaultWikidataSPARQLURL).
func NewWikidata(url string, httpClient *http.Client, logger *slog.Logger) *Wikidata {
	if url == "" {
		url = DefaultWikidataSPARQLURL
	}
	return &Wikidata{url: url, http: httpClient, logger: logger}
}

// sparqlResponse is the part of a SPARQL JSON result the parser reads.
type sparqlResponse struct {
	Results struct {
		Bindings []map[string]struct {
			Value string `json:"value"`
		} `json:"bindings"`
	} `json:"results"`
}

// Records returns what Wikidata holds for each IMDb id: the Tomatometer and
// Metascore (Rotten Tomatoes first) and the English Wikipedia article. Ids
// Wikidata has no item for are absent from the map; ids that are not title
// ids are ignored.
func (w *Wikidata) Records(ctx context.Context, imdbIDs []string) (map[string]Record, error) {
	ids := make([]string, 0, len(imdbIDs))
	for _, id := range imdbIDs {
		if wikidataIDPattern.MatchString(id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return map[string]Record{}, nil
	}

	query := url.Values{}
	query.Set("query", sparqlFor(ids))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.url+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", wikidataAgent)

	resp, err := w.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wikidata: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			w.logger.InfoContext(ctx, "Error closing Wikidata response body", "error", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wikidata: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, wikidataMaxBody))
	if err != nil {
		return nil, fmt.Errorf("wikidata: read: %w", err)
	}
	var result sparqlResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("wikidata: decode: %w", err)
	}
	return parseRecords(result), nil
}

// sparqlFor asks, for the items with the given IMDb ids, for every review
// score with its reviewer and date, the English Wikipedia article, and
// whether the item is a film.
func sparqlFor(ids []string) string {
	var values strings.Builder
	for _, id := range ids {
		values.WriteString(`"` + id + `" `)
	}
	return `SELECT ?imdb ?score ?by ?date ?article ?film WHERE { VALUES ?imdb { ` + values.String() + `} ` +
		`?item wdt:P345 ?imdb . ` +
		`OPTIONAL { ?article schema:about ?item ; schema:isPartOf <https://en.wikipedia.org/> . } ` +
		`OPTIONAL { ?item p:P444 ?st . ?st ps:P444 ?score . ?st pq:P447 ?by . OPTIONAL { ?st pq:P585 ?date } } ` +
		`BIND(EXISTS { ?item wdt:P31/wdt:P279* wd:Q11424 } AS ?film) }`
}

// parseRecords folds the result rows into one Record per id.
func parseRecords(result sparqlResponse) map[string]Record {
	scores := parseScores(result)
	records := map[string]Record{}
	for _, binding := range result.Results.Bindings {
		id := binding["imdb"].Value
		record := records[id]
		if article, ok := binding["article"]; ok && record.Article == "" {
			record.Article = articleTitle(article.Value)
		}
		if film, ok := binding["film"]; ok && film.Value == "true" {
			record.Film = true
		}
		record.Ratings = scores[id]
		records[id] = record
	}
	return records
}

// articleTitle turns "https://en.wikipedia.org/wiki/Sinners_(2025_film)" into
// "Sinners (2025 film)".
func articleTitle(link string) string {
	path := link[strings.LastIndex(link, "/wiki/")+len("/wiki/"):]
	if strings.LastIndex(link, "/wiki/") < 0 {
		return ""
	}
	title, err := url.PathUnescape(path)
	if err != nil {
		title = path
	}
	return strings.ReplaceAll(title, "_", " ")
}

// scored is one candidate score with the date it was taken.
type scored struct {
	value float64
	date  string
}

// parseScores keeps, per id and source, the most recently dated score in
// the source's own format: "93%" for Rotten Tomatoes (its "8.7/10" average
// is skipped), "81/100" for Metacritic.
func parseScores(result sparqlResponse) map[string][]tvdb.Rating {
	type key struct{ id, source string }
	best := map[key]scored{}
	for _, binding := range result.Results.Bindings {
		id := binding["imdb"].Value
		reviewer := binding["by"].Value[strings.LastIndex(binding["by"].Value, "/")+1:]
		raw := strings.TrimSpace(binding["score"].Value)
		var (
			source  string
			pattern *regexp.Regexp
		)
		switch reviewer {
		case qidRottenTomatoes:
			source, pattern = tvdb.RatingSourceRottenTomatoes, percentPattern
		case qidMetacritic:
			source, pattern = tvdb.RatingSourceMetacritic, outOf100Pattern
		default:
			continue
		}
		match := pattern.FindStringSubmatch(raw)
		if match == nil {
			continue
		}
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil || value > 100 {
			continue
		}
		date := binding["date"].Value
		k := key{id, source}
		if current, ok := best[k]; !ok || date >= current.date {
			best[k] = scored{value: value, date: date}
		}
	}

	scores := map[string][]tvdb.Rating{}
	for k, s := range best {
		scores[k.id] = append(scores[k.id], tvdb.Rating{Source: k.source, Value: s.value})
	}
	for id, ratings := range scores {
		if len(ratings) == 2 && ratings[0].Source != tvdb.RatingSourceRottenTomatoes {
			ratings[0], ratings[1] = ratings[1], ratings[0]
		}
		scores[id] = ratings
	}
	return scores
}
