package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golift.io/starr"
	"golift.io/starr/prowlarr"
)

var (
	baseSavePath  = "/data/Downloads"
	scoutTag      = "scout"
	torrentTracer = otel.Tracer("torrenter/torrent")

	// trailingYearRe matches a trailing parenthesized or bracketed year, e.g.
	// " (2019)" or " [2019]", optionally preceded by whitespace.
	trailingYearRe = regexp.MustCompile(`\s*[\(\[]\d{4}[\)\]]\s*$`)
)

type QbittHandler struct {
	c      *qbittorrent.Client
	p      *prowlarr.Prowlarr
	repo   Repository
	logger *slog.Logger
	parser *TorrentParser
}

// Indexer IDs
var (
	NYAA_ID    = int64(1) // Anime
	ONE337x_ID = int64(2) // General
)

func NewQbittHandler(qCfg *models.QbittCfg, pCfg *models.ProwlarrCfg, repo Repository, logger *slog.Logger) (*QbittHandler, error) {
	logger.Debug("QBittorrent config", "host", qCfg.Host, "user", qCfg.User)

	// Create new qBittorrent client with Config
	qb := qbittorrent.NewClient(qbittorrent.Config{
		Host:     qCfg.Host,
		Username: qCfg.User,
		Password: qCfg.Password,
	})

	// Authenticate with qBittorrent
	if err := qb.Login(); err != nil {
		return nil, fmt.Errorf("failed to login to qBittorrent: %w", err)
	}

	logger.Info("Connected to QBittorrent successfully")

	p := prowlarr.New(starr.New(pCfg.Key, pCfg.Host, 60*time.Hour))
	return &QbittHandler{
		c:      qb,
		logger: logger,
		p:      p,
		repo:   repo,
		parser: NewTorrentParser(),
	}, nil
}

// prowlarrSearchBackoffs is the sleep applied BEFORE each retry attempt.
// len(backoffs)+1 total attempts: attempt 1 (no sleep), then 2s, then 8s.
var prowlarrSearchBackoffs = []time.Duration{2 * time.Second, 8 * time.Second}

const prowlarrSearchTimeout = 45 * time.Second

// episodeSearchDeadline bounds the WHOLE per-episode tier ladder (all relax tiers and
// every strategy/Prowlarr search within them, i.e. roughly prowlarrSearchTimeout ×
// attempts × strategies × tiers), so a sick indexer can't pin a single episode's request.
const episodeSearchDeadline = 5 * time.Minute

