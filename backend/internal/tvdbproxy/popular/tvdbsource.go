package popular

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// Ranking parameters for the TVDB-only source. TVDB's score is a lifetime
// total, so it is weighted towards recent releases:
//
//   - A series is weighted by seriesDecay to the power of its age in years
//     (first-aired year), so a hit from this year outranks a long-runner
//     with ten times the lifetime score. A series that has aired, or is
//     scheduled to air, within airingWindow gets at least airingFloor so a
//     popular show in the middle of a new season stays in the row.
//   - Movie scores grow with age far faster than series scores (a 2024
//     release has ten times the score of a 2025 one a year later), so a
//     movie's score is normalised against the best-scored release of its
//     own year and then decayed by movieDecay per year of age. Only the
//     current and previous release year are kept: "popular now" for a movie
//     means the titles still in their theatrical and streaming window, and
//     the year filter also returns re-releases of older films.
const (
	seriesDecay  = 0.5
	airingFloor  = 0.02
	airingWindow = 45 * 24 * time.Hour
	movieDecay   = 0.5
	// seriesYears and movieYears are how many release years candidates are
	// drawn from (the current year counts as one); series also draw from
	// every continuing title.
	seriesYears = 2
	movieYears  = 2
	// statusContinuing is TVDB's series status id for an ongoing show.
	statusContinuing = 1
	// attentionPool is how many of the TVDB-ranked titles are re-ordered by
	// current attention when an Attention source is configured.
	attentionPool = 50
)

// region is a TVDB filter country and original-language pair.
type region struct{ country, lang string }

var (
	// englishRegions feed every row but Anime: US releases plus UK ones,
	// which the old usa-only filter left out.
	englishRegions = []region{{"usa", "eng"}, {"gbr", "eng"}}
	// animeRegions put Japanese releases first; TVDB's "Anime" tag on US
	// titles mostly marks Western animation.
	animeRegions = []region{{"jpn", "jpn"}, {"usa", "eng"}}
)

// TVDBSource ranks rows from TVDB alone, for installs without a Trakt
// client id and as the fallback when Trakt fails. With an Attention source
// the best of the TVDB ranking is re-ordered by what is being looked at this
// week, and trending titles TVDB users have not scored yet are added.
type TVDBSource struct {
	client    TVDBGetter
	genres    GenreLookup
	records   *records
	attention Attention
	anime     AnimeTrends
	now       func() time.Time
	logger    *slog.Logger
}

// NewTVDBSource returns a TVDB-only source.
func NewTVDBSource(client TVDBGetter, genres GenreLookup, logger *slog.Logger) *TVDBSource {
	return &TVDBSource{
		client:  client,
		genres:  genres,
		records: newRecords(client, logger),
		now:     time.Now,
		logger:  logger,
	}
}

// SetAttention orders the TVDB ranking by attention from a; nil turns it off.
func (s *TVDBSource) SetAttention(a Attention) { s.attention = a }

// SetAnimeTrends leads the Anime rows with what a reports; nil turns it off.
func (s *TVDBSource) SetAnimeTrends(a AnimeTrends) { s.anime = a }

// Name implements Source.
func (s *TVDBSource) Name() string {
	if s.attention != nil {
		return "tvdb recent releases by wikipedia attention"
	}
	return "tvdb recency-weighted score"
}

// Rank implements Source.
func (s *TVDBSource) Rank(ctx context.Context, kind Kind, genre string, limit int) ([]tvdb.Media, error) {
	limit = clampLimit(limit)
	genreID := 0
	if genre != "" {
		id, ok := s.genres.LookupID(genre)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownGenre, genre)
		}
		genreID = id
	}
	isAnime := strings.EqualFold(strings.TrimSpace(genre), "anime")
	regions := englishRegions
	if isAnime {
		regions = animeRegions
	}
	today := s.now()

	candidates, err := s.candidates(ctx, kind, regions, genreID, today.Year())
	if err != nil {
		return nil, err
	}
	var ranked []baseRecord
	if kind == KindMovie {
		ranked = rankMovies(candidates, today)
	} else {
		ranked = rankSeries(candidates, today)
	}
	if s.attention != nil {
		ranked = s.rankByAttention(ctx, kind, genreID, ranked, today)
	}
	if isAnime && s.anime != nil {
		ranked = s.leadWithAnime(ctx, kind, ranked, limit, today)
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	results := s.toMedia(ctx, kind, ranked, isAnime)
	s.logger.InfoContext(ctx, "TVDB row ranked", telemetry.WithTraceContext(ctx,
		"kind", string(kind), "genre", genre, "candidates", len(candidates), "count", len(results))...)
	return results, nil
}

