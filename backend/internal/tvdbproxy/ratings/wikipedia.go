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
	// DefaultWikipediaAPIURL is English Wikipedia's MediaWiki API.
	DefaultWikipediaAPIURL = "https://en.wikipedia.org/w/api.php"
	// ProseChunk is how many articles one request fetches; a film article
	// runs 20-100 KB of wikitext.
	ProseChunk = 20

	proseTimeout = 30 * time.Second
	proseMaxBody = 16 << 20
)

var (
	rtTemplate = regexp.MustCompile(`(?i)\{\{\s*(?:Rotten Tomatoes prose|RT prose)\s*\|([^}]*)\}\}`)
	// rtProse finds the first percentage within a sentence or so of a
	// Rotten Tomatoes mention: "On Rotten Tomatoes, 93% of 250 critics'
	// reviews are positive", "holds an approval rating of 93%".
	rtProse    = regexp.MustCompile(`(?i)Rotten Tomatoes\]*[^%]{0,260}?(\d{1,3})%`)
	mcTemplate = regexp.MustCompile(`(?i)\{\{\s*Metacritic film prose\s*\|([^}]*)\}\}`)
	mcProse    = regexp.MustCompile(`(?i)Metacritic\]*[^.]{0,200}?(?:(\d{1,3}) out of 100|score of (\d{1,3}))`)
)

// Prose reads the Tomatometer and Metascore out of English Wikipedia film
// articles, which editors update from release week on, well before the
// score lands on Wikidata. It covers the brand-new releases Wikidata lacks.
type Prose struct {
	url    string
	http   *http.Client
	logger *slog.Logger
}

// NewProseHTTPClient returns the traced HTTP client used for the MediaWiki API.
func NewProseHTTPClient() *http.Client {
	return &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: proseTimeout}
}

// NewProse returns a source reading articles through the MediaWiki API at
// url (empty means DefaultWikipediaAPIURL).
func NewProse(url string, httpClient *http.Client, logger *slog.Logger) *Prose {
	if url == "" {
		url = DefaultWikipediaAPIURL
	}
	return &Prose{url: url, http: httpClient, logger: logger}
}

// mediawikiQuery is the part of a revisions query result the parser reads.
type mediawikiQuery struct {
	Query struct {
		Normalized []struct{ From, To string } `json:"normalized"`
		Redirects  []struct{ From, To string } `json:"redirects"`
		Pages      []struct {
			Title     string `json:"title"`
			Missing   bool   `json:"missing"`
			Revisions []struct {
				Slots struct {
					Main struct {
						Content string `json:"content"`
					} `json:"main"`
				} `json:"slots"`
			} `json:"revisions"`
		} `json:"pages"`
	} `json:"query"`
}

// searchCandidates is how many search hits Search reads before giving up.
const searchCandidates = 5

var infoboxFilm = regexp.MustCompile(`(?i)\{\{\s*Infobox film\b`)

// Search finds the article for a film by name and release year, for titles
// Wikidata has no item for. A hit counts only if it is a film article (it
// carries {{Infobox film}}) and names the year in its lead, so a namesake
// from another year is not taken.
func (p *Prose) Search(ctx context.Context, name, year string) (string, bool) {
	query := url.Values{}
	query.Set("action", "query")
	query.Set("list", "search")
	query.Set("srsearch", `intitle:"`+strings.ReplaceAll(name, `"`, "")+`"`)
	query.Set("srlimit", strconv.Itoa(searchCandidates))
	query.Set("format", "json")
	query.Set("formatversion", "2")
	var result struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := p.getJSON(ctx, query, &result); err != nil {
		p.logger.WarnContext(ctx, "Wikipedia search failed", "name", name, "error", err)
		return "", false
	}
	for _, hit := range result.Query.Search {
		texts, err := p.wikitext(ctx, []string{hit.Title})
		if err != nil {
			return "", false
		}
		text := texts[hit.Title]
		if !infoboxFilm.MatchString(text) {
			continue
		}
		lead := text
		if len(lead) > 3000 {
			lead = lead[:3000]
		}
		if year == "" || strings.Contains(lead, year) {
			return hit.Title, true
		}
	}
	return "", false
}

// Scores returns the Tomatometer and Metascore found in each article's
// wikitext, keyed by the title as given. Articles without either are absent.
func (p *Prose) Scores(ctx context.Context, titles []string) (map[string][]tvdb.Rating, error) {
	scores := map[string][]tvdb.Rating{}
	for start := 0; start < len(titles); start += ProseChunk {
		chunk := titles[start:min(start+ProseChunk, len(titles))]
		if err := p.fetch(ctx, chunk, scores); err != nil {
			return nil, err
		}
	}
	return scores, nil
}

