package popular

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// baseRecord is the short series/movie record TVDB returns from
// /series/{id}, /movies/{id}, the /filter endpoints and remote-id lookups.
// Series carry the aired dates and an overview; movies carry a runtime.
type baseRecord struct {
	ID               int         `json:"id"`
	Name             string      `json:"name"`
	Slug             string      `json:"slug"`
	Image            string      `json:"image"`
	Year             string      `json:"year"`
	Score            float64     `json:"score"`
	Status           tvdb.Status `json:"status"`
	Overview         string      `json:"overview"`
	FirstAired       string      `json:"firstAired"`
	LastAired        string      `json:"lastAired"`
	OriginalCountry  string      `json:"originalCountry"`
	OriginalLanguage string      `json:"originalLanguage"`
	Runtime          int         `json:"runtime"`
	AverageRuntime   int         `json:"averageRuntime"`
	// NameTranslations lists the languages TVDB has a name in.
	NameTranslations []string `json:"nameTranslations"`
	// imdbID is filled from an extended record's remote ids; base records have none.
	imdbID string
	// popularity is what ranked the record, when something measured it.
	popularity *tvdb.Popularity
}

// hasEnglishName reports whether TVDB can be expected to hold an English
// name: an unknown translation list is taken as yes.
func (r baseRecord) hasEnglishName() bool {
	if len(r.NameTranslations) == 0 {
		return true
	}
	for _, lang := range r.NameTranslations {
		if lang == "eng" {
			return true
		}
	}
	return false
}

// records resolves TVDB base records and English names, caching every answer
// and every definite miss (a failed call is simply retried next time, so a
// TVDB hiccup during warm-up cannot pin a wrong name for an hour) and
// bounding how many TVDB calls run at once.
type records struct {
	client        TVDBGetter
	cache         *ttlCache[*baseRecord]
	names         *ttlCache[string]
	extendedCache *ttlCache[*extendedRecord]
	searchCache   *ttlCache[int]
	sem           chan struct{}
	logger        *slog.Logger
}

func newRecords(client TVDBGetter, logger *slog.Logger) *records {
	return &records{
		client:        client,
		cache:         newTTLCache[*baseRecord](),
		names:         newTTLCache[string](),
		extendedCache: newTTLCache[*extendedRecord](),
		searchCache:   newTTLCache[int](),
		sem:           make(chan struct{}, tvdbConcurrency),
		logger:        logger,
	}
}

// get performs one bounded TVDB GET.
func (r *records) get(ctx context.Context, uri string) ([]byte, error) {
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-r.sem }()
	return r.client.Get(ctx, uri)
}

// series returns the base record for a TVDB series id. A nil record with a
// nil error means TVDB has no such series.
func (r *records) series(ctx context.Context, id int) (*baseRecord, error) {
	key := "series:" + strconv.Itoa(id)
	if hit, ok := r.cache.get(key); ok {
		return hit, nil
	}
	body, err := r.get(ctx, "/series/"+strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var response struct {
		Data *baseRecord `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode series %d: %w", id, err)
	}
	if response.Data == nil || response.Data.ID == 0 {
		r.cache.set(key, nil, missTTL)
		return nil, nil
	}
	r.cache.set(key, response.Data, recordTTL)
	return response.Data, nil
}

// movieByIMDb resolves an IMDb title id to TVDB's movie record through the
// remote-id lookup. A nil record with a nil error means TVDB has no movie
// under that id.
func (r *records) movieByIMDb(ctx context.Context, imdbID string) (*baseRecord, error) {
	key := "movie:" + imdbID
	if hit, ok := r.cache.get(key); ok {
		return hit, nil
	}
	body, err := r.get(ctx, "/search/remoteid/"+url.PathEscape(imdbID))
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []struct {
			Movie *baseRecord `json:"movie"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode remote id %s: %w", imdbID, err)
	}
	for _, hit := range response.Data {
		if hit.Movie != nil && hit.Movie.ID != 0 {
			r.cache.set(key, hit.Movie, recordTTL)
			return hit.Movie, nil
		}
	}
	r.cache.set(key, nil, missTTL)
	return nil, nil
}

// englishName returns TVDB's English name for a record, or "" when it has
// none. Used for titles whose original language is not English, which the
// filter endpoints return under their original name.
func (r *records) englishName(ctx context.Context, kind Kind, id int) (string, error) {
	key := string(kind) + ":" + strconv.Itoa(id)
	if hit, ok := r.names.get(key); ok {
		return hit, nil
	}
	collection := "series"
	if kind == KindMovie {
		collection = "movies"
	}
	body, err := r.get(ctx, fmt.Sprintf("/%s/%d/translations/eng", collection, id))
	if err != nil {
		return "", err
	}
	var response struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode translation %s %d: %w", kind, id, err)
	}
	r.names.set(key, response.Data.Name, recordTTL)
	return response.Data.Name, nil
}

