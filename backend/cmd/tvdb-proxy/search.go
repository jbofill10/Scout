package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/search"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

const (
	// maxSearchResults bounds the merged result list, and with it the number
	// of per-series episode lookups one search can trigger.
	maxSearchResults = 60
	// maxCreditsPerPerson bounds how much of a prolific actor's filmography
	// is folded into one search.
	maxCreditsPerPerson = 40
	// maxPeoplePerSearch bounds how many TVDB people records are expanded
	// for one name; duplicates of the same person are common on TVDB.
	maxPeoplePerSearch = 3
	// episodeFetchConcurrency caps parallel episode lookups per search.
	episodeFetchConcurrency = 20
	peopleCacheTTL          = time.Hour
)

// Result tiers, lowest first. Ranking sorts by tier, then by TVDB score.
// Credits occupy a band: tierCredits plus the credit's rank (see creditRank),
// so a person's main work sorts ahead of their guest spots and side jobs.
const (
	tierExactTitle = 0  // title or alias equals the query
	tierCredits    = 10 // credits of a person whose name is the query
	tierTitle      = 20 // the rest of the title search
	tierAltTitle   = 30 // hits for the title with its trailing year kept
)

// credit roles TVDB uses in peopleType
const (
	roleActor     = "Actor"
	roleGuestStar = "Guest Star"
	roleDirector  = "Director"
)

var (
	peopleSearchCache  = newTTLCache[[]tvdb.TVDBSearchItem](peopleCacheTTL)
	peopleCreditsCache = newTTLCache[tvdb.TVDBPeopleExtended](peopleCacheTTL)
)

type candidate struct {
	media tvdb.Media
	tier  int
}

// searchMedia answers a structured query with ranked Media. It fans out to
// TVDB's search index (with whatever filters it supports natively), to people
// credits when the query names a person, or to a remote-id lookup for an IMDb
// id, then fetches episodes for every series so counts and schedules work.
func searchMedia(ctx context.Context, q search.Query, mediaType string) ([]tvdb.Media, error) {
	if q.ImdbID != "" {
		return searchByImdbID(ctx, q.ImdbID, mediaType)
	}

	var candidates []candidate
	var err error
	if personName, role, ok := explicitPerson(q, mediaType); ok {
		candidates, err = creditCandidates(ctx, q, personName, role, mediaType, true)
	} else {
		candidates, err = titleCandidates(ctx, q, mediaType)
	}
	if err != nil {
		return nil, err
	}

	candidates = dedupe(candidates)
	if len(candidates) > maxSearchResults {
		candidates = candidates[:maxSearchResults]
	}

	results := make([]tvdb.Media, len(candidates))
	for idx, cand := range candidates {
		results[idx] = cand.media
	}
	fetchSeriesEpisodes(ctx, results)
	for idx := range results {
		candidates[idx].media = results[idx]
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].tier != candidates[j].tier {
			return candidates[i].tier < candidates[j].tier
		}
		if candidates[i].media.Score != candidates[j].media.Score {
			return candidates[i].media.Score > candidates[j].media.Score
		}
		if candidates[i].tier >= tierCredits && candidates[i].tier < tierTitle {
			return candidates[i].media.Year > candidates[j].media.Year
		}
		return false
	})
	for idx, cand := range candidates {
		results[idx] = cand.media
	}
	return results, nil
}

// explicitPerson reports whether the query must be answered from a person's
// credits: an actor:/person: filter always, and a director: filter when TVDB's
// own director filter cannot be used (series, or no title to search with).
func explicitPerson(q search.Query, mediaType string) (name, role string, ok bool) {
	if q.Person != "" {
		role = ""
		if q.PersonRole == search.RoleActor {
			role = roleActor
		}
		return q.Person, role, true
	}
	if q.Director != "" && (mediaType != "movie" || q.Title == "") {
		return q.Director, roleDirector, true
	}
	return "", "", false
}