func (q *QbittHandler) searchProwlarr(ctx context.Context, query, category string, isAnime bool) ([]*prowlarr.Search, error) {
	q.logger.InfoContext(ctx, "Searching Prowlarr", "query", query, "category", category)

	input := prowlarr.SearchInput{
		Query:      query,
		IndexerIDs: q.calcIndexerIDs(category, isAnime),
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

func (q *QbittHandler) HandleDownload(ctx context.Context, req *tvdb.Media, done chan<- models.TorrentCompleteEvent) ([]dlstatus.EpisodeResult, error) {
	results := []dlstatus.EpisodeResult{}

	// Handle movies: check if movie already exists in Plex
	if req.Category == "movie" {
		exists, err := q.repo.MovieExistsByTvdbId(ctx, req.Id)
		if err != nil {
			q.logger.WarnContext(ctx, "Failed to check movie existence by TVDB ID",
				telemetry.WithTraceContext(ctx, "tvdb_id", req.Id, "movie", req.Name, "error", err.Error())...)
		}
		if exists {
			q.logger.InfoContext(ctx, "Movie already exists in Plex, skipping",
				telemetry.WithTraceContext(ctx, "movie", req.Name, "tvdb_id", req.Id)...)
			// Update notification to completed with "already exists" reason
			q.updateNotificationStatus(ctx, req.Id, "Already exists in Plex library", notifications.StatusCompleted)
			results = append(results, dlstatus.EpisodeResult{
				TvdbID:  req.Id,
				Outcome: dlstatus.OutcomeExists,
			})
			close(done)
			return results, nil
		}
	}

	// Handle series: Filter out episodes that already exist in Plex
	if req.Category == "series" {
		episodesToDownload, existsResults, err := q.filterExistingEpisodes(ctx, req)
		if err != nil {
			return results, err
		}
		results = append(results, existsResults...)

		if len(episodesToDownload) == 0 {
			q.logger.InfoContext(ctx, "All episodes already exist in Plex, nothing to download",
				telemetry.WithTraceContext(ctx, "show", req.Name)...)
			close(done)
			return results, nil
		}

		q.logger.InfoContext(ctx, "Episodes to download after filtering",
			telemetry.WithTraceContext(ctx, "show", req.Name,
				"total", len(req.Metadata.Episodes), "to_download", len(episodesToDownload),
				"show_aliases", req.Aliases)...)

		// Build search strategies for each episode
		searchStrategies := q.buildSearchStrategies(ctx, req, episodesToDownload)

		// Process all episodes concurrently
		var wg sync.WaitGroup
		ctx, searchSpan := torrentTracer.Start(ctx, "searchForEpisodes")
		searchSpan.SetAttributes(
			attribute.String("show_name", req.Name),
			attribute.String("tvdb_id", req.Id),
			attribute.Int("episode_count", len(searchStrategies)),
			attribute.Bool("is_anime", req.Anime),
		)
		defer searchSpan.End()

		for episodeIdx, strategies := range searchStrategies {
			result := q.processEpisodeDownload(ctx, episodeIdx, strategies, req, done, &wg)
			results = append(results, result)
		}

		// Close the channel after all monitoring goroutines complete
		go func() {
			wg.Wait()
			close(done)
			q.logger.InfoContext(ctx, "All torrents processed, channel closed")
		}()

		return results, nil
	}

	// Handle movies: create movie search strategy and process
	if req.Category == "movie" {
		movieStrategy := q.createMovieSearchStrategy(req)

		var wg sync.WaitGroup
		ctx, searchSpan := torrentTracer.Start(ctx, "searchForMovie")
		searchSpan.SetAttributes(
			attribute.String("movie_name", req.Name),
			attribute.String("tvdb_id", req.Id),
			attribute.Bool("is_anime", req.Anime),
		)
		defer searchSpan.End()

		// Process movie download using existing episode download logic
		result := q.processEpisodeDownload(ctx, 0, movieStrategy, req, done, &wg)
		results = append(results, result)

		// Close the channel after monitoring goroutine completes
		go func() {
			wg.Wait()
			close(done)
			q.logger.InfoContext(ctx, "Movie torrent processed, channel closed")
		}()

		return results, nil
	}

	// Unknown category — nothing to process; close the channel so the
	// completion handler goroutine can exit.
	close(done)
	return results, nil
}

// filterExistingEpisodes checks which episodes already exist in Plex and returns only those that need downloading.
// For episodes that already exist, it updates their notifications to "completed" status and returns an
// EpisodeResult with Outcome "exists" so callers can report them in the structured download response.
func (q *QbittHandler) filterExistingEpisodes(ctx context.Context, req *tvdb.Media) ([]tvdb.Episode, []dlstatus.EpisodeResult, error) {
	ctx, span := torrentTracer.Start(ctx, "filterExistingEpisodes")
	defer span.End()

	span.SetAttributes(
		attribute.String("show_name", req.Name),
		attribute.String("tvdb_id", req.Id),
		attribute.Int("total_episodes", len(req.Metadata.Episodes)),
	)

	episodesToDownload := []tvdb.Episode{}
	existsResults := []dlstatus.EpisodeResult{}
	for _, episode := range req.Metadata.Episodes {
		exists, err := q.repo.EpisodeExistsByTvdbId(ctx, req.Id, episode.SeasonNumber, episode.Number)
		if err != nil {
			q.logger.WarnContext(ctx, "Failed to check episode existence by TVDB ID",
				telemetry.WithTraceContext(ctx, "tvdb_id", req.Id, "season", episode.SeasonNumber, "episode", episode.Number, "error", err.Error())...)
		}

		if exists {
			q.logger.InfoContext(ctx, "Episode already exists in Plex, skipping",
				telemetry.WithTraceContext(ctx, "name", req.Name, "season", episode.SeasonNumber, "episode", episode.Number)...)
			// Update notification to completed with "already exists" reason
			q.updateNotificationStatus(ctx, strconv.Itoa(episode.Id), "Already exists in Plex library", notifications.StatusCompleted)
			existsResults = append(existsResults, dlstatus.EpisodeResult{
				TvdbID:  strconv.Itoa(episode.Id),
				Season:  episode.SeasonNumber,
				Episode: episode.Number,
				Outcome: dlstatus.OutcomeExists,
			})
			continue
		}

		episodesToDownload = append(episodesToDownload, episode)
	}

	episodesFiltered := len(req.Metadata.Episodes) - len(episodesToDownload)
	span.SetAttributes(
		attribute.Int("episodes_filtered", episodesFiltered),
		attribute.Int("episodes_to_download", len(episodesToDownload)),
	)
	span.SetStatus(codes.Ok, "Episode filtering complete")

	return episodesToDownload, existsResults, nil
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

// processEpisodeDownload handles the complete download workflow for a single episode (or movie)
// and returns a per-episode EpisodeResult describing the outcome. It never returns an error: every
// failure mode is mapped to a structured result so the webserver's retry engine can act on it.
func (q *QbittHandler) processEpisodeDownload(
	ctx context.Context,
	episodeIdx int,
	strategies []*models.SearchStrategy,
	req *tvdb.Media,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
) dlstatus.EpisodeResult {
	// Create episode-level parent span
	episodeCtx, episodeSpan := torrentTracer.Start(ctx, "searchEpisode")

	// Overall per-episode search deadline so a sick indexer can't pin the request.
	episodeCtx, cancel := context.WithTimeout(episodeCtx, episodeSearchDeadline)
	defer cancel()

	if len(strategies) > 0 {
		episodeSpan.SetAttributes(
			attribute.Int("episode_index", episodeIdx),
			attribute.Int("season", strategies[0].Season),
			attribute.Int("episode", strategies[0].Episode),
			attribute.Int("strategy_count", len(strategies)),
			attribute.String("media_name", strategies[0].MediaName),
		)
	}

	// Run strategies tier-by-tier (ascending RelaxLevel), stopping at the first tier
	// that produces a confident best pick. Track failure semantics across all attempted
	// tiers: only treat the indexer as unreachable when EVERY attempted strategy errored.
	var (
		match        *models.TorrentMatch
		winningCtx   context.Context
		attemptedAny bool
		erroredAll   = true
	)

	for _, tier := range groupStrategiesByRelaxLevel(strategies) {
		bestMatches, strategyContexts, allIndexersFailed := q.executeSearchStrategies(episodeCtx, tier, req)
		attemptedAny = true
		if !allIndexersFailed {
			erroredAll = false
		}

		// Select the best torrent from this tier's matches.
		q.sortBySeedersDESC(bestMatches)
		q.sortTorrentsByQuality(bestMatches)
		tierMatch := q.pickBestTorrent(episodeCtx, bestMatches, req.Category, req.Anime)

		// End all non-winning strategy spans for this tier.
		for ss, strategyCtx := range strategyContexts {
			if tierMatch == nil || ss != tierMatch.Strategy {
				span := trace.SpanFromContext(strategyCtx)
				span.SetStatus(codes.Ok, "strategy not selected")
				span.End()
			}
		}

		if tierMatch != nil {
			match = tierMatch
			winningCtx = strategyContexts[tierMatch.Strategy]
			break
		}
	}

	// If every strategy actually attempted (across all tried tiers) errored against
	// Prowlarr, the indexer is unreachable.
	if attemptedAny && erroredAll {
		q.logger.WarnContext(episodeCtx, "All indexer searches failed", "show", req.Name)
		ss := strategies[0]
		q.recordDownloadFailure(episodeCtx, req, ss, "indexer unreachable")
		q.applyFailureNotification(episodeCtx, ss.TvdbId, dlstatus.CodeIndexerUnreachable)
		episodeSpan.SetStatus(codes.Error, "all indexer searches failed")
		episodeSpan.End()
		return q.failedResult(ss, dlstatus.CodeIndexerUnreachable)
	}

	if match == nil {
		q.logger.WarnContext(episodeCtx, "No suitable torrent found", "show", req.Name)
		ss := strategies[0]
		q.recordDownloadFailure(episodeCtx, req, ss, "no suitable torrent found")
		// Transient: a torrent may appear later, so keep searching (retry owned by Phase 2).
		q.applyFailureNotification(episodeCtx, ss.TvdbId, dlstatus.CodeNoTorrentFound)
		episodeSpan.SetStatus(codes.Ok, "no suitable torrent found")
		episodeSpan.End()
		return q.failedResult(ss, dlstatus.CodeNoTorrentFound)
	}

	// Download the torrent and start monitoring
	episodeSpan.End()
	return q.initiateDownloadAndMonitor(winningCtx, match, done, wg)
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

// recordStrategyFailure inserts a download-history failure row from a search strategy alone
// (used by the monitor, which has no *tvdb.Media). Nil-safe for movies (EpisodeMeta == nil).
func (q *QbittHandler) recordStrategyFailure(ctx context.Context, ss *models.SearchStrategy, reason string) {
	if ss == nil {
		return
	}
	absoluteNumber := 0
	if ss.EpisodeMeta != nil {
		absoluteNumber = ss.EpisodeMeta.AbsoluteNumber
	}
	if err := q.repo.InsertDownloadHistory(ctx, ss.MediaName, ss.Season, ss.Episode, absoluteNumber, "", "failure", reason); err != nil {
		q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
	}
}

// recordDownloadFailure inserts a download-history failure row, preserving the prior behavior of
// always recording failures for observability.
func (q *QbittHandler) recordDownloadFailure(ctx context.Context, req *tvdb.Media, ss *models.SearchStrategy, reason string) {
	absoluteNumber := 0
	if ss.EpisodeMeta != nil {
		absoluteNumber = ss.EpisodeMeta.AbsoluteNumber
	}
	if err := q.repo.InsertDownloadHistory(ctx, req.Name, ss.Season, ss.Episode, absoluteNumber, "", "failure", reason); err != nil {
		q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
	}
}

// failedResult builds a failed EpisodeResult for a search strategy and failure code.
func (q *QbittHandler) failedResult(ss *models.SearchStrategy, code dlstatus.FailureCode) dlstatus.EpisodeResult {
	return dlstatus.EpisodeResult{
		TvdbID:  ss.TvdbId,
		Season:  ss.Season,
		Episode: ss.Episode,
		Outcome: dlstatus.OutcomeFailed,
		Code:    code,
		Reason:  code.HumanReason(),
	}
}

// applyFailureNotification updates the episode notification for a failure. Permanent failures move the
// notification to "failed"; transient failures keep it in "searching" with the human-readable reason,
// because the webserver's retry engine (Phase 2) owns the final failure decision.
func (q *QbittHandler) applyFailureNotification(ctx context.Context, tvdbID string, code dlstatus.FailureCode) {
	if code.Category() == dlstatus.CategoryPermanent {
		q.updateNotificationStatus(ctx, tvdbID, code.HumanReason(), notifications.StatusFailed)
		return
	}
	q.updateNotificationStatus(ctx, tvdbID, code.HumanReason(), notifications.StatusSearching)
}

// executeSearchStrategies runs all search strategies and collects matching torrents.
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
		)

		q.logger.InfoContext(strategyCtx, "Search Query", telemetry.WithTraceContext(strategyCtx, "query", ss.Query)...)
		pResp, err := q.searchProwlarr(strategyCtx, ss.Query, req.Category, req.Anime)
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

// filterTorrents validates torrents and returns those that pass validation along with rejection reasons
func (q *QbittHandler) filterTorrents(
	ctx context.Context,
	torrents []*prowlarr.Search,
	strategy *models.SearchStrategy,
) ([]*models.TorrentMatch, map[string]int) {
	possibleTorrents := []*models.TorrentMatch{}
	rejectionReasons := make(map[string]int)

	for _, torrent := range torrents {
		if q.isCorrectTorrent(ctx, torrent, strategy) {
			possibleTorrents = append(possibleTorrents, &models.TorrentMatch{Strategy: strategy, Torrent: torrent})
		} else {
			result := q.validateTorrent(torrent, strategy)
			rejectionReasons[result.Reason]++
		}
	}

	return possibleTorrents, rejectionReasons
}

// logFilteringSummary logs statistics about torrent filtering with rejection breakdown
func (q *QbittHandler) logFilteringSummary(ctx context.Context, total, valid int, rejectionReasons map[string]int, query string) {
	summaryAttrs := []any{
		"total_torrents", total,
		"valid_torrents", valid,
		"rejected_torrents", total - valid,
		"query", query,
	}

	if total-valid > 0 {
		for reason, count := range rejectionReasons {
			summaryAttrs = append(summaryAttrs, fmt.Sprintf("rejected_%s", sanitizeReasonKey(reason)), count)
		}
	}

	q.logger.InfoContext(ctx, "Torrent filtering summary", telemetry.WithTraceContext(ctx, summaryAttrs...)...)
}

// initiateDownloadAndMonitor starts the download and begins monitoring for completion
func (q *QbittHandler) initiateDownloadAndMonitor(
	ctx context.Context,
	match *models.TorrentMatch,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
) dlstatus.EpisodeResult {
	ss := match.Strategy
	infoHash, trackingUUID, err := q.downloadTorrent(ctx, match.Torrent)
	if err != nil {
		q.logger.ErrorContext(ctx, "Failed to download torrent", "error", err)
		// Transient torrent-client error: keep the notification searching for the retry engine.
		q.applyFailureNotification(ctx, ss.TvdbId, dlstatus.CodeTorrentClientError)
		return q.failedResult(ss, dlstatus.CodeTorrentClientError)
	}

	// Update notification: download has started successfully
	q.updateNotificationStatus(ctx, ss.TvdbId, "", notifications.StatusDownloading)

	// Start monitoring goroutine
	wg.Add(1)
	go q.monitorTorrentCompletion(ctx, match, infoHash, trackingUUID, done, wg)

	return dlstatus.EpisodeResult{
		TvdbID:  ss.TvdbId,
		Season:  ss.Season,
		Episode: ss.Episode,
		Outcome: dlstatus.OutcomeDownloading,
	}
}

// updateNotificationStatus updates a notification with the given status and reason
// This is a generalized function used by all notification update operations to reduce code duplication
func (q *QbittHandler) updateNotificationStatus(ctx context.Context, tvdbId, reason string, status notifications.NotificationStatus) {
	notification, err := q.repo.GetNotification(ctx, tvdbId)

	if err != nil {
		q.logger.ErrorContext(ctx, "Failed to find notification for update",
			telemetry.WithTraceContext(ctx,
				"error", err.Error(),
				"tvdb_id", tvdbId,
				"status", string(status))...)
		return
	}

	if notification == nil {
		q.logger.WarnContext(ctx, "No notification found (webserver should have created it)",
			telemetry.WithTraceContext(ctx,
				"tvdb_id", tvdbId,
				"status", string(status))...)
		return
	}

	// Update notification fields
	notification.Status = status
	notification.Reason = reason

	err = q.repo.UpdateNotification(ctx, notification)
	if err != nil {
		q.logger.ErrorContext(ctx, "Failed to update notification",
			telemetry.WithTraceContext(ctx,
				"error", err.Error(),
				"notification_id", notification.ID,
				"status", string(status))...)
		return
	}

	// Build log attributes
	logAttrs := []any{
		"notification_id", notification.ID,
		"status", string(status),
	}
	if reason != "" {
		logAttrs = append(logAttrs, "reason", reason)
	}
}

const (
	defaultMonitorTimeout = 6 * time.Hour
	// monitorFastPollWindow is how long we poll frequently before backing off.
	monitorFastPollWindow = 10 * time.Minute
	monitorFastInterval   = 10 * time.Second
	monitorSlowInterval   = 60 * time.Second
)

// nextPollInterval returns the poll cadence given elapsed time since monitoring began:
// 10s for the first 10 minutes, then 60s thereafter.
func nextPollInterval(elapsed time.Duration) time.Duration {
	if elapsed < monitorFastPollWindow {
		return monitorFastInterval
	}
	return monitorSlowInterval
}

// monitorTimeout reads the overall monitor deadline from MONITOR_TIMEOUT (a Go
// duration like "6h"), falling back to 6h when unset or unparseable.
func monitorTimeout() time.Duration {
	if v := os.Getenv("MONITOR_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultMonitorTimeout
}

// monitorTorrentCompletion watches a torrent until it completes and sends the completion event
func (q *QbittHandler) monitorTorrentCompletion(
	ctx context.Context,
	match *models.TorrentMatch,
	infoHash string,
	trackingUUID string,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	// The winning strategy span represents the (now-complete) search. End it here so the
	// search trace doesn't stay open for the lifetime of this long-lived goroutine.
	searchSpan := trace.SpanFromContext(ctx)
	searchSpanCtx := trace.SpanContextFromContext(ctx)
	searchSpan.End()

	// Start a self-contained monitor span as a new root, linked back to the search span,
	// rather than parenting it under the already-ended search/episode spans. The monitor
	// can run for up to MONITOR_TIMEOUT (default 6h), so its spans must not dangle off an
	// ended parent. Use a background context so it's independent of the HTTP request.
	monitorCtx, monitorSpan := torrentTracer.Start(context.Background(), "monitorTorrent",
		trace.WithNewRoot(),
		trace.WithLinks(trace.Link{SpanContext: searchSpanCtx}),
		trace.WithAttributes(
			attribute.String("torrent_title", match.Torrent.Title),
			attribute.String("torrent_hash", infoHash),
		),
	)
	defer monitorSpan.End()

	start := time.Now()
	timeout := monitorTimeout()
	q.logger.InfoContext(monitorCtx, "Starting torrent monitor",
		telemetry.WithTraceContext(monitorCtx,
			"torrent_title", match.Torrent.Title,
			"torrent_hash", infoHash,
			"timeout", timeout.String())...)

	ranRecheck := false
	for {
		elapsed := time.Since(start)

		// Overall deadline: a torrent that never completes shouldn't be watched forever.
		if elapsed >= timeout {
			q.logger.WarnContext(monitorCtx, "Torrent monitor timed out, giving up",
				telemetry.WithTraceContext(monitorCtx,
					"torrent_title", match.Torrent.Title,
					"torrent_hash", infoHash,
					"timeout", timeout.String())...)
			q.updateNotificationStatus(monitorCtx, match.Strategy.TvdbId, dlstatus.CodeMonitorLost.HumanReason(), notifications.StatusFailed)
			q.recordStrategyFailure(monitorCtx, match.Strategy, "monitor timeout")
			monitorSpan.SetStatus(codes.Error, "monitor timeout")
			return
		}

		time.Sleep(nextPollInterval(elapsed))
		q.logger.InfoContext(monitorCtx, "Monitoring torrent status",
			telemetry.WithTraceContext(monitorCtx,
				"torrent_title", match.Torrent.Title,
				"torrent_hash", infoHash)...)

		filter := qbittorrent.TorrentFilterOptions{
			Category: scoutTag,
			Hashes:   []string{infoHash},
		}

		torrents, err := q.queryQbittorrentWithRetries(monitorCtx, filter, 6, 10*time.Second)
		if err != nil {
			q.logger.WarnContext(monitorCtx, "Failed to monitor torrent after retries, marking failed",
				telemetry.WithTraceContext(monitorCtx,
					"torrent_title", match.Torrent.Title,
					"torrent_hash", infoHash,
					"error", err.Error())...)
			q.updateNotificationStatus(monitorCtx, match.Strategy.TvdbId, dlstatus.CodeMonitorLost.HumanReason(), notifications.StatusFailed)
			q.recordStrategyFailure(monitorCtx, match.Strategy, "monitor lost: qbittorrent query exhausted")
			monitorSpan.SetStatus(codes.Error, "max retries reached")
			return
		}

		if len(torrents) == 0 {
			q.logger.InfoContext(monitorCtx, "Torrent not found, continuing watch", "torrent", match.Torrent.Title)
			continue
		}

		torrent := torrents[0]
		if q.didTorrentComplete(&torrent) {
			if !ranRecheck {
				ranRecheck = true
				if err := q.c.RecheckCtx(monitorCtx, []string{torrent.Hash}); err != nil {
					q.logger.WarnContext(monitorCtx, "Failed to recheck torrent", "hash", torrent.Hash, "error", err)
				}
				continue
			}

			q.logger.InfoContext(monitorCtx, "Torrent completed", "title", match.Torrent.Title)
			monitorSpan.SetStatus(codes.Ok, "torrent downloaded successfully")
			done <- models.TorrentCompleteEvent{
				SavePath:    torrent.SavePath,
				Req:         match.Strategy,
				Hash:        match.Torrent.InfoHash,
				UUID:        trackingUUID,
				SpanContext: trace.SpanContextFromContext(monitorCtx),
			}
			break
		} else {
			q.logger.InfoContext(monitorCtx, "Torrent downloading", "title", match.Torrent.Title, "progress", torrent.Progress*100)
		}
	}
}

func (q *QbittHandler) createSearchStrategy(req *tvdb.Media, episode *tvdb.Episode) []*models.SearchStrategy {
	var ss []*models.SearchStrategy

	// Build media names from req.Name + all aliases, with deduplication
	mediaNames := []string{req.Name}
	mediaNames = append(mediaNames, req.Aliases...)

	// Deduplicate
	seen := make(map[string]bool)
	uniqueNames := []string{}
	for _, name := range mediaNames {
		if !seen[name] && name != "" {
			seen[name] = true
			uniqueNames = append(uniqueNames, name)
		}
	}
	showNames := uniqueNames

	paddedSeason := standardizeNumber(episode.SeasonNumber)
	paddedEpisode := standardizeNumber(episode.Number)

	// newShowStrategy builds a seasonal show strategy with the given query and relax level.
	newShowStrategy := func(query, name string, relax int) *models.SearchStrategy {
		return &models.SearchStrategy{
			Query:       query,
			MediaName:   name,
			Season:      episode.SeasonNumber,
			Episode:     episode.Number,
			EpisodeMeta: episode,
			TvdbId:      req.Id,
			RelaxLevel:  relax,
		}
	}

	// newAbsoluteStrategy builds an anime absolute-numbered strategy with the
	// authoritative Exclude:["season","episode"] behavior preserved.
	newAbsoluteStrategy := func(query, name string, relax int) *models.SearchStrategy {
		return &models.SearchStrategy{
			Query:       query,
			MediaName:   name,
			Season:      episode.SeasonNumber,
			Episode:     episode.AbsoluteNumber,
			EpisodeMeta: episode,
			Exclude:     []string{"season", "episode"},
			TvdbId:      req.Id,
			RelaxLevel:  relax,
		}
	}

	// ----- Tier 0: strict (existing behavior) -----
	if req.Anime {
		for _, showName := range showNames {
			// Only create absolute number strategy if TVDB provided one (absoluteNumber > 0).
			if episode.AbsoluteNumber > 0 {
				ss = append(ss, newAbsoluteStrategy(
					fmt.Sprint(showName, " ", standardizeNumber(episode.AbsoluteNumber)), showName, 0))
			}
		}
	}

	for _, format := range []string{"%s S%sE%s", "%s Season %s Episode %s"} {
		for _, showName := range showNames {
			ss = append(ss, newShowStrategy(
				fmt.Sprintf(format, showName, paddedSeason, paddedEpisode), showName, 0))
		}
	}

	// ----- Tier 1: relaxed -----
	for _, showName := range showNames {
		// "{name} {S}x{EE}" e.g. "Show 1x05"
		ss = append(ss, newShowStrategy(
			fmt.Sprintf("%s %dx%s", showName, episode.SeasonNumber, paddedEpisode), showName, 1))

		// Anime absolute UNPADDED "{name} {absolute}"
		if req.Anime && episode.AbsoluteNumber > 0 {
			ss = append(ss, newAbsoluteStrategy(
				fmt.Sprintf("%s %d", showName, episode.AbsoluteNumber), showName, 1))
		}

		// Punctuation-stripped name variant applied to the S##E## format.
		if stripped := stripNamePunctuation(showName); stripped != "" && stripped != showName {
			ss = append(ss, newShowStrategy(
				fmt.Sprintf("%s S%sE%s", stripped, paddedSeason, paddedEpisode), stripped, 1))
		}
	}

	// ----- Tier 2: broad -----
	for _, showName := range showNames {
		// Loose episode token "{name} E{EE}"
		ss = append(ss, newShowStrategy(
			fmt.Sprintf("%s E%s", showName, paddedEpisode), showName, 2))

		// For anime, name-only — rely on validators/parser to match S/E or absolute.
		if req.Anime {
			if episode.AbsoluteNumber > 0 {
				ss = append(ss, newAbsoluteStrategy(showName, showName, 2))
			} else {
				ss = append(ss, newShowStrategy(showName, showName, 2))
			}
		}
	}

	return ss
}

// stripNamePunctuation removes punctuation that commonly differs between TVDB
// titles and release titles: colons, apostrophes (straight and curly), commas
// and hyphens, plus any trailing bracketed or parenthesized year (e.g. "(2019)"
// or "[2019]"). Whitespace is collapsed to single spaces.
func stripNamePunctuation(name string) string {
	s := name

	// Strip a trailing bracketed/parenthesized year, e.g. "Show (2019)" / "Show [2019]".
	s = trailingYearRe.ReplaceAllString(s, "")

	// Remove punctuation characters.
	s = strings.Map(func(r rune) rune {
		switch r {
		case ':', '\'', '’', ',', '-':
			return -1
		}
		return r
	}, s)

	// Collapse whitespace.
	return strings.Join(strings.Fields(s), " ")
}

// createMovieSearchStrategy generates search strategies for movie downloads
// Returns array of SearchStrategy with single movie query
// Note: Indexer selection is handled by calcIndexerIDs() based on req.Anime flag
func (q *QbittHandler) createMovieSearchStrategy(req *tvdb.Media) []*models.SearchStrategy {
	var ss []*models.SearchStrategy

	newMovie := func(query string, relax int) *models.SearchStrategy {
		return &models.SearchStrategy{
			Query:       query,
			MediaName:   req.Name,
			Season:      0,   // Movies have no season
			Episode:     0,   // Movies have no episode (used to detect movie vs show)
			EpisodeMeta: nil, // Movies have no episode metadata
			TvdbId:      req.Id,
			ReleaseYear: req.Year,
			IsMovie:     true, // Flag this as a movie search strategy
			RelaxLevel:  relax,
		}
	}

	// Tier 0: "{name} {year}" (or just name when year unknown).
	tier0 := req.Name
	if req.Year != "" {
		tier0 = fmt.Sprintf("%s %s", req.Name, req.Year)
	}
	ss = append(ss, newMovie(tier0, 0))

	// Tier 1: name-only.
	ss = append(ss, newMovie(req.Name, 1))

	// Tier 2: punctuation-stripped name-only (only if it differs).
	if stripped := stripNamePunctuation(req.Name); stripped != "" && stripped != req.Name {
		ss = append(ss, newMovie(stripped, 2))
	}

	return ss
}

func (q *QbittHandler) didTorrentComplete(torrent *qbittorrent.Torrent) bool {
	q.logger.Debug("torrent state", "state", torrent.State, "progress", torrent.Progress)
	// Complete if fully downloaded (progress == 1.0) OR the original stalled-upload
	// condition. Using progress catches terminal states other than "stalledUP"
	// (e.g. uploading / pausedUP) that would otherwise be missed.
	if torrent.Progress == 1 {
		return true
	}
	return torrent.Completed == torrent.Size && torrent.State == "stalledUP"
}

func (q *QbittHandler) calcCategories(show bool) []int64 {
	if show {
		return []int64{5000}
	} else {
		return []int64{2000}
	}
}

// calcIndexerIDs determines which Prowlarr indexers to use based on media type and anime status
// Anime shows: NYAA only
// Regular shows: 1337x only
// Anime movies: NYAA + 1337x (both indexers for better coverage)
// Regular movies: 1337x only
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

// TorrentValidationResult holds the result of torrent validation
type TorrentValidationResult struct {
	IsValid     bool
	Confidence  float64
	Reason      string
	Parsed      *ParsedTorrent // For TV show torrents
	ParsedMovie *ParsedMovie   // For movie torrents
}

// isCorrectTorrent performs layered validation on a torrent with logging
func (q *QbittHandler) isCorrectTorrent(ctx context.Context, torrent *prowlarr.Search, strategy *models.SearchStrategy) bool {
	result := q.validateTorrent(torrent, strategy)

	// Log rejected torrents with detailed information
	if !result.IsValid {
		logAttrs := []any{
			"torrent_title", torrent.Title,
			"validation_result", "rejected",
			"rejection_reason", result.Reason,
			"expected_media_name", strategy.MediaName,
			"expected_season", strategy.Season,
			"expected_episode", strategy.Episode,
		}

		// Add parsed information if available
		if result.Parsed != nil {
			logAttrs = append(logAttrs,
				"parsed_show_name", result.Parsed.ShowName,
				"parsed_season", result.Parsed.Season,
				"parsed_episode", result.Parsed.Episode,
				"parsed_quality", result.Parsed.Quality,
				"parsed_format", func() string {
					if result.Parsed.IsAbsolute {
						return "absolute"
					} else if result.Parsed.IsSeasonal {
						return "seasonal"
					}
					return "unknown"
				}(),
				"parsed_release_group", result.Parsed.ReleaseGroup,
			)
		}

		q.logger.InfoContext(ctx, "Torrent rejected during validation", telemetry.WithTraceContext(ctx, logAttrs...)...)
	}

	return result.IsValid
}

// validateTorrent performs comprehensive layered validation with confidence scoring
func (q *QbittHandler) validateTorrent(torrent *prowlarr.Search, strategy *models.SearchStrategy) TorrentValidationResult {
	if strategy.IsMovie {
		return q.validateMovieTorrent(torrent, strategy)
	}
	return q.validateShowTorrent(torrent, strategy)
}

// validateShowTorrent performs TV show-specific validation with layered checks
func (q *QbittHandler) validateShowTorrent(torrent *prowlarr.Search, strategy *models.SearchStrategy) TorrentValidationResult {
	// Layer 1: Parse torrent title
	parsed := q.parser.Parse(torrent.Title)
	if parsed == nil {
		return TorrentValidationResult{
			IsValid:    false,
			Confidence: 0.0,
			Reason:     "failed to parse torrent title",
		}
	}

	// Layer 2: Reject multi-episode torrents (per user requirement)
	if parsed.IsMultiEpisode() {
		return TorrentValidationResult{
			IsValid:    false,
			Confidence: 0.0,
			Reason:     "multi-episode torrent rejected",
			Parsed:     parsed,
		}
	}

	// Layer 3: Reject batch/cour releases
	titleLower := strings.ToLower(torrent.Title)
	if strings.Contains(titleLower, "batch") || strings.Contains(titleLower, "cour") {
		return TorrentValidationResult{
			IsValid:    false,
			Confidence: 0.0,
			Reason:     "batch/cour release rejected",
			Parsed:     parsed,
		}
	}

	// Layer 4: Apply strategy-specific exclusions (anime: exclude "season", "episode")
	for _, exclude := range strategy.Exclude {
		if strings.Contains(titleLower, strings.ToLower(exclude)) {
			return TorrentValidationResult{
				IsValid:    false,
				Confidence: 0.0,
				Reason:     fmt.Sprintf("excluded keyword: %s", exclude),
				Parsed:     parsed,
			}
		}
	}

	// Layer 5: Format-aware episode matching
	if parsed.IsAbsolute {
		// For absolute-numbered torrents, check against absolute episode number from metadata
		if strategy.EpisodeMeta != nil && parsed.Episode != strategy.EpisodeMeta.AbsoluteNumber {
			return TorrentValidationResult{
				IsValid:    false,
				Confidence: 0.0,
				Reason:     fmt.Sprintf("absolute episode mismatch: expected %d, got %d", strategy.EpisodeMeta.AbsoluteNumber, parsed.Episode),
				Parsed:     parsed,
			}
		}
	} else if parsed.IsSeasonal {
		// For seasonal format (S##E##), check season and episode numbers
		if parsed.Season != strategy.Season {
			return TorrentValidationResult{
				IsValid:    false,
				Confidence: 0.0,
				Reason:     fmt.Sprintf("season mismatch: expected %d, got %d", strategy.Season, parsed.Season),
				Parsed:     parsed,
			}
		}
		if parsed.Episode != strategy.Episode {
			return TorrentValidationResult{
				IsValid:    false,
				Confidence: 0.0,
				Reason:     fmt.Sprintf("episode mismatch: expected %d, got %d", strategy.Episode, parsed.Episode),
				Parsed:     parsed,
			}
		}
	} else {
		// Torrent doesn't use absolute or seasonal format - reject
		return TorrentValidationResult{
			IsValid:    false,
			Confidence: 0.0,
			Reason:     "torrent must use either absolute or seasonal format",
			Parsed:     parsed,
		}
	}

	// Layer 6: Show name validation
	nameMatchScore := q.scoreNameMatch(parsed.ShowName, strategy.MediaName)
	if nameMatchScore < 0.3 { // Threshold for minimum name match
		return TorrentValidationResult{
			IsValid:    false,
			Confidence: 0.0,
			Reason:     fmt.Sprintf("show name mismatch: parsed=%q, expected=%q", parsed.ShowName, strategy.MediaName),
			Parsed:     parsed,
		}
	}

	// All validation layers passed
	confidence := q.calculateConfidence(parsed, strategy, nameMatchScore)
	return TorrentValidationResult{
		IsValid:    true,
		Confidence: confidence,
		Reason:     "validation passed",
		Parsed:     parsed,
	}
}

// validateMovieTorrent performs movie-specific validation using the movie parser
func (q *QbittHandler) validateMovieTorrent(torrent *prowlarr.Search, strategy *models.SearchStrategy) TorrentValidationResult {
	// Layer 1: Parse movie torrent title
	parsedMovie := q.parser.ParseMovie(torrent.Title)
	if parsedMovie == nil {
		return TorrentValidationResult{
			IsValid:    false,
			Confidence: 0.0,
			Reason:     "failed to parse movie torrent title",
		}
	}

	// Layer 2: Validate movie name match (fuzzy matching like TV shows)
	nameMatchScore := q.scoreNameMatch(parsedMovie.MovieName, strategy.MediaName)
	if nameMatchScore < 0.3 { // Same threshold as TV shows
		return TorrentValidationResult{
			IsValid:     false,
			Confidence:  0.0,
			Reason:      fmt.Sprintf("movie name mismatch: parsed=%q, expected=%q", parsedMovie.MovieName, strategy.MediaName),
			ParsedMovie: parsedMovie,
		}
	}

	// Layer 3: Validate year if available (optional but helps with confidence)
	yearMismatch := false
	if parsedMovie.Year > 0 && strategy.ReleaseYear != "" {
		expectedYear, err := strconv.Atoi(strategy.ReleaseYear)
		if err == nil && parsedMovie.Year != expectedYear {
			// Allow +/- 1 year tolerance for re-releases and regional variations
			yearDiff := parsedMovie.Year - expectedYear
			if yearDiff < -1 || yearDiff > 1 {
				yearMismatch = true
			}
		}
	}

	// Layer 4: Validate quality tier (reject if below minimum acceptable quality)
	qualityTier := parsedMovie.GetQualityTier()
	if qualityTier < 2 { // Reject anything below 480p
		return TorrentValidationResult{
			IsValid:     false,
			Confidence:  0.0,
			Reason:      fmt.Sprintf("quality too low: %s", parsedMovie.Quality),
			ParsedMovie: parsedMovie,
		}
	}

	// Calculate confidence score for movie
	confidence := q.calculateMovieConfidence(parsedMovie, strategy, nameMatchScore, yearMismatch)

	return TorrentValidationResult{
		IsValid:     true,
		Confidence:  confidence,
		Reason:      "movie validation passed",
		ParsedMovie: parsedMovie,
	}
}

// calculateMovieConfidence calculates overall confidence score for a movie match
func (q *QbittHandler) calculateMovieConfidence(parsed *ParsedMovie, strategy *models.SearchStrategy, nameMatchScore float64, yearMismatch bool) float64 {
	// Weighted factors:
	// - Name match: 40%
	// - Year match: 20%
	// - Parse confidence: 20%
	// - Quality: 20%

	confidence := 0.0

	// Name match score (40% - most important for movies)
	confidence += nameMatchScore * 0.40

	// Year match (20%)
	if !yearMismatch && parsed.Year > 0 && strategy.ReleaseYear != "" {
		confidence += 0.20
	} else if parsed.Year == 0 || strategy.ReleaseYear == "" {
		confidence += 0.10 // Partial credit if year unknown
	}

	// Parse confidence (20%)
	confidence += parsed.MatchConfidence * 0.20

	// Quality tier (20%)
	qualityScore := float64(parsed.GetQualityTier()) / 5.0
	confidence += qualityScore * 0.20

	return confidence
}

// normalizeMovieName normalizes movie/show names for more lenient matching
// Handles variations like "Movie & Title" vs "Movie and Title" vs "Movie&Title"
func normalizeMovieName(name string) string {
	// Convert to lowercase
	normalized := strings.ToLower(name)

	// Replace " & " with " and " (space-delimited ampersand)
	normalized = strings.ReplaceAll(normalized, " & ", " and ")

	// Replace "&" with " and " to ensure spaces around "and"
	// This handles cases like "Movie&Title" -> "Movie and Title"
	normalized = strings.ReplaceAll(normalized, "&", " and ")

	// Remove all special characters except spaces and alphanumerics
	// Keep spaces to preserve word boundaries
	var builder strings.Builder
	for _, ch := range normalized {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == ' ' {
			builder.WriteRune(ch)
		}
	}
	normalized = builder.String()

	// Collapse multiple spaces to single space
	normalized = strings.Join(strings.Fields(normalized), " ")

	// Trim leading/trailing spaces
	return strings.TrimSpace(normalized)
}

// scoreNameMatch scores how well the parsed show name matches the expected name
// Simplified to use substring matching since Prowlarr's search already filters relevant results
func (q *QbittHandler) scoreNameMatch(parsedName, expectedName string) float64 {
	// Normalize both names for more lenient matching
	normalizedParsed := normalizeMovieName(parsedName)
	normalizedExpected := normalizeMovieName(expectedName)

	// Exact match after normalization
	if normalizedParsed == normalizedExpected {
		return 1.0
	}

	// Substring match (bidirectional)
	if strings.Contains(normalizedParsed, normalizedExpected) ||
		strings.Contains(normalizedExpected, normalizedParsed) {
		return 0.8
	}

	return 0.0
}

// calculateConfidence calculates overall confidence score for a match
func (q *QbittHandler) calculateConfidence(parsed *ParsedTorrent, strategy *models.SearchStrategy, nameMatchScore float64) float64 {
	// Weighted factors:
	// - Name match: 30%
	// - Format match: 20% (absolute vs seasonal preference)
	// - Parse confidence: 20%
	// - Quality: 15%
	// - Version: 15% (v2/v3 preferred)

	confidence := 0.0

	// Name match score (30%)
	confidence += nameMatchScore * 0.30

	// Format match (20%)
	isAnime := len(strategy.Exclude) > 0
	if isAnime && parsed.IsAbsolute {
		confidence += 0.20 // Prefer absolute for anime
	} else if !isAnime && parsed.IsSeasonal {
		confidence += 0.20 // Prefer seasonal for regular shows
	} else {
		confidence += 0.10 // Acceptable but not preferred
	}

	// Parse confidence (20%)
	confidence += parsed.MatchConfidence * 0.20

	// Quality tier (15%)
	qualityScore := float64(parsed.GetQualityTier()) / 5.0
	confidence += qualityScore * 0.15

	// Version preference (15%) - v2/v3 are corrections/improvements
	if parsed.Version >= 2 {
		confidence += 0.15
	} else {
		confidence += 0.10 // v1 or no version still acceptable
	}

	return confidence
}

// sanitizeReasonKey converts rejection reason strings into valid log attribute keys
// Examples: "show name mismatch: parsed=\"X\", expected=\"Y\"" -> "show_name_mismatch"
func sanitizeReasonKey(reason string) string {
	// Take only the part before the colon (if any)
	if idx := strings.Index(reason, ":"); idx != -1 {
		reason = reason[:idx]
	}

	// Replace spaces with underscores and remove special characters
	key := strings.ReplaceAll(reason, " ", "_")
	key = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1 // Remove character
	}, key)

	return strings.ToLower(key)
}

func (q *QbittHandler) downloadTorrent(ctx context.Context, torrent *prowlarr.Search) (string, string, error) {
	// Generate UUID for torrent tracking
	trackingUUID := uuid.New().String()

	q.logger.InfoContext(ctx, "Downloading torrent",
		"filename",
		torrent.FileName,
		"hash", torrent.InfoHash,
		"guid", torrent.GUID,
		"tracking_uuid", trackingUUID,
		"torrent_meta", torrent)

	torrentSavePath := baseSavePath + "/" + torrent.Title
	q.logger.InfoContext(ctx, "Torrent save path", "path", torrentSavePath)

	if err := os.Mkdir(torrentSavePath, 0777); err != nil && !os.IsExist(err) {
		q.logger.ErrorContext(ctx, "Error creating directory", "value", torrentSavePath, "error", err)
		return "", "", err
	}

	// Prepare torrent add options using new library
	addOpts := qbittorrent.TorrentAddOptions{
		SavePath: torrentSavePath,
		Category: scoutTag,
		Tags:     trackingUUID,
	}
	options := addOpts.Prepare()

	var url string // If downloadURL is empty, that means we have a magnet
	if torrent.DownloadURL == "" {
		url = torrent.GUID
	} else {
		url = torrent.DownloadURL
	}

	if err := q.c.AddTorrentFromUrlCtx(ctx, url, options); err != nil {
		q.logger.ErrorContext(ctx, "Error downloading torrent", "error", err)
		return "", "", err
	}

	// If this was a .torrent file (not a magnet), resolve the InfoHash from qBittorrent
	if torrent.InfoHash == "" {
		q.logger.InfoContext(ctx, "Torrent file detected, resolving InfoHash from qBittorrent",
			"torrent_title", torrent.Title,
			"download_url", torrent.DownloadURL,
			"prowlarr_hash", torrent.InfoHash,
			"tracking_uuid", trackingUUID)

		resolvedHash, err := q.resolveInfoHashFromQbittorrent(ctx, trackingUUID)
		if err != nil {
			q.logger.ErrorContext(ctx, "Failed to resolve InfoHash from qBittorrent via UUID tag",
				"error", err,
				"torrent_title", torrent.Title,
				"tracking_uuid", trackingUUID)
			return "", "", fmt.Errorf("failed to resolve InfoHash via UUID tag for torrent '%s': %w", torrent.Title, err)
		}

		q.logger.InfoContext(ctx, "Successfully resolved InfoHash for .torrent file",
			"torrent_title", torrent.Title,
			"resolved_hash", resolvedHash,
			"tracking_uuid", trackingUUID)
		return resolvedHash, trackingUUID, nil
	}

	// Magnet link - InfoHash already available from Prowlarr
	q.logger.InfoContext(ctx, "Magnet link detected, using InfoHash from GUID",
		"torrent_title", torrent.Title,
		"infohash", torrent.InfoHash)
	return torrent.InfoHash, trackingUUID, nil
}

func (q *QbittHandler) queryQbittorrentWithRetries(
	ctx context.Context,
	filter qbittorrent.TorrentFilterOptions,
	maxRetries int,
	retryDelay time.Duration) ([]qbittorrent.Torrent, error) {
	var torrents []qbittorrent.Torrent
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		q.logger.InfoContext(ctx, "Querying qBittorrent",
			telemetry.WithTraceContext(ctx,
				"attempt", attempt,
				"max_retries", maxRetries,
				"filter_category", filter.Category,
				"filter_hashes", filter.Hashes)...)

		torrents, lastErr = q.c.GetTorrentsCtx(ctx, filter)
		if lastErr == nil {
			if len(torrents) > 0 {
				q.logger.InfoContext(ctx, "Successfully queried qBittorrent",
					telemetry.WithTraceContext(ctx,
						"attempt", attempt,
						"torrents", torrents,
						"torrents_returned", len(torrents))...)
				return torrents, nil
			}

			// API succeeded but no torrents found - retry
			q.logger.InfoContext(ctx, "qBittorrent query returned empty results",
				telemetry.WithTraceContext(ctx,
					"attempt", attempt,
					"max_retries", maxRetries,
					"filter_category", filter.Category,
					"filter_tag", filter.Tag)...)
		} else {
			// API error - retry
			q.logger.WarnContext(ctx, "Failed to query qBittorrent",
				telemetry.WithTraceContext(ctx,
					"attempt", attempt,
					"max_retries", maxRetries,
					"error", lastErr.Error())...)
		}

		if attempt < maxRetries {
			q.logger.InfoContext(ctx, "Retrying qBittorrent query",
				telemetry.WithTraceContext(ctx,
					"retry_delay", retryDelay.String(),
					"next_attempt", attempt+1)...)
			time.Sleep(retryDelay)
		}
	}

	// All retries exhausted
	if lastErr != nil {
		q.logger.ErrorContext(ctx, "Max retries exhausted querying qBittorrent",
			telemetry.WithTraceContext(ctx,
				"max_retries", maxRetries,
				"last_error", lastErr.Error())...)
		return nil, fmt.Errorf("failed to query qBittorrent after %d attempts: %w", maxRetries, lastErr)
	}

	// API succeeded but no torrents found after all retries
	q.logger.ErrorContext(ctx, "Max retries exhausted querying qBittorrent",
		telemetry.WithTraceContext(ctx,
			"max_retries", maxRetries,
			"reason", "empty results")...)
	return nil, fmt.Errorf("no torrents found in qBittorrent after %d attempts with filter: category=%s, tag=%s",
		maxRetries, filter.Category, filter.Tag)
}

func (q *QbittHandler) resolveInfoHashFromQbittorrent(ctx context.Context, trackingUUID string) (string, error) {
	q.logger.InfoContext(ctx, "Resolving InfoHash from qBittorrent", "tracking_uuid", trackingUUID)

	// Query torrent by UUID tag with retries
	filter := qbittorrent.TorrentFilterOptions{
		Category: scoutTag,
		Tag:      trackingUUID,
	}

	torrents, err := q.queryQbittorrentWithRetries(ctx, filter, 10, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to resolve hash from qBittorrent: %w", err)
	}

	if len(torrents) == 0 {
		q.logger.WarnContext(ctx, "Could not find torrent in qBittorrent with UUID tag",
			"tracking_uuid", trackingUUID)
		return "", fmt.Errorf("torrent not found in qBittorrent with UUID: %s", trackingUUID)
	}

	// Should only be one torrent with this unique UUID tag
	torrent := torrents[0]
	q.logger.InfoContext(ctx, "Successfully resolved InfoHash from qBittorrent",
		"tracking_uuid", trackingUUID,
		"qbittorrent_name", torrent.Name,
		"resolved_hash", torrent.Hash)

	return torrent.Hash, nil
}

func (h *QbittHandler) sortTorrentsByQuality(matches []*models.TorrentMatch) {
	sort.SliceStable(matches, func(i, j int) bool {
		qualityOrder := func(title string) int {
			titleLower := strings.ToLower(title)
			if strings.Contains(titleLower, "4k") {
				return 0
			}
			if strings.Contains(titleLower, "2160p") {
				return 1
			}
			if strings.Contains(titleLower, "1080p") {
				return 2
			}
			if strings.Contains(titleLower, "720p") {
				return 3
			}
			if strings.Contains(titleLower, "480p") {
				return 4
			}
			return 5
		}
		return qualityOrder(matches[i].Torrent.Title) < qualityOrder(matches[j].Torrent.Title)
	})
}

// sortBySeedersDESC sorts torrent matches by seeder count in descending order
// This pre-sorts torrents to prioritize healthy torrents with more seeders
func (h *QbittHandler) sortBySeedersDESC(matches []*models.TorrentMatch) {
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Torrent.Seeders > matches[j].Torrent.Seeders
	})
}

