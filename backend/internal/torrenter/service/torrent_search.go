package service

import (
	"context"
	"html"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golift.io/starr/prowlarr"
)

// Prowlarr indexer IDs. Nyaa and 1337x are the preferred indexers for their media
// kinds. The Pirate Bay is a last resort: it is only searched when the preferred
// indexers turn up nothing usable (see indexerTiers), so a release that exists on
// both is always taken from the preferred one.
var (
	NYAA_ID    = int64(1) // Anime
	ONE337x_ID = int64(2) // General, preferred
	TPB_ID     = int64(3) // General, fallback only
)

// prowlarrSearchBackoffs is the sleep applied BEFORE each retry attempt.
// len(backoffs)+1 total attempts: attempt 1 (no sleep), then 2s, then 8s.
var prowlarrSearchBackoffs = []time.Duration{2 * time.Second, 8 * time.Second}

const prowlarrSearchTimeout = 45 * time.Second

// episodeSearchDeadline bounds the WHOLE per-episode tier ladder (all relax tiers and
// every strategy/Prowlarr search within them, i.e. roughly prowlarrSearchTimeout ×
// attempts × strategies × tiers), so a sick indexer can't pin a single episode's request.
const episodeSearchDeadline = 5 * time.Minute

func (q *QbittHandler) searchProwlarr(ctx context.Context, query, category string, indexerIDs []int64) ([]*prowlarr.Search, error) {
	q.logger.InfoContext(ctx, "Searching Prowlarr", "query", query, "category", category, "indexer_ids", indexerIDs)

	input := prowlarr.SearchInput{
		Query:      query,
		IndexerIDs: indexerIDs,
		Limit:      500,
		Categories: q.calcCategories(category == "series"),
	}

	totalAttempts := len(prowlarrSearchBackoffs) + 1
	var lastErr error
	for attempt := 1; attempt <= totalAttempts; attempt++ {
		// Abort immediately on parent cancellation — don't burn retries on a dead request.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		resp, err := q.searchProwlarrOnce(ctx, input)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// If the parent context was cancelled, the error is terminal — don't retry.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if attempt < totalAttempts {
			backoff := prowlarrSearchBackoffs[attempt-1]
			q.logger.WarnContext(ctx, "Prowlarr search failed, retrying",
				telemetry.WithTraceContext(ctx,
					"query", query,
					"attempt", attempt,
					"max_attempts", totalAttempts,
					"backoff", backoff.String(),
					"error", err.Error())...)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	q.logger.ErrorContext(ctx, "Prowlarr search failed after all attempts",
		telemetry.WithTraceContext(ctx, "query", query, "attempts", totalAttempts, "error", lastErr.Error())...)
	return nil, lastErr
}

// searchProwlarrOnce performs a single Prowlarr search bounded by a per-attempt timeout.
func (q *QbittHandler) searchProwlarrOnce(ctx context.Context, input prowlarr.SearchInput) ([]*prowlarr.Search, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, prowlarrSearchTimeout)
	defer cancel()
	return q.p.SearchContext(attemptCtx, input)
}

// buildSearchStrategies creates search strategies for all episodes that need downloading
func (q *QbittHandler) buildSearchStrategies(ctx context.Context, req *tvdb.Media, episodes []tvdb.Episode) [][]*models.SearchStrategy {
	searchStrategies := [][]*models.SearchStrategy{}
	for _, episode := range episodes {
		q.logger.InfoContext(ctx, "Processing episode", telemetry.WithTraceContext(ctx, "name", req.Name, "season", episode.SeasonNumber,
			"episode", episode.Number)...)
		searchStrategies = append(searchStrategies, q.createSearchStrategy(req, &episode))
	}
	return searchStrategies
}

// excludedTorrents is the set of releases already tried and given up on for
// one item. A nil set excludes nothing.
type excludedTorrents struct {
	hashes map[string]bool
	titles map[string]bool
}

func newExcludedTorrents(failed []*models.FailedTorrent) *excludedTorrents {
	ex := &excludedTorrents{hashes: map[string]bool{}, titles: map[string]bool{}}
	for _, f := range failed {
		if f.InfoHash != "" {
			ex.hashes[strings.ToLower(f.InfoHash)] = true
		}
		if f.Title != "" {
			ex.titles[normalizeTitle(f.Title)] = true
		}
	}
	return ex
}

// normalizeTitle makes release titles comparable across sources: qBittorrent
// HTML-encodes characters such as the apostrophe in "Marvel's" while Prowlarr
// does not, and whitespace and case vary between listings of one release.
func normalizeTitle(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(html.UnescapeString(title)), " "))
}