// titleCandidates runs the title search, the verbatim retry when a year was
// inferred, and the implicit person lookup, and tiers the results.
func titleCandidates(ctx context.Context, q search.Query, mediaType string) ([]candidate, error) {
	if q.Title == "" {
		// Only filters that need a title to go with them (year, studio, ...).
		return []candidate{}, nil
	}

	hits, err := tvdbSearch(ctx, q.Title, mediaType, q)
	if err != nil {
		return nil, err
	}
	candidates := tierTitleHits(hits, q.Title, tierExactTitle, tierTitle)

	if q.AltTitle != "" {
		verbatim := q
		verbatim.Year = ""
		altHits, err := tvdbSearch(ctx, q.AltTitle, mediaType, verbatim)
		if err != nil {
			logger.WarnContext(ctx, "Verbatim title search failed, keeping year-filtered results",
				telemetry.WithTraceContext(ctx, "title", q.AltTitle, "error", err)...)
		} else {
			candidates = append(candidates, tierTitleHits(altHits, q.AltTitle, tierExactTitle, tierAltTitle)...)
		}
	}

	credits, err := creditCandidates(ctx, q, q.Title, "", mediaType, false)
	if err != nil {
		logger.WarnContext(ctx, "People lookup failed, returning title matches only",
			telemetry.WithTraceContext(ctx, "name", q.Title, "error", err)...)
	} else {
		candidates = append(candidates, credits...)
	}
	return candidates, nil
}

// tierTitleHits maps search hits to candidates, putting exact title or alias
// matches in exactTier and everything else in restTier.
func tierTitleHits(hits []tvdb.TVDBSearchItem, title string, exactTier, restTier int) []candidate {
	want := search.Normalize(title)
	candidates := make([]candidate, 0, len(hits))
	for _, hit := range hits {
		media, ok := searchItemToMedia(hit)
		if !ok {
			continue
		}
		tier := restTier
		if want != "" && titleMatches(media, want) {
			tier = exactTier
		}
		candidates = append(candidates, candidate{media: media, tier: tier})
	}
	return candidates
}

func titleMatches(media tvdb.Media, want string) bool {
	if search.Normalize(media.Name) == want || search.Normalize(media.OriginalName) == want {
		return true
	}
	for _, alias := range media.Aliases {
		if search.Normalize(alias) == want {
			return true
		}
	}
	return false
}

// tvdbSearch calls TVDB's /search with the filters it supports natively and
// applies the studio filter, which it does not, to the hits.
func tvdbSearch(ctx context.Context, title, mediaType string, q search.Query) ([]tvdb.TVDBSearchItem, error) {
	params := url.Values{}
	params.Set("query", title)
	params.Set("type", mediaType)
	if q.Year != "" {
		params.Set("year", q.Year)
	}
	if q.Country != "" {
		params.Set("country", q.Country)
	}
	if q.Language != "" {
		params.Set("language", q.Language)
	}
	switch mediaType {
	case "movie":
		if q.Director != "" {
			params.Set("director", q.Director)
		}
	case "series":
		if q.Company != "" {
			params.Set("network", q.Company)
		}
	}

	body, err := tvDbGet(ctx, "/search?"+params.Encode())
	if err != nil {
		return nil, err
	}
	var response tvdb.TVDBSearchResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	hits := response.Data
	if mediaType == "movie" && q.Company != "" {
		hits = filterByStudio(hits, q.Company)
	}
	logger.InfoContext(ctx, "TVDB search", telemetry.WithTraceContext(ctx,
		"title", title, "media_type", mediaType, "year", q.Year, "count", len(hits))...)
	return hits, nil
}

// filterByStudio keeps movie hits produced by a studio whose name contains
// the wanted name, compared loosely ("a24" matches "A24", "A24 Films").
func filterByStudio(hits []tvdb.TVDBSearchItem, studio string) []tvdb.TVDBSearchItem {
	want := search.Normalize(studio)
	kept := make([]tvdb.TVDBSearchItem, 0, len(hits))
	for _, hit := range hits {
		for _, s := range hit.Studios {
			if strings.Contains(search.Normalize(s), want) {
				kept = append(kept, hit)
				break
			}
		}
	}
	return kept
}

