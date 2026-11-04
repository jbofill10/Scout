package ratings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	// DefaultOMDbHost is OMDb's public API.
	DefaultOMDbHost = "https://www.omdbapi.com"

	omdbTimeout = 10 * time.Second
	// omdbMaxBody bounds a title record; real ones are a few KB.
	omdbMaxBody = 1 << 20
)

// ErrRateLimited is returned when the OMDb key's daily allowance is used up.
var ErrRateLimited = errors.New("omdb: request limit reached")

// ErrUnauthorized is returned when OMDb rejects the key itself.
var ErrUnauthorized = errors.New("omdb: key rejected")

// OMDb looks titles up on OMDb (omdbapi.com), which carries IMDb's rating
// next to the Rotten Tomatoes Tomatometer and Metacritic's Metascore. A free
// key allows 1,000 lookups a day, which is why Service caches every answer.
type OMDb struct {
	host   string
	apiKey string
	http   *http.Client
	logger *slog.Logger
}

// NewOMDbHTTPClient returns the traced HTTP client used for OMDb requests.
func NewOMDbHTTPClient() *http.Client {
	return &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: omdbTimeout}
}

// NewOMDb returns a source for the OMDb API at host (empty means
// DefaultOMDbHost) authenticated with apiKey.
func NewOMDb(host, apiKey string, httpClient *http.Client, logger *slog.Logger) *OMDb {
	if host == "" {
		host = DefaultOMDbHost
	}
	return &OMDb{host: strings.TrimRight(host, "/"), apiKey: apiKey, http: httpClient, logger: logger}
}

// omdbRecord is the part of an OMDb title record the lookup reads. Numbers
// come as strings, "N/A" when unknown.
type omdbRecord struct {
	Response string `json:"Response"`
	Error    string `json:"Error"`
	Ratings  []struct {
		Source string `json:"Source"`
		Value  string `json:"Value"`
	} `json:"Ratings"`
	IMDbRating string `json:"imdbRating"`
	IMDbVotes  string `json:"imdbVotes"`
	Metascore  string `json:"Metascore"`
}

// Lookup returns the ratings OMDb holds for an IMDb title id, IMDb first,
// then Rotten Tomatoes, then Metacritic. A nil slice with a nil error means
// OMDb has no record for the id, or none with a rating on it.
func (o *OMDb) Lookup(ctx context.Context, imdbID string) ([]tvdb.Rating, error) {
	query := url.Values{}
	query.Set("i", imdbID)
	query.Set("apikey", o.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.host+"/?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("omdb: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			o.logger.InfoContext(ctx, "Error closing OMDb response body", "error", cerr)
		}
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, omdbMaxBody))
	if err != nil {
		return nil, fmt.Errorf("omdb: read %s: %w", imdbID, err)
	}

	var record omdbRecord
	if jsonErr := json.Unmarshal(body, &record); jsonErr != nil && resp.StatusCode == http.StatusOK {
		return nil, fmt.Errorf("omdb: decode %s: %w", imdbID, jsonErr)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		if strings.Contains(strings.ToLower(record.Error), "limit") {
			return nil, ErrRateLimited
		}
		return nil, fmt.Errorf("%w: %s", ErrUnauthorized, record.Error)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("omdb: status %d for %s", resp.StatusCode, imdbID)
	case record.Response != "True":
		// "Incorrect IMDb ID." and "Error getting data." both mean OMDb has
		// nothing under the id.
		return nil, nil
	}
	return parseRatings(record), nil
}

// parseRatings reads the ratings out of a record in a fixed source order.
// The Ratings list is preferred; the top-level imdbRating and Metascore
// fields fill in when the list omits a source.
func parseRatings(record omdbRecord) []tvdb.Rating {
	values := map[string]float64{}
	for _, entry := range record.Ratings {
		source, value, ok := parseRatingEntry(entry.Source, entry.Value)
		if ok {
			values[source] = value
		}
	}
	if _, ok := values[tvdb.RatingSourceIMDb]; !ok {
		if value, ok := parseNumber(record.IMDbRating); ok {
			values[tvdb.RatingSourceIMDb] = value
		}
	}
	if _, ok := values[tvdb.RatingSourceMetacritic]; !ok {
		if value, ok := parseNumber(record.Metascore); ok {
			values[tvdb.RatingSourceMetacritic] = value
		}
	}

	var ratings []tvdb.Rating
	for _, source := range []string{
		tvdb.RatingSourceIMDb, tvdb.RatingSourceRottenTomatoes, tvdb.RatingSourceMetacritic,
	} {
		value, ok := values[source]
		if !ok {
			continue
		}
		rating := tvdb.Rating{Source: source, Value: value}
		if source == tvdb.RatingSourceIMDb {
			rating.Votes = parseVotes(record.IMDbVotes)
		}
		ratings = append(ratings, rating)
	}
	return ratings
}

// parseRatingEntry maps one Ratings entry ("Rotten Tomatoes", "94%") to a
// source and value. Unknown sources and "N/A" values are skipped.
func parseRatingEntry(source, raw string) (string, float64, bool) {
	var name string
	switch source {
	case "Internet Movie Database":
		name = tvdb.RatingSourceIMDb
	case "Rotten Tomatoes":
		name = tvdb.RatingSourceRottenTomatoes
	case "Metacritic":
		name = tvdb.RatingSourceMetacritic
	default:
		return "", 0, false
	}
	// "8.5/10", "94%", "80/100": the number before the scale.
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "/%"); i >= 0 {
		raw = raw[:i]
	}
	value, ok := parseNumber(raw)
	return name, value, ok
}

// parseNumber parses an OMDb numeric string; "N/A" and "" are not numbers.
func parseNumber(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "N/A" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

// parseVotes parses "1,234,567"; 0 when unknown.
func parseVotes(raw string) int {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), ",", "")
	if raw == "" || raw == "N/A" {
		return 0
	}
	votes, err := strconv.Atoi(raw)
	if err != nil || votes < 0 {
		return 0
	}
	return votes
}