// contains reports whether a search hit is one of the excluded releases, by
// info hash when the indexer supplies one and by title otherwise. Prowlarr
// leaves the hash empty for .torrent-file results, so the title match does
// most of the work for 1337x.
func (ex *excludedTorrents) contains(t *prowlarr.Search) bool {
	if ex == nil || t == nil {
		return false
	}
	if t.InfoHash != "" && ex.hashes[strings.ToLower(t.InfoHash)] {
		return true
	}
	return ex.titles[normalizeTitle(t.Title)]
}

// drop removes excluded hits from results in place and returns how many went.
func (ex *excludedTorrents) drop(results *[]*prowlarr.Search) int {
	if ex == nil || len(*results) == 0 {
		return 0
	}
	kept := (*results)[:0]
	dropped := 0
	for _, t := range *results {
		if ex.contains(t) {
			dropped++
			continue
		}
		kept = append(kept, t)
	}
	*results = kept
	return dropped
}

// loadExcludedTorrents fetches the releases already given up on for an item.
// A lookup failure excludes nothing: the search still runs, at the risk of
// re-picking a torrent that failed before.
func (q *QbittHandler) loadExcludedTorrents(ctx context.Context, tvdbID string, season, episode int) *excludedTorrents {
	failed, err := q.repo.GetFailedTorrents(ctx, tvdbID, season, episode)
	if err != nil {
		q.logger.WarnContext(ctx, "Failed to load previously failed torrents, searching without that check",
			telemetry.WithTraceContext(ctx, "error", err.Error(), "tvdb_id", tvdbID, "season", season, "episode", episode)...)
		return nil
	}
	if len(failed) == 0 {
		return nil
	}
	return newExcludedTorrents(failed)
}

// searchOutcome is what searchForMatch learned about one episode (or movie) across
// every indexer tier and relax tier it tried.
type searchOutcome struct {
	match      *models.TorrentMatch
	winningCtx context.Context
	// attemptedAny is true once at least one strategy was sent to Prowlarr.
	attemptedAny bool
	// erroredAll is true when every strategy sent to Prowlarr errored, which means
	// the indexers were unreachable rather than merely empty.
	erroredAll bool
}

// searchForMatch walks the indexer tiers in priority order and, within each, the relax
// tiers in ascending order, stopping at the first tier that produces a confident best
// pick. A later indexer tier is only consulted when every earlier one produced no
// usable torrent, so the preferred indexer always wins when it carries the release at all.
func (q *QbittHandler) searchForMatch(ctx context.Context, strategies []*models.SearchStrategy, req *tvdb.Media, excluded *excludedTorrents) searchOutcome {
	outcome := searchOutcome{erroredAll: true}
	relaxTiers := groupStrategiesByRelaxLevel(strategies)

	// A pick whose reported seeder count is suspect (see seederBoost) is only a
	// last resort: on public indexers such listings are usually dead, and a
	// dead torrent sits at 0% until the monitor gives up. So instead of taking
	// it, the search moves straight on to the next indexer tier looking for a
	// live swarm, and settles for the weak pick only when nothing better turns
	// up anywhere. Relaxed queries on the same indexer are skipped in that case:
	// they exist to find a release under a different name, not a healthier one,
	// and every extra query counts against the indexer's rate limit.
	var weak *models.TorrentMatch
	var weakCtx context.Context

	for tierIdx, indexerIDs := range q.indexerTiers(req.Category, req.Anime) {
		if tierIdx > 0 {
			q.logger.InfoContext(ctx, "Preferred indexers had no usable torrent, trying fallback indexers",
				telemetry.WithTraceContext(ctx, "media", req.Name, "indexer_ids", indexerIDs, "have_weak_pick", weak != nil)...)
		}

		for _, tier := range relaxTiers {
			bestMatches, strategyContexts, allIndexersFailed := q.executeSearchStrategies(ctx, tier, req, indexerIDs, excluded)
			outcome.attemptedAny = true
			if !allIndexersFailed {
				outcome.erroredAll = false
			}

			// Select the best torrent from this tier's matches.
			q.sortBySeedersDESC(bestMatches)
			q.sortTorrentsByQuality(bestMatches)
			tierMatch := q.pickBestTorrent(ctx, bestMatches, req.Category, req.Anime)

			// End all non-winning strategy spans for this tier.
			for ss, strategyCtx := range strategyContexts {
				if tierMatch == nil || ss != tierMatch.Strategy {
					span := trace.SpanFromContext(strategyCtx)
					span.SetStatus(codes.Ok, "strategy not selected")
					span.End()
				}
			}

			if tierMatch == nil {
				continue
			}

			if tierMatch.Torrent.Seeders > suspectSeederCount {
				if weak != nil {
					endStrategySpan(weakCtx, "superseded by a better-seeded pick")
				}
				outcome.match = tierMatch
				outcome.winningCtx = strategyContexts[tierMatch.Strategy]
				return outcome
			}

			q.logger.WarnContext(ctx, "Best pick has too few seeders to trust, keeping it as a last resort and searching on",
				telemetry.WithTraceContext(ctx,
					"media", req.Name,
					"torrent_title", tierMatch.Torrent.Title,
					"seeders", tierMatch.Torrent.Seeders,
					"indexer_ids", indexerIDs)...)
			if weak == nil || tierMatch.Torrent.Seeders > weak.Torrent.Seeders {
				if weak != nil {
					endStrategySpan(weakCtx, "superseded by a better-seeded last resort")
				}
				weak = tierMatch
				weakCtx = strategyContexts[tierMatch.Strategy]
			} else {
				endStrategySpan(strategyContexts[tierMatch.Strategy], "weaker than the kept last resort")
			}
			// Skip this indexer's relaxed tiers and try the next indexer.
			break
		}
	}

	if weak != nil {
		q.logger.WarnContext(ctx, "No better-seeded torrent found anywhere, settling for the weakly seeded pick",
			telemetry.WithTraceContext(ctx,
				"media", req.Name,
				"torrent_title", weak.Torrent.Title,
				"seeders", weak.Torrent.Seeders)...)
		outcome.match = weak
		outcome.winningCtx = weakCtx
	}
	return outcome
}