// creditCandidates finds people called name and turns their credits into
// candidates. With explicit set, TVDB's best match is taken even if the name
// differs (the user asked for this person); otherwise only people whose name
// equals the query are expanded, so "Pearl" does not drag in everyone whose
// name contains it. role limits the credits kept: "" means actors and
// directors for an implicit lookup, or any role for an explicit person:.
func creditCandidates(
	ctx context.Context, q search.Query, name, role, mediaType string, explicit bool,
) ([]candidate, error) {
	people, err := searchPeople(ctx, name)
	if err != nil {
		return nil, err
	}
	selected := selectPeople(people, name, explicit)
	if len(selected) == 0 {
		return []candidate{}, nil
	}

	var candidates []candidate
	for _, person := range selected {
		extended, err := peopleCredits(ctx, person.TvdbID)
		if err != nil {
			logger.WarnContext(ctx, "People credits lookup failed",
				telemetry.WithTraceContext(ctx, "person_id", person.TvdbID, "error", err)...)
			continue
		}
		credits := creditsOf(extended, mediaType, role, explicit)
		credits = filterCredits(credits, q, explicit)
		rankCredits(credits)
		sort.SliceStable(credits, func(i, j int) bool {
			if credits[i].rank != credits[j].rank {
				return credits[i].rank < credits[j].rank
			}
			return credits[i].media.Year > credits[j].media.Year
		})
		if len(credits) > maxCreditsPerPerson {
			credits = credits[:maxCreditsPerPerson]
		}
		for _, c := range credits {
			candidates = append(candidates, candidate{media: c.media, tier: tierCredits + c.rank})
		}
	}
	logger.InfoContext(ctx, "People credits", telemetry.WithTraceContext(ctx,
		"name", name, "people", len(selected), "credits", len(candidates), "explicit", explicit)...)
	return candidates, nil
}

// credit is one title from a person's filmography with the strongest role
// they had on it.
type credit struct {
	media    tvdb.Media
	role     string
	featured bool
	rank     int
}

// isActing reports whether a TVDB role is an on-screen one.
func isActing(role string) bool {
	return role == roleActor || role == roleGuestStar
}

// rolePreference orders a person's roles on one title, strongest first, for
// picking which to show when they had several (Cranston acted in and
// directed Breaking Bad; "Actor" is the credit that matters).
func rolePreference(role string, featured bool) int {
	switch {
	case role == roleActor && featured:
		return 0
	case role == roleDirector:
		return 1
	case role == roleActor:
		return 2
	case role == roleGuestStar:
		return 3
	}
	return 4
}

// rankCredits assigns each credit a rank for ordering. The person's dominant
// line of work (acting or directing, by credit count) comes first, main-cast
// roles before supporting ones and guest spots; an actor's occasional
// directing, or a director's cameo, follows; every other role trails. TVDB
// only ever flags acting credits as featured, so that bit cannot rank
// directors on its own.
func rankCredits(credits []credit) {
	acting, directing := 0, 0
	for _, c := range credits {
		switch {
		case isActing(c.role):
			acting++
		case c.role == roleDirector:
			directing++
		}
	}
	directorDominant := directing > acting

	for idx := range credits {
		c := &credits[idx]
		switch {
		case c.role == roleDirector:
			c.rank = 0
			if !directorDominant {
				c.rank = 3
			}
		case isActing(c.role):
			switch {
			case c.role == roleActor && c.featured:
				c.rank = 0
			case c.role == roleActor:
				c.rank = 1
			default:
				c.rank = 2
			}
			if directorDominant {
				c.rank += 3
			}
		default:
			c.rank = 6
		}
	}
}

// selectPeople picks which people hits to expand (see creditCandidates).
func selectPeople(people []tvdb.TVDBSearchItem, name string, explicit bool) []tvdb.TVDBSearchItem {
	want := search.Normalize(name)
	if want == "" {
		return nil
	}
	var selected []tvdb.TVDBSearchItem
	for idx, person := range people {
		if person.TvdbID == "" {
			continue
		}
		exact := search.Normalize(person.OriginalName) == want
		if !exact && !(explicit && idx == 0) {
			continue
		}
		selected = append(selected, person)
		if len(selected) == maxPeoplePerSearch {
			break
		}
	}
	return selected
}