// extendedRecord is the part of a TVDB extended record the harvest needs:
// the base fields plus genres and remote ids.
type extendedRecord struct {
	baseRecord
	Genres []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"genres"`
	RemoteIDs []struct {
		ID         string `json:"id"`
		SourceName string `json:"sourceName"`
	} `json:"remoteIds"`
}

func (r extendedRecord) hasGenre(id int) bool {
	for _, genre := range r.Genres {
		if genre.ID == id {
			return true
		}
	}
	return false
}

func (r extendedRecord) imdbID() string {
	for _, remote := range r.RemoteIDs {
		if remote.SourceName == "IMDB" || strings.HasPrefix(remote.ID, "tt") {
			return remote.ID
		}
	}
	return ""
}

// extended fetches the extended record of a series or movie. A nil record
// with a nil error means TVDB has no such record.
func (r *records) extended(ctx context.Context, kind Kind, id int) (*extendedRecord, error) {
	key := "extended:" + string(kind) + ":" + strconv.Itoa(id)
	if hit, ok := r.extendedCache.get(key); ok {
		return hit, nil
	}
	collection := "series"
	if kind == KindMovie {
		collection = "movies"
	}
	body, err := r.get(ctx, fmt.Sprintf("/%s/%d/extended", collection, id))
	if err != nil {
		return nil, err
	}
	var response struct {
		Data *extendedRecord `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode extended %s %d: %w", kind, id, err)
	}
	if response.Data == nil || response.Data.ID == 0 {
		r.extendedCache.set(key, nil, missTTL)
		return nil, nil
	}
	r.extendedCache.set(key, response.Data, recordTTL)
	return response.Data, nil
}

// searchID finds the TVDB id of a title of the given kind, optionally
// narrowed by release year, through TVDB's search index. Only a hit whose
// English or original name is the title counts, and when several do the
// most recent wins: a trending "Ghosts (American TV series)" is the 2021
// show, not a 1995 namesake. 0 means no match.
func (r *records) searchID(ctx context.Context, kind Kind, title, year string) (int, error) {
	key := "search:" + string(kind) + "|" + strings.ToLower(title) + "|" + year
	if hit, ok := r.searchCache.get(key); ok {
		return hit, nil
	}
	query := url.Values{}
	query.Set("query", title)
	query.Set("type", string(kind))
	query.Set("limit", "5")
	if year != "" {
		query.Set("year", year)
	}
	body, err := r.get(ctx, "/search?"+query.Encode())
	if err != nil {
		return 0, err
	}
	var response struct {
		Data []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Year         string `json:"year"`
			Translations struct {
				Eng string `json:"eng"`
			} `json:"translations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("decode search %q: %w", title, err)
	}
	want := normalizeTitle(title)
	bestID, bestYear := 0, -1
	for _, hit := range response.Data {
		if normalizeTitle(hit.Translations.Eng) != want && normalizeTitle(hit.Name) != want {
			continue
		}
		parts := strings.SplitN(hit.ID, "-", 2)
		if len(parts) != 2 {
			continue
		}
		id, err := strconv.Atoi(parts[1])
		if err != nil || id == 0 {
			continue
		}
		hitYear, _ := strconv.Atoi(hit.Year)
		if hitYear > bestYear {
			bestID, bestYear = id, hitYear
		}
	}
	r.searchCache.set(key, bestID, recordTTL)
	return bestID, nil
}