// endStrategySpan closes the span of a search strategy that had been kept
// open as a potential winner but was not selected in the end.
func endStrategySpan(strategyCtx context.Context, why string) {
	if strategyCtx == nil {
		return
	}
	span := trace.SpanFromContext(strategyCtx)
	span.SetStatus(codes.Ok, why)
	span.End()
}

// groupStrategiesByRelaxLevel partitions strategies into tiers ordered by ascending
// RelaxLevel. Strategy order within a tier is preserved.
func groupStrategiesByRelaxLevel(strategies []*models.SearchStrategy) [][]*models.SearchStrategy {
	if len(strategies) == 0 {
		return nil
	}

	// Collect distinct relax levels in ascending order.
	levels := []int{}
	seen := map[int]bool{}
	for _, ss := range strategies {
		if !seen[ss.RelaxLevel] {
			seen[ss.RelaxLevel] = true
			levels = append(levels, ss.RelaxLevel)
		}
	}
	sort.Ints(levels)

	tiers := make([][]*models.SearchStrategy, 0, len(levels))
	for _, lvl := range levels {
		tier := []*models.SearchStrategy{}
		for _, ss := range strategies {
			if ss.RelaxLevel == lvl {
				tier = append(tier, ss)
			}
		}
		tiers = append(tiers, tier)
	}
	return tiers
}