// TorrentMatchWithScore extends TorrentMatch with confidence scoring
type TorrentMatchWithScore struct {
	*models.TorrentMatch
	ValidationResult TorrentValidationResult
	FinalScore       float64
}

// formatTorrentMatches converts a slice of matches into a readable string representation
func formatTorrentMatches(matches []*models.TorrentMatch) string {
	if len(matches) == 0 {
		return "[]"
	}

	result := "["
	for i, m := range matches {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf("{Title: %s, Seeders: %d}", m.Torrent.Title, m.Torrent.Seeders)
	}
	result += "]"
	return result
}

// baseConfidenceCutoff is the minimum validation confidence required for a match
// at relax levels 0 and 1. Relax level >= 2 (broad queries) requires this plus a bump.
const (
	baseConfidenceCutoff = 0.6
	relaxConfidenceBump  = 0.1
	broadRelaxLevel      = 2
)

// relaxLevelOf returns the strategy's relax level, defaulting to 0 for a nil strategy.
func relaxLevelOf(ss *models.SearchStrategy) int {
	if ss == nil {
		return 0
	}
	return ss.RelaxLevel
}

// relaxConfidenceCutoff returns the minimum confidence a match must clear given
// its strategy's relax level. Broad (level >= 2) queries are gated higher.
func relaxConfidenceCutoff(ss *models.SearchStrategy) float64 {
	if relaxLevelOf(ss) >= broadRelaxLevel {
		return baseConfidenceCutoff + relaxConfidenceBump
	}
	return baseConfidenceCutoff
}