// filterQuery is one TVDB /filter call.
type filterQuery struct {
	region region
	year   int // 0 for no year filter
	status int // 0 for no status filter
}

func (q filterQuery) uri(kind Kind, genreID int) string {
	collection := "series"
	if kind == KindMovie {
		collection = "movies"
	}
	uri := fmt.Sprintf("/%s/filter?country=%s&lang=%s&sort=score&sortType=desc",
		collection, q.region.country, q.region.lang)
	if genreID != 0 {
		uri += "&genre=" + strconv.Itoa(genreID)
	}
	if q.year != 0 {
		uri += "&year=" + strconv.Itoa(q.year)
	}
	if q.status != 0 {
		uri += "&status=" + strconv.Itoa(q.status)
	}
	return uri
}

// queries lists the filter calls that feed a row: the last few release
// years per region, plus every continuing series.
func queries(kind Kind, regions []region, year int) []filterQuery {
	var out []filterQuery
	for _, r := range regions {
		if kind == KindMovie {
			for back := 0; back < movieYears; back++ {
				out = append(out, filterQuery{region: r, year: year - back})
			}
			continue
		}
		for back := 0; back < seriesYears; back++ {
			out = append(out, filterQuery{region: r, year: year - back})
		}
		out = append(out, filterQuery{region: r, status: statusContinuing})
	}
	return out
}

// candidates runs every query in parallel and merges the records by id.
// A query that fails is skipped; only when all of them fail is the error returned.
func (s *TVDBSource) candidates(
	ctx context.Context, kind Kind, regions []region, genreID, year int,
) ([]baseRecord, error) {
	qs := queries(kind, regions, year)
	pages := make([][]baseRecord, len(qs))
	errs := make([]error, len(qs))
	var wg sync.WaitGroup
	for i, q := range qs {
		wg.Add(1)
		go func(i int, q filterQuery) {
			defer wg.Done()
			pages[i], errs[i] = s.filter(ctx, q.uri(kind, genreID))
		}(i, q)
	}
	wg.Wait()

	seen := map[int]bool{}
	var merged []baseRecord
	var lastErr error
	for i, page := range pages {
		if errs[i] != nil {
			lastErr = errs[i]
			s.logger.WarnContext(ctx, "TVDB filter query failed", telemetry.WithTraceContext(ctx,
				"uri", qs[i].uri(kind, genreID), "error", errs[i])...)
			continue
		}
		for _, rec := range page {
			if rec.ID == 0 || seen[rec.ID] {
				continue
			}
			seen[rec.ID] = true
			// Movie filter records carry no originalLanguage; the query's
			// language says what the name is in.
			if rec.OriginalLanguage == "" {
				rec.OriginalLanguage = qs[i].region.lang
			}
			merged = append(merged, rec)
		}
	}
	if len(merged) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return merged, nil
}