// creditsOf maps a person's credits of the wanted type and role onto one
// credit per title, keeping the strongest role when they had several. Role ""
// keeps actors and directors unless explicit, when every role counts.
func creditsOf(person tvdb.TVDBPeopleExtended, mediaType, role string, explicit bool) []credit {
	byTitle := make(map[string]int) // title id -> index in credits
	var credits []credit
	for _, c := range person.Characters {
		if !roleWanted(c.PeopleType, role, explicit) {
			continue
		}
		var id int
		var record *tvdb.TVDBRecordInfo
		switch mediaType {
		case "movie":
			if c.MovieId == nil || c.Movie == nil {
				continue
			}
			id, record = *c.MovieId, c.Movie
		default:
			if c.SeriesId == nil || c.Series == nil {
				continue
			}
			id, record = *c.SeriesId, c.Series
		}
		if record.Name == "" {
			continue
		}
		key := strconv.Itoa(id)
		if idx, seen := byTitle[key]; seen {
			existing := &credits[idx]
			if rolePreference(c.PeopleType, c.IsFeatured) < rolePreference(existing.role, existing.featured) {
				existing.role, existing.featured = c.PeopleType, c.IsFeatured
				existing.media.MatchedVia = person.Name + " · " + c.PeopleType
			}
			continue
		}
		byTitle[key] = len(credits)
		credits = append(credits, credit{
			role:     c.PeopleType,
			featured: c.IsFeatured,
			media: tvdb.Media{
				Id:           key,
				Name:         record.Name,
				OriginalName: record.Name,
				Category:     mediaType,
				ImageUrl:     absoluteArtworkURL(record.Image),
				Year:         record.Year,
				Aliases:      []string{},
				MatchedVia:   person.Name + " · " + c.PeopleType,
				Metadata:     tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}},
			},
		})
	}
	return credits
}

func roleWanted(peopleType, role string, explicit bool) bool {
	switch role {
	case roleActor:
		return peopleType == roleActor || peopleType == roleGuestStar
	case roleDirector:
		return peopleType == roleDirector
	case "":
		if explicit {
			return true
		}
		return peopleType == roleActor || peopleType == roleDirector
	}
	return peopleType == role
}

// filterCredits applies the rest of the query to a person's credits. A title
// typed alongside an explicit person narrows their credits to titles
// containing it; a year keeps that year only. Studio, country and language
// cannot be applied here because credits do not carry them.
func filterCredits(credits []credit, q search.Query, explicit bool) []credit {
	wantTitle := ""
	if explicit {
		wantTitle = search.Normalize(q.Title)
	}
	kept := make([]credit, 0, len(credits))
	for _, c := range credits {
		if q.Year != "" && c.media.Year != q.Year {
			continue
		}
		if wantTitle != "" && !strings.Contains(search.Normalize(c.media.Name), wantTitle) {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}

// searchPeople looks a name up in TVDB's people index, cached per name.
func searchPeople(ctx context.Context, name string) ([]tvdb.TVDBSearchItem, error) {
	key := search.Normalize(name)
	if people, ok := peopleSearchCache.get(key); ok {
		return people, nil
	}
	params := url.Values{}
	params.Set("query", name)
	params.Set("type", "person")
	body, err := tvDbGet(ctx, "/search?"+params.Encode())
	if err != nil {
		return nil, err
	}
	var response tvdb.TVDBSearchResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode people search response: %w", err)
	}
	peopleSearchCache.set(key, response.Data)
	return response.Data, nil
}

// peopleCredits fetches a person's extended record, cached per id.
func peopleCredits(ctx context.Context, personID string) (tvdb.TVDBPeopleExtended, error) {
	if person, ok := peopleCreditsCache.get(personID); ok {
		return person, nil
	}
	body, err := tvDbGet(ctx, "/people/"+url.PathEscape(personID)+"/extended")
	if err != nil {
		return tvdb.TVDBPeopleExtended{}, err
	}
	var response tvdb.TVDBPeopleExtendedResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return tvdb.TVDBPeopleExtended{}, fmt.Errorf("decode people extended response: %w", err)
	}
	peopleCreditsCache.set(personID, response.Data)
	return response.Data, nil
}