func (q *QbittHandler) pickBestTorrent(ctx context.Context, matches []*models.TorrentMatch, mediaType string, isAnime bool) *models.TorrentMatch {
	if len(matches) == 0 {
		return nil
	}

	q.logger.InfoContext(ctx, "Picking the best torrent",
		"total_matches", len(matches),
		"media_type", mediaType,
		"is_anime", isAnime,
		"torrents", formatTorrentMatches(matches))

	// Get preferred uploaders
	preferred, err := q.repo.GetPreferredUploaders(ctx, mediaType, isAnime)
	if err != nil {
		q.logger.ErrorContext(ctx, "Error querying uploader preferences", "error", err)
	}

	if isAnime {
		preferred = append(preferred, "SubsPlease", "Erai-raws")
	}

	if mediaType == "movie" {
		preferred = append(preferred, "QxR", "YIFY", "GalaxyRG")
	}

	q.logger.InfoContext(ctx, "Using preferred uploaders", "uploaders", preferred)

	// Score each match
	scoredMatches := make([]*TorrentMatchWithScore, 0, len(matches))
	for i, match := range matches {
		q.logger.InfoContext(ctx, "Evaluating torrent for selection",
			"index", i+1,
			"total", len(matches),
			"torrent_title", match.Torrent.Title,
			"seeders", match.Torrent.Seeders,
			"strategy_is_movie", match.Strategy.IsMovie)

		// Re-validate to get confidence score and parsed info
		validationResult := q.validateTorrent(match.Torrent, match.Strategy)

		q.logger.InfoContext(ctx, "Re-validation result",
			"torrent_title", match.Torrent.Title,
			"is_valid", validationResult.IsValid,
			"confidence", validationResult.Confidence,
			"reason", validationResult.Reason,
			"has_parsed_torrent", validationResult.Parsed != nil,
			"has_parsed_movie", validationResult.ParsedMovie != nil)

		if !validationResult.IsValid {
			q.logger.WarnContext(ctx, "Torrent failed re-validation, skipping",
				"torrent_title", match.Torrent.Title,
				"reason", validationResult.Reason)
			continue // Skip invalid matches
		}

		// Relaxation guardrail: broad (RelaxLevel >= 2) queries cast a wide net, so
		// require a higher minimum confidence before accepting their matches. The
		// validators above stay authoritative; this is an extra confidence gate.
		minConfidence := relaxConfidenceCutoff(match.Strategy)
		if validationResult.Confidence < minConfidence {
			q.logger.WarnContext(ctx, "Torrent below relax-level confidence cutoff, skipping",
				"torrent_title", match.Torrent.Title,
				"confidence", validationResult.Confidence,
				"min_confidence", minConfidence,
				"relax_level", relaxLevelOf(match.Strategy))
			continue
		}

		// Calculate final score with quality as highest priority
		finalScore := validationResult.Confidence

		// Quality boost (highest priority) - year-aware quality selection
		// Recognizes that 4K didn't exist before 2012, prefers native HD for older content
		qualityBoost := 0.0
		qualityTier := 0
		quality := ""

		// Get quality from the appropriate parsed object
		if validationResult.ParsedMovie != nil {
			qualityTier = validationResult.ParsedMovie.GetQualityTier()
			quality = validationResult.ParsedMovie.Quality
		} else if validationResult.Parsed != nil {
			qualityTier = validationResult.Parsed.GetQualityTier()
			quality = validationResult.Parsed.Quality
		}

		// Calculate year-aware quality boost
		releaseYear := ""
		if match.Strategy != nil {
			releaseYear = match.Strategy.ReleaseYear
		}
		qualityBoost = calculateYearAwareQualityBoost(qualityTier, releaseYear)
		finalScore += qualityBoost

		q.logger.InfoContext(ctx, "Calculated year-aware quality boost",
			"torrent_title", match.Torrent.Title,
			"quality", quality,
			"quality_tier", qualityTier,
			"release_year", releaseYear,
			"quality_boost", qualityBoost,
			"confidence", validationResult.Confidence)

		// Uploader preference boost (second priority) - reduced weight to prioritize quality
		uploaderBoost := 0.0
		matchedUploader := ""
		for i, uploader := range preferred {
			if strings.Contains(strings.ToLower(match.Torrent.Title), strings.ToLower(uploader)) {
				// Higher boost for earlier uploaders in the list
				uploaderBoost = 0.25 * (1.0 - float64(i)*0.1)
				if uploaderBoost < 0.1 {
					uploaderBoost = 0.1
				}
				matchedUploader = uploader
				break
			}
		}
		finalScore += uploaderBoost

		q.logger.InfoContext(ctx, "Calculated final score",
			"torrent_title", match.Torrent.Title,
			"final_score", finalScore,
			"uploader_boost", uploaderBoost,
			"matched_uploader", matchedUploader)

		scoredMatches = append(scoredMatches, &TorrentMatchWithScore{
			TorrentMatch:     match,
			ValidationResult: validationResult,
			FinalScore:       finalScore,
		})
	}

	q.logger.InfoContext(ctx, "Torrent scoring complete",
		"total_evaluated", len(matches),
		"total_scored", len(scoredMatches),
		"total_rejected", len(matches)-len(scoredMatches))

	if len(scoredMatches) == 0 {
		q.logger.WarnContext(ctx, "No torrents passed re-validation, cannot select best torrent")
		return nil
	}

	// Sort by final score (descending)
	sort.SliceStable(scoredMatches, func(i, j int) bool {
		return scoredMatches[i].FinalScore > scoredMatches[j].FinalScore
	})

	bestMatch := scoredMatches[0]

	// Get quality and release group from appropriate parsed object
	quality := ""
	releaseGroup := ""
	if bestMatch.ValidationResult.ParsedMovie != nil {
		quality = bestMatch.ValidationResult.ParsedMovie.Quality
		releaseGroup = bestMatch.ValidationResult.ParsedMovie.ReleaseGroup
	} else if bestMatch.ValidationResult.Parsed != nil {
		quality = bestMatch.ValidationResult.Parsed.Quality
		releaseGroup = bestMatch.ValidationResult.Parsed.ReleaseGroup
	}

	q.logger.InfoContext(ctx, "Selected best torrent",
		"title", bestMatch.Torrent.Title,
		"score", bestMatch.FinalScore,
		"confidence", bestMatch.ValidationResult.Confidence,
		"quality", quality,
		"releaseGroup", releaseGroup,
		"seeders", bestMatch.Torrent.Seeders,
	)

	return bestMatch.TorrentMatch
}

