package service

import (
	"context"
	"fmt"
	"path/filepath"
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
	ShowName string
	Season   int // first (or only) season named; 0 when the title names none
	// SeasonEnd is the last season of a range such as "S01-S05" or
	// "Seasons 1-5"; 0 when the title names a single season.
	SeasonEnd  int
	EpStart    int // episode range, when present
	EpEnd      int
	Complete   bool // "complete" / "batch" wording
	Series     bool // "complete series" and the like: every season there is
	Quality    string
	SingleEp   bool // a single S##E## release, never a pack
	ParseError bool
}

var (
	packSeasonRangeRe = regexp.MustCompile(
		`(?i)\b(?:S|Seasons?[ ._-]?)(\d{1,2})[ ._-]?(?:-|–|to|and|&|\+)[ ._-]?S?(\d{1,2})\b`)
	packSeasonRe   = regexp.MustCompile(`(?i)\b(?:S|Season[ ._-]?)(\d{1,2})(?:E|\b)`)
	packSingleEpRe = regexp.MustCompile(`(?i)\bS\d{1,2}[ ._-]?E\d{1,3}\b`)
	packRangeRe    = regexp.MustCompile(`(?i)\b(?:S\d{1,2})?[ ._-]?E?(\d{1,3})[ ._-]?-[ ._-]?E?(\d{1,3})\b`)
	packQualityRe  = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|4k)\b`)
	packCompleteRe = regexp.MustCompile(`(?i)\b(complete|batch|full season|season pack)\b`)
	packSeriesRe   = regexp.MustCompile(
		`(?i)\b(complete series|complete collection|complete show|full series|entire series|all seasons|the complete)\b`)
	// packDirSeasonRe matches a season folder inside a pack: "Season 05", "S05", "Season.5".
	packDirSeasonRe = regexp.MustCompile(`(?i)^(?:season|s)[ ._-]?(\d{1,2})$`)
	// packFileRunRe reads the last episode of a run named "S07E01-E02",
	// "S07E01-02" or "S07E01E02" off a file name.
	packFileRunRe = regexp.MustCompile(`(?i)\bS\d{1,2}E(\d{1,3})(?:[ ._-]?(?:-|–)?[ ._-]?E|-)(\d{1,3})\b`)
)

// parsePack reads a season-pack style title. It is deliberately separate from
// the per-episode parser, which returns nothing for titles without an episode.
func parsePack(title string) packTitle {
	pt := packTitle{}
	clean := strings.NewReplacer(".", " ", "_", " ").Replace(title)
	if packSingleEpRe.MatchString(clean) && !packRangeRe.MatchString(clean) {
		pt.SingleEp = true
	}

	// A season range ("S01-S13", "Seasons 1-5", "S08 and S09") is read first
	// and removed, or its digits would pass for an episode range.
	rest := clean
	if m := packSeasonRangeRe.FindStringSubmatch(clean); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if a > b {
			a, b = b, a
		}
		joined := strings.Contains(strings.ToLower(m[0]), "and") || strings.ContainsAny(m[0], "&+")
		if a > 0 && b > a && (!joined || b == a+1) {
			pt.Season, pt.SeasonEnd = a, b
			rest = packSeasonRangeRe.ReplaceAllString(clean, " ")
		}
	}
	if pt.SeasonEnd == 0 {
		if m := packSeasonRe.FindStringSubmatch(rest); m != nil {
			pt.Season, _ = strconv.Atoi(m[1])
		}
	}
	if m := packRangeRe.FindStringSubmatch(rest); m != nil {
		pt.EpStart, _ = strconv.Atoi(m[1])
		pt.EpEnd, _ = strconv.Atoi(m[2])
		if pt.EpEnd < pt.EpStart {
			pt.EpStart, pt.EpEnd = 0, 0
		}
	}
	pt.Series = packSeriesRe.MatchString(clean)
	pt.Complete = pt.Series || packCompleteRe.MatchString(clean)
	if m := packQualityRe.FindStringSubmatch(clean); m != nil {
		pt.Quality = strings.ToLower(m[1])
		if pt.Quality == "4k" {
			pt.Quality = "2160p"
		}
	}
	// The show name is whatever precedes the first season / range / quality marker.
	cut := len(clean)
	for _, re := range []*regexp.Regexp{packSeasonRangeRe, packSeasonRe, packRangeRe, packQualityRe, packCompleteRe, packSeriesRe} {
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

// spannedSeasons is how many seasons a pack title claims: the range it
// names, one for a single season, and 0 when it does not say (a "complete
// series" pack).
func (pt packTitle) spannedSeasons() int {
	switch {
	case pt.SeasonEnd > 0:
		return pt.SeasonEnd - pt.Season + 1
	case pt.Season > 0:
		return 1
	}
	return 0
}

// packStrategy builds the one strategy a pack download is tracked under: the
// first wanted episode's identity, plus the pack itself.
func (q *QbittHandler) packStrategy(req *tvdb.Media, seasons []int, episodes []tvdb.Episode, query string) *models.SearchStrategy {
	first := episodes[0]
	base := q.createSearchStrategy(req, &first)
	ss := &models.SearchStrategy{MediaName: req.Name, Query: query, Season: seasons[0], Episode: first.Number, TvdbId: req.Id,
		EpisodeTvdbID: strconv.Itoa(first.Id), EpisodeMeta: &first}
	if len(base) > 0 {
		ss.ReleaseYear = base[0].ReleaseYear
		ss.ImdbID = base[0].ImdbID
		ss.RuntimeMinutes = base[0].RuntimeMinutes
	}
	eps := append([]tvdb.Episode{}, episodes...)
	ss.Pack = &models.PackInfo{Season: seasons[0], Seasons: append([]int{}, seasons...), Episodes: eps}
	return ss
}

// packQueries are the searches tried for a pack of the given seasons,
// strictest first: one season's pack, or for several seasons the range and
// complete-series forms.
func packQueries(req *tvdb.Media, seasons []int) []string {
	queries := []string{}
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			queries = append(queries, s)
		}
	}
	first, last := seasons[0], seasons[len(seasons)-1]
	for _, name := range searchNames(req) {
		if len(seasons) > 1 {
			add(fmt.Sprintf("%s S%02d-S%02d", name, first, last))
			add(fmt.Sprintf("%s complete series", name))
			if req.Anime {
				add(fmt.Sprintf("%s batch", name))
			}
			add(fmt.Sprintf("%s seasons %d-%d", name, first, last))
			add(fmt.Sprintf("%s complete", name))
			continue
		}
		if req.Anime {
			add(fmt.Sprintf("%s batch", name))
			add(fmt.Sprintf("%s season %d", name, first))
			continue
		}
		add(fmt.Sprintf("%s S%02d", name, first))
		add(fmt.Sprintf("%s season %d", name, first))
		add(fmt.Sprintf("%s S%02d complete", name, first))
	}
	if len(queries) > 6 {
		queries = queries[:6]
	}
	return queries
}

// validatePack decides whether a release is a usable pack for the wanted
// episodes of the given seasons (ascending): the show matches, the title
// names every wanted season (a single season, a season range covering them
// all, a complete series, or an episode range covering every wanted number),
// it is not a single episode, and it clears the quality floor. It returns the
// parsed quality tier, the name score and how many seasons the pack spans
// beyond the wanted ones (for scoring).
func (q *QbittHandler) validatePack(torrent *prowlarr.Search, req *tvdb.Media, seasons []int, wanted []tvdb.Episode) (bool, string, int, float64) {
	ok, why, tier, score, _ := q.validatePackSpan(torrent, req, seasons, wanted)
	return ok, why, tier, score
}

func (q *QbittHandler) validatePackSpan(torrent *prowlarr.Search, req *tvdb.Media, seasons []int, wanted []tvdb.Episode) (bool, string, int, float64, int) {
	pt := parsePack(torrent.Title)
	if pt.ParseError {
		return false, "could not parse title", 0, 0, 0
	}
	if pt.SingleEp {
		return false, "single episode, not a pack", 0, 0, 0
	}
	nameScore := q.scoreNameMatch(pt.ShowName, req.Name)
	for _, alias := range req.Aliases {
		if s := q.scoreNameMatch(pt.ShowName, alias); s > nameScore {
			nameScore = s
		}
	}
	if nameScore < 0.3 {
		return false, fmt.Sprintf("show name mismatch: parsed=%q", pt.ShowName), 0, 0, 0
	}
	first, last := seasons[0], seasons[len(seasons)-1]
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
	extra := 0
	switch {
	case pt.Series && pt.SeasonEnd == 0 && pt.EpStart == 0:
		// "complete series": every season, however many that is
		extra = 1
	case pt.SeasonEnd > 0 && pt.Season <= first && pt.SeasonEnd >= last:
		extra = pt.spannedSeasons() - len(seasons)
	case len(seasons) == 1 && pt.Season == first && pt.EpStart == 0:
		// season-only title ("Show S01", "Show Season 1 Complete")
	case len(seasons) == 1 && pt.Season == first && covers():
	case pt.Season == 0 && (pt.Complete || req.Anime) && covers():
	case pt.Season == 0 && pt.Complete && pt.EpStart == 0 && len(seasons) == 1 && first == 1 && req.Anime:
		// anime "batch" with no numbering at all: only trust it for season 1
	default:
		return false, fmt.Sprintf("does not cover season %s", seasonLabel(seasons)), 0, 0, 0
	}
	tier := QualityTier(pt.Quality)
	if floor := q.qualityFloor(); floor > 0 && tier < floor {
		return false, fmt.Sprintf("%s is below the minimum quality", orLabel(pt.Quality)), tier, nameScore, 0
	}
	return true, "", tier, nameScore, extra
}

// seasonLabel renders wanted seasons for a message: "7" or "5-10".
func seasonLabel(seasons []int) string {
	if len(seasons) == 1 {
		return strconv.Itoa(seasons[0])
	}
	return fmt.Sprintf("%d-%d", seasons[0], seasons[len(seasons)-1])
}

func orLabel(q string) string {
	if q == "" {
		return "unknown resolution"
	}
	return q
}

// searchForPack runs the pack queries across the indexer tiers and returns
// the best acceptable pack for the wanted episodes of the given seasons, or
// nil. A pack that spans more seasons than wanted still qualifies, scored
// below an exact one: it is a bigger download, though only the wanted files
// are selected once it is added.
func (q *QbittHandler) searchForPack(ctx context.Context, req *tvdb.Media, seasons []int, wanted []tvdb.Episode, excluded *excludedTorrents) *models.TorrentMatch {
	type scored struct {
		torrent *prowlarr.Search
		score   float64
		query   string
	}
	var best *scored
	seen := map[string]bool{}
	for _, indexerIDs := range q.indexerTiers(req.Category, req.Anime) {
		for _, query := range packQueries(req, seasons) {
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
				ok, why, tier, nameScore, extra := q.validatePackSpan(t, req, seasons, wanted)
				if !ok {
					q.logger.DebugContext(ctx, "Pack rejected", "title", t.Title, "reason", why)
					continue
				}
				score := nameScore + calculateYearAwareQualityBoost(tier, "") + seederBoost(t.Seeders)
				if t.Seeders <= suspectSeederCount {
					score -= 1.0 // a dead pack wastes a whole season's wait
				}
				if extra > 0 {
					score -= min(0.1*float64(extra), 0.5)
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
		telemetry.WithTraceContext(ctx, "show", req.Name, "seasons", seasonLabel(seasons), "title", best.torrent.Title,
			"seeders", best.torrent.Seeders, "score", best.score, "episodes", len(wanted))...)
	return &models.TorrentMatch{Strategy: q.packStrategy(req, seasons, wanted, best.query), Torrent: best.torrent}
}

// tryPacks groups the episodes to download by season and, when packs are on,
// downloads packs for what they can cover: first one pack spanning every
// season with episodes to download when there are several, then one pack
// per season that has enough of them. It returns the per-episode results
// for packed episodes and which episodes were covered; anything else goes
// through the per-episode path.
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
	for _, s := range seasons {
		sort.Slice(bySeason[s], func(i, j int) bool { return bySeason[s][i].Number < bySeason[s][j].Number })
	}

	results := []dlstatus.EpisodeResult{}
	packed := map[int]bool{}
	download := func(span []int, wanted []tvdb.Episode) bool {
		packCtx, packSpan := torrentTracer.Start(ctx, "searchForSeasonPack")
		defer packSpan.End()
		packSpan.SetAttributes(attribute.String("show", req.Name), attribute.String("seasons", seasonLabel(span)),
			attribute.Int("episodes", len(wanted)))
		excluded := q.loadExcludedTorrents(packCtx, req.Id, span[0], wanted[0].Number)
		match := q.searchForPack(packCtx, req, span, wanted, excluded)
		if match == nil {
			q.logger.InfoContext(packCtx, "No pack found",
				telemetry.WithTraceContext(packCtx, "show", req.Name, "seasons", seasonLabel(span))...)
			return false
		}
		result := q.initiateDownloadAndMonitor(packCtx, match, searchRequestFor(req, match.Strategy), done, wg)
		if result.Outcome != dlstatus.OutcomeDownloading {
			return false // the add failed; let the narrower searches have a go
		}
		for _, ep := range wanted {
			packed[ep.Id] = true
			results = append(results, dlstatus.EpisodeResult{TvdbID: strconv.Itoa(ep.Id), Season: ep.SeasonNumber, Episode: ep.Number, Outcome: dlstatus.OutcomeDownloading})
		}
		return true
	}

	// Several seasons: one pack for all of them first.
	if len(seasons) > 1 {
		wanted := []tvdb.Episode{}
		for _, s := range seasons {
			wanted = append(wanted, bySeason[s]...)
		}
		if download(seasons, wanted) {
			return results, packed
		}
		q.logger.InfoContext(ctx, "No multi-season pack, trying one season at a time",
			telemetry.WithTraceContext(ctx, "show", req.Name, "seasons", seasonLabel(seasons))...)
	}

	for _, season := range seasons {
		wanted := bySeason[season]
		if len(wanted) < minEpisodes {
			continue
		}
		if !download([]int{season}, wanted) {
			q.logger.InfoContext(ctx, "No season pack found, searching per episode",
				telemetry.WithTraceContext(ctx, "show", req.Name, "season", season)...)
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

// packEpisodeIndex keys a pack's wanted episodes by season/episode and by
// absolute number, for matching file names against them.
func packEpisodeIndex(pack *models.PackInfo) map[string]tvdb.Episode {
	byKey := map[string]tvdb.Episode{}
	for _, ep := range pack.Episodes {
		byKey[fmt.Sprintf("s%de%d", ep.SeasonNumber, ep.Number)] = ep
		if ep.AbsoluteNumber > 0 {
			byKey[fmt.Sprintf("abs%d", ep.AbsoluteNumber)] = ep
		}
	}
	return byKey
}

// packFileSeason is the season a file inside a pack belongs to when its name
// does not say: the season folder it sits in, else the pack's first season.
func packFileSeason(path string, pack *models.PackInfo) int {
	dir := filepath.Dir(filepath.ToSlash(path))
	for dir != "." && dir != "/" && dir != "" {
		if m := packDirSeasonRe.FindStringSubmatch(filepath.Base(dir)); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				return n
			}
		}
		dir = filepath.Dir(dir)
	}
	return pack.Season
}

// packFileEpisodes matches one file of a landed or listed pack to the wanted
// episodes it holds, by the file's own numbering: S##E##, an absolute number,
// or a bare episode number read against the season folder it sits in. A
// file named for a run of episodes ("S07E01-E02", common for shows whose
// segments TVDB lists separately) yields every wanted episode in the run,
// in order. An empty result means the file holds nothing that was wanted.
func packFileEpisodes(parser *TorrentParser, path string, pack *models.PackInfo, byKey map[string]tvdb.Episode) []tvdb.Episode {
	name := filepath.Base(path)
	parsed := parser.Parse(name)
	if parsed == nil {
		// Bare "E05" / "05" names inside a season folder.
		parsed = parsePackFileFallback(name, packFileSeason(path, pack))
	}
	if parsed == nil {
		return nil
	}
	last := parsed.Episode
	if parsed.IsMultiEpisode() {
		last = parsed.EpisodeEnd
	} else if m := packFileRunRe.FindStringSubmatch(name); m != nil && !parsed.IsAbsolute {
		if first, err := strconv.Atoi(m[1]); err == nil && first == parsed.Episode {
			if end, err := strconv.Atoi(m[2]); err == nil && end > first && end-first < 10 {
				last = end
			}
		}
	}
	out := []tvdb.Episode{}
	for n := parsed.Episode; n <= last; n++ {
		if parsed.IsAbsolute {
			if ep, ok := byKey[fmt.Sprintf("abs%d", n)]; ok {
				out = append(out, ep)
				continue
			}
			if ep, ok := byKey[fmt.Sprintf("s%de%d", packFileSeason(path, pack), n)]; ok {
				out = append(out, ep)
			}
			continue
		}
		season := parsed.Season
		if season == 0 {
			season = packFileSeason(path, pack)
		}
		if ep, ok := byKey[fmt.Sprintf("s%de%d", season, n)]; ok {
			out = append(out, ep)
		}
	}
	return out
}

// packFileEpisode is packFileEpisodes for callers that only need to know the
// first wanted episode a file holds.
func packFileEpisode(parser *TorrentParser, path string, pack *models.PackInfo, byKey map[string]tvdb.Episode) (tvdb.Episode, bool) {
	eps := packFileEpisodes(parser, path, pack, byKey)
	if len(eps) == 0 {
		return tvdb.Episode{}, false
	}
	return eps[0], true
}

// trimPackFiles deselects, in qBittorrent, the files of a pack that hold no
// wanted episode, so a complete-series pack fetched for six seasons does not
// download the other seven. done is false while the file list is not
// available yet (a magnet still fetching metadata) and the caller should try
// again. usable is false when not one file could be matched to a wanted
// episode: downloading a whole pack blind is not what was asked for, so the
// caller gives it up. A failure to trim a usable pack leaves all of it
// downloading, which is only slower.
func (q *QbittHandler) trimPackFiles(ctx context.Context, infoHash string, pack *models.PackInfo) (done, usable bool) {
	if pack == nil {
		return true, true
	}
	files, err := q.c.GetFilesInformationCtx(ctx, infoHash)
	if err != nil {
		q.logger.WarnContext(ctx, "Could not list the pack's files, downloading all of it",
			telemetry.WithTraceContext(ctx, "torrent_hash", infoHash, "error", err.Error())...)
		return true, true
	}
	if files == nil || len(*files) == 0 {
		return false, true
	}
	byKey := packEpisodeIndex(pack)
	wanted, skip := 0, []string{}
	for _, f := range *files {
		if isVideoFile(f.Name) {
			if _, ok := packFileEpisode(q.parser, f.Name, pack, byKey); ok {
				wanted++
				continue
			}
		}
		skip = append(skip, strconv.Itoa(f.Index))
	}
	if wanted == 0 {
		q.logger.WarnContext(ctx, "No file in the pack matched a wanted episode",
			telemetry.WithTraceContext(ctx, "torrent_hash", infoHash, "files", len(*files))...)
		return true, false
	}
	if len(skip) == 0 {
		return true, true
	}
	if err := q.c.SetFilePriorityCtx(ctx, infoHash, strings.Join(skip, "|"), 0); err != nil {
		q.logger.WarnContext(ctx, "Could not deselect the pack's unwanted files, downloading all of it",
			telemetry.WithTraceContext(ctx, "torrent_hash", infoHash, "error", err.Error())...)
		return true, true
	}
	q.logger.InfoContext(ctx, "Pack trimmed to the wanted episodes",
		telemetry.WithTraceContext(ctx, "torrent_hash", infoHash, "wanted_files", wanted, "skipped_files", len(skip))...)
	return true, true
}