func (s *TVDBSource) filter(ctx context.Context, uri string) ([]baseRecord, error) {
	body, err := s.records.get(ctx, uri)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []baseRecord `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode %s: %w", uri, err)
	}
	return response.Data, nil
}

type scored struct {
	rec    baseRecord
	weight float64
}

func sortScored(items []scored) []baseRecord {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].weight != items[j].weight {
			return items[i].weight > items[j].weight
		}
		if items[i].rec.Score != items[j].rec.Score {
			return items[i].rec.Score > items[j].rec.Score
		}
		return items[i].rec.ID < items[j].rec.ID
	})
	out := make([]baseRecord, len(items))
	for i, item := range items {
		out[i] = item.rec
	}
	return out
}

// rankSeries orders series by lifetime score decayed by age, keeping shows
// that are airing now afloat. Series that have not started yet are dropped.
func rankSeries(recs []baseRecord, today time.Time) []baseRecord {
	var items []scored
	for _, rec := range recs {
		first, ok := parseDate(rec.FirstAired)
		if !ok || first.After(today) || strings.EqualFold(rec.Status.Name, "Upcoming") {
			continue
		}
		age := today.Year() - first.Year()
		if age < 0 {
			age = 0
		}
		weight := math.Pow(seriesDecay, float64(age))
		if last, ok := parseDate(rec.LastAired); ok && !last.Before(today.Add(-airingWindow)) {
			weight = math.Max(weight, airingFloor)
		}
		items = append(items, scored{rec: rec, weight: rec.Score * weight})
	}
	return sortScored(items)
}

// rankMovies orders released movies by their score relative to the best of
// their release year, decayed by age. Unreleased movies, and movies older
// than the window (TVDB's year filter also returns re-releases), are dropped.
func rankMovies(recs []baseRecord, today time.Time) []baseRecord {
	type candidate struct {
		rec  baseRecord
		year int
	}
	var candidates []candidate
	yearMax := map[int]float64{}
	oldest := today.Year() - movieYears + 1
	for _, rec := range recs {
		year, _ := strconv.Atoi(rec.Year)
		if year < oldest || year > today.Year() || !movieReleased(rec.Status) {
			continue
		}
		candidates = append(candidates, candidate{rec: rec, year: year})
		if rec.Score > yearMax[year] {
			yearMax[year] = rec.Score
		}
	}
	var items []scored
	for _, c := range candidates {
		relative := 0.0
		if yearMax[c.year] > 0 {
			relative = c.rec.Score / yearMax[c.year]
		}
		age := today.Year() - c.year
		items = append(items, scored{rec: c.rec, weight: relative * math.Pow(movieDecay, float64(age))})
	}
	return sortScored(items)
}

// movieReleased reads TVDB's movie status; an unknown status is taken as released.
func movieReleased(status tvdb.Status) bool {
	return status.Name == "" || strings.EqualFold(status.Name, "Released")
}

func parseDate(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// displayNames returns each record's English name, fetching the translation
// for titles whose original language is not English (anime comes back under
// Japanese names) and keeping the original when TVDB has none.
func (s *TVDBSource) displayNames(ctx context.Context, kind Kind, recs []baseRecord) []string {
	names := make([]string, len(recs))
	var wg sync.WaitGroup
	for i, rec := range recs {
		names[i] = rec.Name
		if rec.OriginalLanguage == "" || rec.OriginalLanguage == "eng" || !rec.hasEnglishName() {
			continue
		}
		wg.Add(1)
		go func(i int, rec baseRecord) {
			defer wg.Done()
			english, err := s.records.englishName(ctx, kind, rec.ID)
			if err != nil {
				s.logger.WarnContext(ctx, "English name lookup failed", telemetry.WithTraceContext(ctx,
					"kind", string(kind), "tvdb_id", rec.ID, "error", err)...)
				return
			}
			if english != "" {
				names[i] = english
			}
		}(i, rec)
	}
	wg.Wait()
	return names
}

// attended is a record with its measured attention.
type attended struct {
	rec   baseRecord
	views int
	known bool
}

// rankByAttention takes the best attentionPool titles of the TVDB ranking,
// adds the trending titles of the kind that TVDB's year filters missed, and
// orders everything measured by attention, most looked-at first. Titles with
// no article keep the TVDB order after the measured ones.
func (s *TVDBSource) rankByAttention(
	ctx context.Context, kind Kind, genreID int, ranked []baseRecord, today time.Time,
) []baseRecord {
	pool := ranked
	if len(pool) > attentionPool {
		pool = pool[:attentionPool]
	}
	seen := make(map[int]bool, len(pool))
	for _, rec := range pool {
		seen[rec.ID] = true
	}

	names := s.displayNames(ctx, kind, pool)
	titles := make([]Title, len(pool))
	for i, rec := range pool {
		titles[i] = Title{Name: names[i], Year: rec.Year}
	}
	var (
		measured []Measurement
		wg       sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		measured = s.attention.Measure(ctx, kind, titles)
	}()
	harvested := s.harvest(ctx, kind, genreID, today, seen)
	wg.Wait()

	items := make([]attended, len(pool))
	for i, rec := range pool {
		if measured[i].Found {
			rec.popularity = wikipediaPopularity(measured[i].Views, measured[i].Article)
		}
		items[i] = attended{rec: rec, views: measured[i].Views, known: measured[i].Found}
	}

	var known, unknown []attended
	for _, item := range items {
		if item.known {
			known = append(known, item)
		} else {
			unknown = append(unknown, item)
		}
	}
	known = append(known, harvested...)
	sort.SliceStable(known, func(i, j int) bool { return known[i].views > known[j].views })

	out := make([]baseRecord, 0, len(known)+len(unknown))
	for _, item := range known {
		out = append(out, item.rec)
	}
	for _, item := range unknown {
		out = append(out, item.rec)
	}
	s.logger.InfoContext(ctx, "Popular row ordered by attention", telemetry.WithTraceContext(ctx,
		"kind", string(kind), "measured", len(known)-len(harvested), "unmeasured", len(unknown),
		"harvested", len(harvested))...)
	return out
}

// harvest resolves the kind's trending articles to TVDB records that are
// not already in the pool, released or aired, and in the row's genre.
func (s *TVDBSource) harvest(
	ctx context.Context, kind Kind, genreID int, today time.Time, seen map[int]bool,
) []attended {
	trending := s.attention.Trending(ctx, kind)
	found := make([]*attended, len(trending))
	var wg sync.WaitGroup
	for i, article := range trending {
		wg.Add(1)
		go func(i int, article TrendingArticle) {
			defer wg.Done()
			found[i] = s.harvestOne(ctx, kind, genreID, today, article)
		}(i, article)
	}
	wg.Wait()

	var out []attended
	for _, item := range found {
		if item == nil || seen[item.rec.ID] {
			continue
		}
		seen[item.rec.ID] = true
		out = append(out, *item)
	}
	return out
}

func (s *TVDBSource) harvestOne(
	ctx context.Context, kind Kind, genreID int, today time.Time, article TrendingArticle,
) *attended {
	id, err := s.records.searchID(ctx, kind, article.Title, article.Year)
	if err != nil {
		s.logger.WarnContext(ctx, "Trending title lookup failed", telemetry.WithTraceContext(ctx,
			"kind", string(kind), "title", article.Title, "error", err)...)
		return nil
	}
	if id == 0 {
		return nil
	}
	ext, err := s.records.extended(ctx, kind, id)
	if err != nil || ext == nil {
		return nil
	}
	if genreID != 0 && !ext.hasGenre(genreID) {
		return nil
	}
	rec := ext.baseRecord
	rec.imdbID = ext.imdbID()
	if !current(kind, rec, today) {
		return nil
	}
	views, ok := s.attention.ArticleViews(ctx, article.Article)
	if !ok {
		views = article.Views
	}
	rec.popularity = wikipediaPopularity(views, article.Article)
	return &attended{rec: rec, views: views, known: true}
}

func wikipediaPopularity(views int, article string) *tvdb.Popularity {
	return &tvdb.Popularity{Source: "wikipedia", Value: views, Detail: article + ", views over 7 days"}
}

// animeLeadShare is the share of an Anime row that AniList's trending list may fill.
const animeLeadShare = 2

// leadWithAnime puts the anime that AniList reports as trending at the head
// of an Anime row, in AniList's order and up to half the row, ahead of the
// attention-ranked titles. Titles TVDB does not know, or that have not been
// released, are skipped.
func (s *TVDBSource) leadWithAnime(
	ctx context.Context, kind Kind, ranked []baseRecord, limit int, today time.Time,
) []baseRecord {
	trends, err := s.anime.Trending(ctx, kind)
	if err != nil {
		s.logger.WarnContext(ctx, "AniList trending failed, Anime row keeps the TVDB ranking",
			telemetry.WithTraceContext(ctx, "kind", string(kind), "error", err)...)
		return ranked
	}
	maxLead := limit / animeLeadShare
	if maxLead < 1 {
		maxLead = 1
	}
	if len(trends) > maxLead*2 {
		trends = trends[:maxLead*2]
	}

	resolved := make([]*baseRecord, len(trends))
	var wg sync.WaitGroup
	for i, trend := range trends {
		wg.Add(1)
		go func(i int, trend AnimeTrend) {
			defer wg.Done()
			resolved[i] = s.resolveAnime(ctx, kind, trend, i+1, today)
		}(i, trend)
	}
	wg.Wait()

	seen := map[int]bool{}
	var lead []baseRecord
	for _, rec := range resolved {
		if rec == nil || seen[rec.ID] || len(lead) == maxLead {
			continue
		}
		seen[rec.ID] = true
		lead = append(lead, *rec)
	}
	out := make([]baseRecord, 0, len(lead)+len(ranked))
	out = append(out, lead...)
	for _, rec := range ranked {
		if !seen[rec.ID] {
			out = append(out, rec)
		}
	}
	s.logger.InfoContext(ctx, "Anime row led by AniList trending", telemetry.WithTraceContext(ctx,
		"kind", string(kind), "trending", len(trends), "lead", len(lead))...)
	return out
}

// resolveAnime finds a trending anime on TVDB by its English title, then its
// romaji title, and returns its record when it has been released.
func (s *TVDBSource) resolveAnime(
	ctx context.Context, kind Kind, trend AnimeTrend, rank int, today time.Time,
) *baseRecord {
	id, err := s.records.searchID(ctx, kind, trend.Title, trend.Year)
	if err == nil && id == 0 && trend.Romaji != "" && !strings.EqualFold(trend.Romaji, trend.Title) {
		id, err = s.records.searchID(ctx, kind, trend.Romaji, trend.Year)
	}
	if err != nil {
		s.logger.WarnContext(ctx, "Trending anime lookup failed", telemetry.WithTraceContext(ctx,
			"kind", string(kind), "title", trend.Title, "error", err)...)
		return nil
	}
	if id == 0 {
		return nil
	}
	ext, err := s.records.extended(ctx, kind, id)
	if err != nil || ext == nil {
		return nil
	}
	rec := ext.baseRecord
	rec.imdbID = ext.imdbID()
	if !released(kind, rec, today) {
		return nil
	}
	rec.popularity = &tvdb.Popularity{Source: "anilist", Value: trend.Trending,
		Detail: fmt.Sprintf("AniList trending #%d", rank)}
	return &rec
}

// released reports whether a title can be downloaded at all: a released
// movie, or a series that has started airing.
func released(kind Kind, rec baseRecord, today time.Time) bool {
	if kind == KindMovie {
		return movieReleased(rec.Status)
	}
	first, ok := parseDate(rec.FirstAired)
	return ok && !first.After(today) && !strings.EqualFold(rec.Status.Name, "Upcoming")
}

// current reports whether a trending title belongs in a row of what is
// popular now: a released movie from the last movieYears release years, or
// a series that has started and either began within the last seriesYears
// years or has aired within airingWindow. The limit keeps Halloween-week
// classics and mismatched namesakes out.
func current(kind Kind, rec baseRecord, today time.Time) bool {
	if kind == KindMovie {
		year, _ := strconv.Atoi(rec.Year)
		return movieReleased(rec.Status) && year >= today.Year()-movieYears+1 && year <= today.Year()
	}
	first, ok := parseDate(rec.FirstAired)
	if !ok || first.After(today) || strings.EqualFold(rec.Status.Name, "Upcoming") {
		return false
	}
	if first.Year() >= today.Year()-seriesYears+1 {
		return true
	}
	last, ok := parseDate(rec.LastAired)
	return ok && !last.Before(today.Add(-airingWindow))
}

// toMedia converts ranked records to Media under their English names.
func (s *TVDBSource) toMedia(ctx context.Context, kind Kind, recs []baseRecord, isAnime bool) []tvdb.Media {
	names := s.displayNames(ctx, kind, recs)

	results := make([]tvdb.Media, 0, len(recs))
	for i, rec := range recs {
		runtime := rec.Runtime
		if runtime == 0 {
			runtime = rec.AverageRuntime
		}
		popularity := rec.popularity
		if popularity == nil {
			popularity = &tvdb.Popularity{Source: "tvdb", Value: int(rec.Score), Detail: "lifetime score"}
		}
		results = append(results, tvdb.Media{
			Id:               strconv.Itoa(rec.ID),
			Name:             names[i],
			Category:         string(kind),
			Anime:            isAnime,
			Score:            rec.Score,
			Popularity:       popularity,
			Slug:             rec.Slug,
			ImageUrl:         artworkURL(rec.Image),
			OriginalName:     rec.Name,
			Status:           rec.Status,
			Overview:         rec.Overview,
			Year:             rec.Year,
			Aliases:          []string{},
			Runtime:          runtime,
			OriginalCountry:  rec.OriginalCountry,
			OriginalLanguage: rec.OriginalLanguage,
			ImdbID:           rec.imdbID,
			Metadata:         tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}},
		})
	}
	return results
}