// calculateYearAwareQualityBoost adjusts quality preferences based on media release year.
// This recognizes that 4K content didn't exist before 2012, and "4K" versions of older content
// are upscaled from lower resolution sources, making native HD releases superior.
//
// Year tiers:
//   - Pre-2012: 4K didn't exist - penalize 4K, strongly prefer native 1080p
//   - 2012-2016: Early 4K era - moderate 4K preference, most content still 1080p
//   - 2017+: Modern era - strong 4K preference, native 4K production common
//
// Returns quality boost value to be added to the torrent's score.
func calculateYearAwareQualityBoost(qualityTier int, releaseYear string) float64 {
	// Parse release year, default to 0 if invalid/missing (triggers modern logic)
	year := 0
	if releaseYear != "" {
		if parsed, err := strconv.Atoi(releaseYear); err == nil {
			year = parsed
		}
	}

	// Default to modern logic if year is missing or invalid
	if year == 0 {
		switch qualityTier {
		case 5: // 4K/2160p
			return 2.0
		case 4: // 1080p
			return 0.6
		case 3: // 720p
			return 0.3
		default:
			return 0.0
		}
	}

	// Pre-2012: 4K didn't exist, penalize upscaled content
	if year < 2012 {
		switch qualityTier {
		case 5: // 4K/2160p - upscaled, inferior to native HD
			return -0.5
		case 4: // 1080p - best quality for this era
			return 1.0
		case 3: // 720p - acceptable, original may have been SD anyway
			return 0.5
		default:
			return 0.0
		}
	}

	// 2012-2016: Early 4K era, limited native 4K content
	if year >= 2012 && year < 2017 {
		switch qualityTier {
		case 5: // 4K/2160p - some native content, moderate preference
			return 1.0
		case 4: // 1080p - still the standard for most content
			return 0.6
		case 3: // 720p - acceptable
			return 0.3
		default:
			return 0.0
		}
	}

	// 2017+: Modern era, native 4K production common
	switch qualityTier {
	case 5: // 4K/2160p - strong preference for modern content
		return 2.0
	case 4: // 1080p - still good quality
		return 0.6
	case 3: // 720p - acceptable
		return 0.3
	default:
		return 0.0
	}
}

// RemoveUUIDTag removes the UUID tracking tag from a torrent after processing is complete
func (q *QbittHandler) RemoveUUIDTag(ctx context.Context, hash string, uuid string) error {
	q.logger.InfoContext(ctx, "Removing UUID tag from torrent",
		"hash", hash,
		"uuid", uuid)

	err := q.c.RemoveTagsCtx(ctx, []string{hash}, uuid)
	if err != nil {
		q.logger.WarnContext(ctx, "Failed to remove UUID tag from torrent",
			"hash", hash,
			"uuid", uuid,
			"error", err)
		return err
	}

	q.logger.InfoContext(ctx, "Successfully removed UUID tag from torrent",
		"hash", hash,
		"uuid", uuid)
	return nil
}