// searchByImdbID resolves an IMDb title id through TVDB's remote-id lookup.
// A match of the other media type is dropped: the UI's download actions are
// bound to the selected type, so the user is told to switch instead.
func searchByImdbID(ctx context.Context, imdbID, mediaType string) ([]tvdb.Media, error) {
	body, err := tvDbGet(ctx, "/search/remoteid/"+url.PathEscape(imdbID))
	if err != nil {
		return nil, err
	}
	var response tvdb.TVDBRemoteIDSearchResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode remote id response: %w", err)
	}

	results := []tvdb.Media{}
	for _, hit := range response.Data {
		var record *tvdb.TVDBBaseRecord
		switch mediaType {
		case "movie":
			record = hit.Movie
		case "series":
			record = hit.Series
		}
		if record == nil || record.Id == 0 {
			continue
		}
		media := tvdb.Media{
			Id:           strconv.Itoa(record.Id),
			Name:         record.Name,
			OriginalName: record.Name,
			Category:     mediaType,
			Slug:         record.Slug,
			ImageUrl:     absoluteArtworkURL(record.Image),
			Year:         record.Year,
			Status:       record.Status,
			Score:        record.Score,
			Aliases:      []string{},
			ImdbID:       imdbID,
			Metadata:     tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}},
		}
		results = append(results, media)
	}
	fetchSeriesEpisodes(ctx, results)
	logger.InfoContext(ctx, "IMDb lookup", telemetry.WithTraceContext(ctx,
		"imdb_id", imdbID, "media_type", mediaType, "count", len(results))...)
	return results, nil
}

// fetchSeriesEpisodes fills in episodes and score for every series in results,
// in place and in parallel. Movies are left alone. A failed lookup leaves that
// series with an empty episode list rather than dropping it.
func fetchSeriesEpisodes(ctx context.Context, results []tvdb.Media) {
	sem := make(chan struct{}, episodeFetchConcurrency)
	var wg sync.WaitGroup
	for idx := range results {
		if results[idx].Category != "series" {
			if results[idx].Metadata.Episodes == nil {
				results[idx].Metadata.Episodes = []tvdb.Episode{}
			}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(media *tvdb.Media) {
			defer wg.Done()
			defer func() { <-sem }()
			seriesResponse, err := querySeriesMetadata(ctx, media.Id)
			if err != nil {
				logger.WarnContext(ctx, "Failed to fetch episode metadata, returning partial result",
					"media_id", media.Id, "media_name", media.Name, "error", err)
				media.Metadata = tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}
				return
			}
			media.Metadata = seriesResponse.Data
			if media.Metadata.Episodes == nil {
				media.Metadata.Episodes = []tvdb.Episode{}
			}
			media.Score = seriesResponse.Data.Score
		}(&results[idx])
	}
	wg.Wait()
}

// dedupe keeps the first candidate seen for each id, so a title that is both
// an exact match and one of a person's credits appears once, in its best tier.
func dedupe(candidates []candidate) []candidate {
	seen := make(map[string]bool, len(candidates))
	kept := make([]candidate, 0, len(candidates))
	for _, cand := range candidates {
		if cand.media.Id == "" || seen[cand.media.Id] {
			continue
		}
		seen[cand.media.Id] = true
		kept = append(kept, cand)
	}
	return kept
}

// absoluteArtworkURL prefixes TVDB's relative artwork paths.
func absoluteArtworkURL(path string) string {
	if path == "" || strings.HasPrefix(path, "http") {
		return path
	}
	return "https://artworks.thetvdb.com" + path
}

// ttlCache is a small in-memory cache with per-entry expiry, for TVDB
// records that change rarely but are hit on every keystroke.
type ttlCache[V any] struct {
	mu      sync.Mutex
	entries map[string]ttlEntry[V]
	ttl     time.Duration
	now     func() time.Time
}

type ttlEntry[V any] struct {
	value   V
	expires time.Time
}

func newTTLCache[V any](ttl time.Duration) *ttlCache[V] {
	return &ttlCache[V]{entries: make(map[string]ttlEntry[V]), ttl: ttl, now: time.Now}
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || c.now().After(entry.expires) {
		delete(c.entries, key)
		var zero V
		return zero, false
	}
	return entry.value, true
}

func (c *ttlCache[V]) set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.entries) >= 1000 {
		for k, entry := range c.entries {
			if now.After(entry.expires) {
				delete(c.entries, k)
			}
		}
	}
	c.entries[key] = ttlEntry[V]{value: value, expires: now.Add(c.ttl)}
}

func (c *ttlCache[V]) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]ttlEntry[V])
}