// fetch reads one chunk of articles into scores.
func (p *Prose) fetch(ctx context.Context, titles []string, scores map[string][]tvdb.Rating) error {
	texts, err := p.wikitext(ctx, titles)
	if err != nil {
		return err
	}
	for title, text := range texts {
		if ratings := scoresFromWikitext(text); len(ratings) > 0 {
			scores[title] = ratings
		}
	}
	return nil
}

// getJSON performs one MediaWiki API query and decodes its JSON answer.
func (p *Prose) getJSON(ctx context.Context, query url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", wikidataAgent)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("wikipedia: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			p.logger.InfoContext(ctx, "Error closing Wikipedia response body", "error", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wikipedia: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, proseMaxBody))
	if err != nil {
		return fmt.Errorf("wikipedia: read: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("wikipedia: decode: %w", err)
	}
	return nil
}

// wikitext fetches the current wikitext of each article, keyed by the title
// as given. Missing articles are absent.
func (p *Prose) wikitext(ctx context.Context, titles []string) (map[string]string, error) {
	query := url.Values{}
	query.Set("action", "query")
	query.Set("prop", "revisions")
	query.Set("rvprop", "content")
	query.Set("rvslots", "main")
	query.Set("redirects", "1")
	query.Set("format", "json")
	query.Set("formatversion", "2")
	query.Set("titles", strings.Join(titles, "|"))
	var result mediawikiQuery
	if err := p.getJSON(ctx, query, &result); err != nil {
		return nil, err
	}

	// The API answers under normalised and redirected titles; map each
	// requested title to the page that answered it.
	normalized := map[string]string{}
	for _, n := range result.Query.Normalized {
		normalized[n.From] = n.To
	}
	redirects := map[string]string{}
	for _, r := range result.Query.Redirects {
		redirects[r.From] = r.To
	}
	texts := map[string]string{}
	for _, page := range result.Query.Pages {
		if !page.Missing && len(page.Revisions) > 0 {
			texts[page.Title] = page.Revisions[0].Slots.Main.Content
		}
	}
	out := make(map[string]string, len(titles))
	for _, title := range titles {
		resolved := title
		if to, ok := normalized[resolved]; ok {
			resolved = to
		}
		if to, ok := redirects[resolved]; ok {
			resolved = to
		}
		if text, ok := texts[resolved]; ok {
			out[title] = text
		}
	}
	return out, nil
}

// scoresFromWikitext reads the Tomatometer and Metascore an article states,
// Rotten Tomatoes first.
func scoresFromWikitext(text string) []tvdb.Rating {
	var ratings []tvdb.Rating
	if value, ok := tomatometerFromWikitext(text); ok {
		ratings = append(ratings, tvdb.Rating{Source: tvdb.RatingSourceRottenTomatoes, Value: value})
	}
	if value, ok := metascoreFromWikitext(text); ok {
		ratings = append(ratings, tvdb.Rating{Source: tvdb.RatingSourceMetacritic, Value: value})
	}
	return ratings
}

// tomatometerFromWikitext prefers the {{Rotten Tomatoes prose}} template's
// score and otherwise the first critics' percentage after a Rotten Tomatoes
// mention, skipping sentences that only give the audience score.
func tomatometerFromWikitext(text string) (float64, bool) {
	if m := rtTemplate.FindStringSubmatch(text); m != nil {
		if value, ok := templateScore(m[1]); ok {
			return value, true
		}
	}
	for _, m := range rtProse.FindAllStringSubmatchIndex(text, -1) {
		window := strings.ToLower(text[m[0]:m[1]])
		if strings.Contains(window, "audience") && !strings.Contains(window, "critic") {
			continue
		}
		if value, ok := percent(text[m[2]:m[3]]); ok {
			return value, true
		}
	}
	return 0, false
}

// metascoreFromWikitext prefers the {{Metacritic film prose}} template and
// otherwise a "N out of 100" or "score of N" after a Metacritic mention.
func metascoreFromWikitext(text string) (float64, bool) {
	if m := mcTemplate.FindStringSubmatch(text); m != nil {
		if value, ok := templateScore(m[1]); ok {
			return value, true
		}
	}
	if m := mcProse.FindStringSubmatch(text); m != nil {
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		return percent(raw)
	}
	return 0, false
}

// templateScore reads the score out of a prose template's arguments: the
// named score= argument or the first positional one. A template with no
// number (it then reads Wikidata, which the caller already asked) gives none.
func templateScore(args string) (float64, bool) {
	var positional []string
	for _, arg := range strings.Split(args, "|") {
		arg = strings.TrimSpace(arg)
		name, value, named := strings.Cut(arg, "=")
		if named {
			if strings.TrimSpace(name) == "score" {
				return percent(strings.TrimSpace(value))
			}
			continue
		}
		positional = append(positional, arg)
	}
	if len(positional) > 0 {
		return percent(positional[0])
	}
	return 0, false
}

// percent parses a 0-100 score.
func percent(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value < 0 || value > 100 {
		return 0, false
	}
	return value, true
}
