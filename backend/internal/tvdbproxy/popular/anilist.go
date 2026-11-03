package popular

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// AnimeTrends lists what anime fans are watching right now. The Anime rows
// lead with it: English Wikipedia barely measures anime, while AniList's
// trending score is activity among anime watchers over the last days.
type AnimeTrends interface {
	Trending(ctx context.Context, kind Kind) ([]AnimeTrend, error)
}

// AnimeTrend is one trending anime.
type AnimeTrend struct {
	// Title is the English title, or the romaji one when AniList has no English title.
	Title string
	// Romaji is the romanised Japanese title, tried when Title finds nothing on TVDB.
	Romaji string
	// Year is the release year, "" when unknown.
	Year string
	// Trending is AniList's trending score.
	Trending int
}

const (
	// DefaultAniListHost is AniList's public GraphQL endpoint; no key is needed.
	DefaultAniListHost = "https://graphql.anilist.co"
	anilistTimeout     = 15 * time.Second
	// anilistTTL is how long a trending list is kept.
	anilistTTL = 6 * time.Hour
	// anilistPage is how many trending titles are asked for.
	anilistPage = 40
)

// anilistQuery asks for trending anime of the given formats that have
// started airing or are finished; adult titles are left out.
const anilistQuery = `query ($type: MediaType, $formats: [MediaFormat]) {
  Page(perPage: %d) {
    media(type: $type, format_in: $formats, sort: TRENDING_DESC, status_in: [RELEASING, FINISHED], isAdult: false) {
      trending seasonYear startDate { year } title { romaji english }
    }
  }
}`

// AniList is the AnimeTrends source backed by AniList's GraphQL API.
type AniList struct {
	host   string
	http   *http.Client
	cache  *ttlCache[[]AnimeTrend]
	logger *slog.Logger
}

// NewAniListHTTPClient returns the traced HTTP client used for AniList requests.
func NewAniListHTTPClient() *http.Client {
	return &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: anilistTimeout}
}

// NewAniList returns an AniList source for host; empty means DefaultAniListHost.
func NewAniList(host string, httpClient *http.Client, logger *slog.Logger) *AniList {
	if host == "" {
		host = DefaultAniListHost
	}
	return &AniList{
		host:   strings.TrimRight(host, "/"),
		http:   httpClient,
		cache:  newTTLCache[[]AnimeTrend](),
		logger: logger,
	}
}

// Trending implements AnimeTrends: TV formats for series, films for movies.
func (a *AniList) Trending(ctx context.Context, kind Kind) ([]AnimeTrend, error) {
	if hit, ok := a.cache.get(string(kind)); ok {
		return hit, nil
	}
	formats := []string{"TV", "TV_SHORT", "ONA"}
	if kind == KindMovie {
		formats = []string{"MOVIE"}
	}
	payload, err := json.Marshal(map[string]any{
		"query":     fmt.Sprintf(anilistQuery, anilistPage),
		"variables": map[string]any{"type": "ANIME", "formats": formats},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.host, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anilist: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			a.logger.InfoContext(ctx, "Error closing AniList response body", "error", cerr)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anilist: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anilist: status %d", resp.StatusCode)
	}
	var response struct {
		Data struct {
			Page struct {
				Media []struct {
					Trending   int `json:"trending"`
					SeasonYear int `json:"seasonYear"`
					StartDate  struct {
						Year int `json:"year"`
					} `json:"startDate"`
					Title struct {
						Romaji  string `json:"romaji"`
						English string `json:"english"`
					} `json:"title"`
				} `json:"media"`
			} `json:"Page"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("anilist: decode: %w", err)
	}
	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("anilist: %s", response.Errors[0].Message)
	}
	trends := make([]AnimeTrend, 0, len(response.Data.Page.Media))
	for _, m := range response.Data.Page.Media {
		title := m.Title.English
		if title == "" {
			title = m.Title.Romaji
		}
		if title == "" {
			continue
		}
		year := m.SeasonYear
		if year == 0 {
			year = m.StartDate.Year
		}
		trend := AnimeTrend{Title: title, Romaji: m.Title.Romaji, Trending: m.Trending}
		if year > 0 {
			trend.Year = strconv.Itoa(year)
		}
		trends = append(trends, trend)
	}
	a.cache.set(string(kind), trends, anilistTTL)
	return trends, nil
}
