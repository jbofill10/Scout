package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"golift.io/starr/prowlarr"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// packTitle is what the pack parser reads off a release name.
type packTitle struct {
	ShowName   string
	Season     int // 0 when the title names no season
	EpStart    int // episode range, when present
	EpEnd      int
	Complete   bool // "complete" / "batch" wording
	Quality    string
	SingleEp   bool // a single S##E## release, never a pack
	ParseError bool
}

var (
	packSeasonRe   = regexp.MustCompile(`(?i)\b(?:S|Season[ ._-]?)(\d{1,2})(?:E|\b)`)
	packSingleEpRe = regexp.MustCompile(`(?i)\bS\d{1,2}[ ._-]?E\d{1,3}\b`)
	packRangeRe    = regexp.MustCompile(`(?i)\b(?:S\d{1,2})?[ ._-]?E?(\d{1,3})[ ._-]?-[ ._-]?E?(\d{1,3})\b`)
	packQualityRe  = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|4k)\b`)
	packCompleteRe = regexp.MustCompile(`(?i)\b(complete|batch|full season|season pack)\b`)
)

// parsePack reads a season-pack style title. It is deliberately separate from
// the per-episode parser, which returns nothing for titles without an episode.
func parsePack(title string) packTitle {
	pt := packTitle{}
	clean := strings.NewReplacer(".", " ", "_", " ").Replace(title)
	if packSingleEpRe.MatchString(clean) && !packRangeRe.MatchString(clean) {
		pt.SingleEp = true
	}
	if m := packSeasonRe.FindStringSubmatch(clean); m != nil {
		pt.Season, _ = strconv.Atoi(m[1])
	}
	if m := packRangeRe.FindStringSubmatch(clean); m != nil {
		pt.EpStart, _ = strconv.Atoi(m[1])
		pt.EpEnd, _ = strconv.Atoi(m[2])
		if pt.EpEnd < pt.EpStart {
			pt.EpStart, pt.EpEnd = 0, 0
		}
	}
	pt.Complete = packCompleteRe.MatchString(clean)
	if m := packQualityRe.FindStringSubmatch(clean); m != nil {
		pt.Quality = strings.ToLower(m[1])
		if pt.Quality == "4k" {
			pt.Quality = "2160p"
		}
	}
	// The show name is whatever precedes the first season / range / quality marker.
	cut := len(clean)
	for _, re := range []*regexp.Regexp{packSeasonRe, packRangeRe, packQualityRe, packCompleteRe} {
		if loc := re.FindStringIndex(clean); loc != nil && loc[0] < cut {
			cut = loc[0]
		}
	}
	name := strings.TrimSpace(clean[:cut])
	name = strings.TrimLeft(name, "[")
	if i := strings.Index(name, "]"); i >= 0 && i < 20 {
		name = strings.TrimSpace(name[i+1:]) // "[Group] Show" -> "Show"
	}
	name = strings.Trim(name, " -([")
	pt.ShowName = name
	if pt.ShowName == "" {
		pt.ParseError = true
	}
	return pt
}

// packStrategy builds the one strategy a pack download is tracked under: the
// first wanted episode's identity, plus the pack itself.
func (q *QbittHandler) packStrategy(req *tvdb.Media, season int, episodes []tvdb.Episode, query string) *models.SearchStrategy {
	first := episodes[0]
	base := q.createSearchStrategy(req, &first)
	ss := &models.SearchStrategy{MediaName: req.Name, Query: query, Season: season, Episode: first.Number, TvdbId: req.Id,
		EpisodeTvdbID: strconv.Itoa(first.Id), EpisodeMeta: &first}
	if len(base) > 0 {
		ss.ReleaseYear = base[0].ReleaseYear
		ss.ImdbID = base[0].ImdbID
		ss.RuntimeMinutes = base[0].RuntimeMinutes
	}
	eps := append([]tvdb.Episode{}, episodes...)
	ss.Pack = &models.PackInfo{Season: season, Episodes: eps}
	return ss
}

// packQueries are the searches tried for a season pack, strictest first.
func packQueries(req *tvdb.Media, season int) []string {
	names := append([]string{req.Name}, req.Aliases...)
	queries := []string{}
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			queries = append(queries, s)
		}
	}
	for _, name := range names {
		if name == "" {
			continue
		}
		if req.Anime {
			add(fmt.Sprintf("%s batch", name))
			add(fmt.Sprintf("%s season %d", name, season))
			continue
		}
		add(fmt.Sprintf("%s S%02d", name, season))
		add(fmt.Sprintf("%s season %d", name, season))
		add(fmt.Sprintf("%s S%02d complete", name, season))
	}
	if len(queries) > 6 {
		queries = queries[:6]
	}
	return queries
}

// validatePack decides whether a release is a usable pack for the wanted
// episodes: the show matches, the season is named (or an episode range covers
// every wanted number), it is not a single episode, and it clears the quality
// floor. It returns the parsed quality tier for scoring.
func (q *QbittHandler) validatePack(torrent *prowlarr.Search, req *tvdb.Media, season int, wanted []tvdb.Episode) (bool, string, int, float64) {
	pt := parsePack(torrent.Title)
	if pt.ParseError {
		return false, "could not parse title", 0, 0
	}
	if pt.SingleEp {
		return false, "single episode, not a pack", 0, 0
	}
	nameScore := q.scoreNameMatch(pt.ShowName, req.Name)
	for _, alias := range req.Aliases {
		if s := q.scoreNameMatch(pt.ShowName, alias); s > nameScore {
			nameScore = s
		}
	}
	if nameScore < 0.3 {
		return false, fmt.Sprintf("show name mismatch: parsed=%q", pt.ShowName), 0, 0
	}
	covers := func() bool {
		if pt.EpStart == 0 {
			return false
		}
		for _, ep := range wanted {
			n := ep.Number
			if req.Anime && ep.AbsoluteNumber > 0 && pt.Season == 0 {
				n = ep.AbsoluteNumber
			}
			if n < pt.EpStart || n > pt.EpEnd {
				return false
			}
		}
		return true
	}
	switch {
	case pt.Season == season && pt.EpStart == 0:
		// season-only title ("Show S01", "Show Season 1 Complete")
	case pt.Season == season && covers():
	case pt.Season == 0 && (pt.Complete || req.Anime) && covers():
	case pt.Season == 0 && pt.Complete && pt.EpStart == 0 && season == 1 && req.Anime:
		// anime "batch" with no numbering at all: only trust it for season 1
	default:
		return false, fmt.Sprintf("does not cover season %d", season), 0, 0
	}
	tier := QualityTier(pt.Quality)
	if floor := q.qualityFloor(); floor > 0 && tier < floor {
		return false, fmt.Sprintf("%s is below the minimum quality", orLabel(pt.Quality)), tier, nameScore
	}
	return true, "", tier, nameScore
}

func orLabel(q string) string {
	if q == "" {
		return "unknown resolution"
	}
	return q
}

// searchForPack runs the pack queries across the indexer tiers and returns
// the best acceptable pack, or nil.
func (q *QbittHandler) searchForPack(ctx context.Context, req *tvdb.Media, season int, wanted []tvdb.Episode, excluded *excludedTorrents) *models.TorrentMatch {
	type scored struct {
		torrent *prowlarr.Search
		score   float64
		query   string
	}
	var best *scored
	seen := map[string]bool{}
	for _, indexerIDs := range q.indexerTiers(req.Category, req.Anime) {
		for _, query := range packQueries(req, season) {
			if ctx.Err() != nil {
				return nil
			}
			results, err := q.searchProwlarr(ctx, query, req.Category, indexerIDs)
			if err != nil {
				continue
			}
			excluded.drop(&results)
			for _, t := range results {
				key := candidateKey(t)
				if seen[key] {
					continue
				}
				seen[key] = true
				ok, why, tier, nameScore := q.validatePack(t, req, season, wanted)
				if !ok {
					q.logger.DebugContext(ctx, "Pack rejected", "title", t.Title, "reason", why)
					continue
				}
				score := nameScore + calculateYearAwareQualityBoost(tier, "") + seederBoost(t.Seeders)
				if t.Seeders <= suspectSeederCount {
					score -= 1.0 // a dead pack wastes a whole season's wait
				}
				if best == nil || score > best.score {
					best = &scored{torrent: t, score: score, query: query}
				}
			}
		}
		if best != nil && best.torrent.Seeders > suspectSeederCount {
			break // the preferred indexer tier had a live pack
		}
	}
	if best == nil {
		return nil
	}
	q.logger.InfoContext(ctx, "Season pack selected",
		telemetry.WithTraceContext(ctx, "show", req.Name, "season", season, "title", best.torrent.Title,
			"seeders", best.torrent.Seeders, "score", best.score, "episodes", len(wanted))...)
	return &models.TorrentMatch{Strategy: q.packStrategy(req, season, wanted, best.query), Torrent: best.torrent}
}

// tryPacks groups the episodes to download by season and, when packs are on
// and a season has enough of them, downloads one pack per such season. It
// returns the per-episode results for packed episodes and which episodes were
// covered; anything else goes through the per-episode path.
func (q *QbittHandler) tryPacks(ctx context.Context, req *tvdb.Media, episodes []tvdb.Episode, done chan<- models.TorrentCompleteEvent, wg *sync.WaitGroup) ([]dlstatus.EpisodeResult, map[int]bool) {
	enabled, minEpisodes := q.packSettings()
	if !enabled || len(episodes) < minEpisodes {
		return nil, nil
	}
	bySeason := map[int][]tvdb.Episode{}
	for _, ep := range episodes {
		bySeason[ep.SeasonNumber] = append(bySeason[ep.SeasonNumber], ep)
	}
	seasons := make([]int, 0, len(bySeason))
	for s := range bySeason {
		seasons = append(seasons, s)
	}
	sort.Ints(seasons)

	results := []dlstatus.EpisodeResult{}
	packed := map[int]bool{}
	for _, season := range seasons {
		wanted := bySeason[season]
		if len(wanted) < minEpisodes {
			continue
		}
		sort.Slice(wanted, func(i, j int) bool { return wanted[i].Number < wanted[j].Number })
		packCtx, span := torrentTracer.Start(ctx, "searchForSeasonPack")
		span.SetAttributes(attribute.String("show", req.Name), attribute.Int("season", season), attribute.Int("episodes", len(wanted)))
		excluded := q.loadExcludedTorrents(packCtx, req.Id, season, wanted[0].Number)
		match := q.searchForPack(packCtx, req, season, wanted, excluded)
		if match == nil {
			q.logger.InfoContext(packCtx, "No season pack found, searching per episode",
				telemetry.WithTraceContext(packCtx, "show", req.Name, "season", season)...)
			span.End()
			continue
		}
		result := q.initiateDownloadAndMonitor(packCtx, match, searchRequestFor(req, match.Strategy), done, wg)
		span.End()
		if result.Outcome != dlstatus.OutcomeDownloading {
			// The add failed; let the per-episode path have a go.
			continue
		}
		for _, ep := range wanted {
			packed[ep.Id] = true
			results = append(results, dlstatus.EpisodeResult{TvdbID: strconv.Itoa(ep.Id), Season: ep.SeasonNumber, Episode: ep.Number, Outcome: dlstatus.OutcomeDownloading})
		}
	}
	return results, packed
}

// withoutEpisodes drops the episodes whose ids are in packed.
func withoutEpisodes(episodes []tvdb.Episode, packed map[int]bool) []tvdb.Episode {
	out := make([]tvdb.Episode, 0, len(episodes))
	for _, ep := range episodes {
		if !packed[ep.Id] {
			out = append(out, ep)
		}
	}
	return out
}