// executeSearchStrategies runs all search strategies against the given Prowlarr indexers
// and collects matching torrents.
//
// Unlike the previous implementation, a Prowlarr error on one strategy no longer aborts the whole
// episode: the error is recorded and the remaining strategies still run. The returned
// allIndexersFailed flag is true only when EVERY strategy errored (and at least one strategy was
// attempted), signalling an unreachable indexer. If any strategy executed successfully — even with
// zero results — the caller proceeds with whatever matches were collected.
func (q *QbittHandler) executeSearchStrategies(
	ctx context.Context,
	strategies []*models.SearchStrategy,
	req *tvdb.Media,
	indexerIDs []int64,
	excluded *excludedTorrents,
) ([]*models.TorrentMatch, map[*models.SearchStrategy]context.Context, bool) {
	bestMatches := []*models.TorrentMatch{}
	strategyContexts := make(map[*models.SearchStrategy]context.Context)

	erroredStrategies := 0

	for strategyIdx, ss := range strategies {
		strategyCtx, strategySpan := torrentTracer.Start(ctx, "searchStrategy")
		strategySpan.SetAttributes(
			attribute.Int("strategy_index", strategyIdx),
			attribute.String("query", ss.Query),
			attribute.String("media_name", ss.MediaName),
			attribute.Int("season", ss.Season),
			attribute.Int("episode", ss.Episode),
			attribute.Bool("is_anime", req.Anime),
			attribute.Int64Slice("indexer_ids", indexerIDs),
		)

		q.logger.InfoContext(strategyCtx, "Search Query", telemetry.WithTraceContext(strategyCtx, "query", ss.Query)...)
		pResp, err := q.searchProwlarr(strategyCtx, ss.Query, req.Category, indexerIDs)
		if err != nil {
			// Record the failure and continue with the remaining strategies instead of aborting.
			erroredStrategies++
			q.logger.WarnContext(strategyCtx, "Prowlarr search failed, continuing with remaining strategies",
				telemetry.WithTraceContext(strategyCtx, "query", ss.Query, "error", err.Error())...)
			strategySpan.SetStatus(codes.Error, "failed to search Prowlarr")
			strategySpan.RecordError(err)
			strategySpan.End()
			continue
		}

		strategySpan.SetAttributes(attribute.Int("results_found", len(pResp)))

		if dropped := excluded.drop(&pResp); dropped > 0 {
			q.logger.InfoContext(strategyCtx, "Skipped torrents that already failed for this item",
				telemetry.WithTraceContext(strategyCtx, "query", ss.Query, "skipped", dropped)...)
			strategySpan.SetAttributes(attribute.Int("previously_failed_skipped", dropped))
		}

		if len(pResp) == 0 {
			q.logger.InfoContext(strategyCtx, "no results found", "query", ss.Query)
			strategySpan.SetAttributes(
				attribute.Int("valid_matches", 0),
				attribute.Bool("success", false),
			)
			strategySpan.SetStatus(codes.Ok, "no results found")
			strategySpan.End()
			continue
		}

		// Filter and validate torrents
		possibleTorrents, rejectionReasons := q.filterTorrents(strategyCtx, pResp, ss)
		q.logFilteringSummary(strategyCtx, len(pResp), len(possibleTorrents), rejectionReasons, ss.Query)

		strategySpan.SetAttributes(
			attribute.Int("valid_matches", len(possibleTorrents)),
			attribute.Int("rejected_matches", len(pResp)-len(possibleTorrents)),
		)

		if len(possibleTorrents) == 0 {
			q.logger.InfoContext(strategyCtx, "no matching torrents found", "query", ss.Query)
			// Use recordDownloadFailure for its nil-safe AbsoluteNumber handling
			// (movie strategies have EpisodeMeta == nil).
			q.recordDownloadFailure(strategyCtx, req, ss, "no matching torrents found")
			strategySpan.SetAttributes(attribute.Bool("success", false))
			strategySpan.SetStatus(codes.Ok, "no matching torrents found")
			strategySpan.End()
		} else {
			bestMatches = append(bestMatches, possibleTorrents...)
			strategySpan.SetAttributes(attribute.Bool("success", true))
			strategySpan.SetStatus(codes.Ok, "found matching torrents")
			// Don't end the span yet - store context for the winning strategy
			strategyContexts[ss] = strategyCtx
		}
	}

	// Indexer is considered unreachable only when every attempted strategy errored.
	allIndexersFailed := len(strategies) > 0 && erroredStrategies == len(strategies)
	return bestMatches, strategyContexts, allIndexersFailed
}

func (q *QbittHandler) calcCategories(show bool) []int64 {
	if show {
		return []int64{5000}
	} else {
		return []int64{2000}
	}
}

// calcIndexerIDs determines the preferred Prowlarr indexers for the media type and anime status
// Anime shows: NYAA only
// Regular shows: 1337x only
// Anime movies: NYAA + 1337x (both indexers for better coverage)
// Regular movies: 1337x only
// The Pirate Bay is never preferred; indexerTiers adds it as the fallback.
func (q *QbittHandler) calcIndexerIDs(category string, isAnime bool) []int64 {
	if category == "movie" {
		// Movies: anime uses both NYAA and 1337x, regular uses 1337x only
		if isAnime {
			return []int64{NYAA_ID, ONE337x_ID}
		}
		return []int64{ONE337x_ID}
	}

	// Shows (series): anime uses NYAA, regular uses 1337x
	if isAnime {
		return []int64{NYAA_ID}
	}
	return []int64{ONE337x_ID}
}

// indexerTiers returns the groups of Prowlarr indexers to search for the media, in
// priority order: the preferred indexers from calcIndexerIDs first, then The Pirate
// Bay on its own. A group is only searched when every earlier group produced no usable
// torrent, so 1337x still wins whenever it carries the release at all.
func (q *QbittHandler) indexerTiers(category string, isAnime bool) [][]int64 {
	preferred := q.calcIndexerIDs(category, isAnime)
	tiers := [][]int64{preferred}
	if TPB_ID != 0 && !slices.Contains(preferred, TPB_ID) {
		tiers = append(tiers, []int64{TPB_ID})
	}
	return tiers
}
