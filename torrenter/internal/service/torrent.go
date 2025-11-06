package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	tvdb "shared/media"
	"shared/telemetry"
	"torrenter/internal/models"

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

func (q *QbittHandler) searchProwlarr(ctx context.Context, query, category string, isAnime bool) ([]*prowlarr.Search, error) {
	q.logger.InfoContext(ctx, "Searching Prowlarr", "query", query, "category", category)

	resp, err := q.p.SearchContext(ctx, prowlarr.SearchInput{
		Query:      query,
		IndexerIDs: q.calcIndexerIDs(isAnime),
		Limit:      500,
		Categories: q.calcCategories(category == "series"),
	})

	if err != nil {
		q.logger.Error("Error searching Prowlarr", "error", err)
		return nil, err
	}

	return resp, nil
}

func (q *QbittHandler) HandleDownload(ctx context.Context, req *tvdb.Media, done chan<- models.TorrentCompleteEvent) error {
	if req.Category != "series" {
		return nil
	}

	// Filter out episodes that already exist in Plex
	episodesToDownload, err := q.filterExistingEpisodes(ctx, req)
	if err != nil {
		return err
	}

	if len(episodesToDownload) == 0 {
		q.logger.InfoContext(ctx, "All episodes already exist in Plex, nothing to download", telemetry.WithTraceContext(ctx, "show", req.Name)...)
		return nil
	}

	q.logger.InfoContext(ctx, "Episodes to download after filtering", telemetry.WithTraceContext(ctx, "show", req.Name,
		"total", len(req.Metadata.Episodes), "to_download", len(episodesToDownload), "show_aliases", req.Aliases)...)

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
		if err := q.processEpisodeDownload(ctx, episodeIdx, strategies, req, done, &wg); err != nil {
			return err
		}
	}

	// Close the channel after all monitoring goroutines complete
	go func() {
		wg.Wait()
		close(done)
		q.logger.InfoContext(ctx, "All torrents processed, channel closed")
	}()

	return nil
}

// filterExistingEpisodes checks which episodes already exist in Plex and returns only those that need downloading
func (q *QbittHandler) filterExistingEpisodes(ctx context.Context, req *tvdb.Media) ([]tvdb.Episode, error) {
	ctx, span := torrentTracer.Start(ctx, "filterExistingEpisodes")
	defer span.End()

	span.SetAttributes(
		attribute.String("show_name", req.Name),
		attribute.String("tvdb_id", req.Id),
		attribute.Int("total_episodes", len(req.Metadata.Episodes)),
	)

	episodesToDownload := []tvdb.Episode{}
	for _, episode := range req.Metadata.Episodes {
		exists, err := q.repo.EpisodeExistsByTvdbId(ctx, req.Id, episode.SeasonNumber, episode.Number)
		if err != nil {
			q.logger.WarnContext(ctx, "Failed to check episode existence by TVDB ID",
				telemetry.WithTraceContext(ctx, "tvdb_id", req.Id, "season", episode.SeasonNumber, "episode", episode.Number, "error", err.Error())...)
		}

		if exists {
			q.logger.InfoContext(ctx, "Episode already exists in Plex, skipping",
				telemetry.WithTraceContext(ctx, "name", req.Name, "season", episode.SeasonNumber, "episode", episode.Number)...)
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

	return episodesToDownload, nil
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

// processEpisodeDownload handles the complete download workflow for a single episode
func (q *QbittHandler) processEpisodeDownload(
	ctx context.Context,
	episodeIdx int,
	strategies []*models.SearchStrategy,
	req *tvdb.Media,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
) error {
	// Create episode-level parent span
	episodeCtx, episodeSpan := torrentTracer.Start(ctx, "searchEpisode")
	if len(strategies) > 0 {
		episodeSpan.SetAttributes(
			attribute.Int("episode_index", episodeIdx),
			attribute.Int("season", strategies[0].Season),
			attribute.Int("episode", strategies[0].Episode),
			attribute.Int("strategy_count", len(strategies)),
			attribute.String("media_name", strategies[0].MediaName),
		)
	}

	// Execute all search strategies for this episode
	bestMatches, strategyContexts, err := q.executeSearchStrategies(episodeCtx, strategies, req)
	if err != nil {
		episodeSpan.SetStatus(codes.Error, "search failed")
		episodeSpan.End()
		return err
	}

	// Select the best torrent from all matches
	q.sortBySeedersDESC(bestMatches)
	q.sortTorrentsByQuality(bestMatches)
	match := q.pickBestTorrent(episodeCtx, bestMatches, req.Category, req.Anime)

	// End all non-winning strategy spans
	for ss, strategyCtx := range strategyContexts {
		if match == nil || ss != match.Strategy {
			span := trace.SpanFromContext(strategyCtx)
			span.SetStatus(codes.Ok, "strategy not selected")
			span.End()
		}
	}

	if match == nil {
		q.logger.WarnContext(episodeCtx, "No suitable torrent found", "show", req.Name)
		ss := strategies[0]
		err := q.repo.InsertDownloadHistory(episodeCtx, req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, "", "failure", "no suitable torrent found")
		if err != nil {
			q.logger.ErrorContext(episodeCtx, "Failed to insert download history", "error", err)
		}
		episodeSpan.SetStatus(codes.Ok, "no suitable torrent found")
		episodeSpan.End()
		return nil
	}

	// Download the torrent and start monitoring
	winningCtx := strategyContexts[match.Strategy]
	return q.initiateDownloadAndMonitor(match, winningCtx, done, wg)
}

// executeSearchStrategies runs all search strategies and collects matching torrents
func (q *QbittHandler) executeSearchStrategies(
	ctx context.Context,
	strategies []*models.SearchStrategy,
	req *tvdb.Media,
) ([]*models.TorrentMatch, map[*models.SearchStrategy]context.Context, error) {
	bestMatches := []*models.TorrentMatch{}
	strategyContexts := make(map[*models.SearchStrategy]context.Context)

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
			strategySpan.SetStatus(codes.Error, "failed to search Prowlarr")
			strategySpan.RecordError(err)
			strategySpan.End()
			return nil, nil, fmt.Errorf("failed to search Prowlarr: %w", err)
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
			err := q.repo.InsertDownloadHistory(strategyCtx, req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, "", "failure", "no matching torrents found")
			if err != nil {
				q.logger.ErrorContext(strategyCtx, "Failed to insert download history", "error", err)
			}
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

	return bestMatches, strategyContexts, nil
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
	match *models.TorrentMatch,
	winningCtx context.Context,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
) error {
	ss := match.Strategy
	err := q.repo.InsertDownloadHistory(
		winningCtx,
		ss.MediaName,
		ss.Season,
		ss.Episode,
		ss.EpisodeMeta.AbsoluteNumber,
		match.Torrent.InfoHash,
		"downloading",
		"")
	if err != nil {
		q.logger.ErrorContext(winningCtx, "Failed to insert download history", "error", err)
	}

	infoHash, trackingUUID, err := q.downloadTorrent(winningCtx, match.Torrent)
	if err != nil {
		q.logger.ErrorContext(winningCtx, "Failed to download torrent", "error", err)
		return err
	}

	// Start monitoring goroutine
	wg.Add(1)
	go q.monitorTorrentCompletion(winningCtx, match, infoHash, trackingUUID, done, wg)

	return nil
}

// monitorTorrentCompletion watches a torrent until it completes and sends the completion event
func (q *QbittHandler) monitorTorrentCompletion(
	parentCtx context.Context,
	match *models.TorrentMatch,
	infoHash string,
	trackingUUID string,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
) {
	// Extract span from context
	winningSpan := trace.SpanFromContext(parentCtx)

	// Create a background context independent of the HTTP request lifecycle
	monitorCtx := trace.ContextWithSpanContext(context.Background(), trace.SpanContextFromContext(parentCtx))

	defer wg.Done()
	defer winningSpan.End()

	ranRecheck := false
	for {
		time.Sleep(10 * time.Second)
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
			q.logger.WarnContext(monitorCtx, "Failed to monitor torrent after retries, stopping watch",
				telemetry.WithTraceContext(monitorCtx,
					"torrent_title", match.Torrent.Title,
					"torrent_hash", infoHash,
					"error", err.Error())...)
			winningSpan.SetStatus(codes.Error, "max retries reached")
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
				q.c.RecheckCtx(monitorCtx, []string{torrent.Hash})
				continue
			}

			q.logger.InfoContext(monitorCtx, "Torrent completed", "title", match.Torrent.Title)
			winningSpan.SetStatus(codes.Ok, "torrent downloaded successfully")
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

	if req.Anime {
		for _, showName := range showNames {
			ss = append(ss, &models.SearchStrategy{
				Query:       fmt.Sprint(showName, " ", standardizeNumber(episode.AbsoluteNumber)),
				MediaName:   showName,
				Season:      episode.SeasonNumber,
				Episode:     episode.AbsoluteNumber,
				EpisodeMeta: episode,
				Exclude:     []string{"season", "episode"},
				TvdbId:      req.Id,
			})
		}
	}

	queryFormats := []string{
		"%s S%sE%s",
		"%s Season %s Episode %s",
	}

	for _, format := range queryFormats {
		for _, showName := range showNames {

			ss = append(ss, &models.SearchStrategy{
				Query:       fmt.Sprintf(format, showName, standardizeNumber(episode.SeasonNumber), standardizeNumber(episode.Number)),
				MediaName:   showName,
				Season:      episode.SeasonNumber,
				Episode:     episode.Number,
				EpisodeMeta: episode,
				TvdbId:      req.Id,
			})
		}
	}

	return ss
}

func (q *QbittHandler) didTorrentComplete(torrent *qbittorrent.Torrent) bool {
	fmt.Printf("Torrent State: %s\n", torrent.State)
	if torrent.Completed == torrent.Size && torrent.State == "stalledUP" {
		return true
	}
	return false
}

func (q *QbittHandler) calcCategories(show bool) []int64 {
	if show {
		return []int64{5000}
	} else {
		return []int64{2000}
	}
}

func (q *QbittHandler) calcIndexerIDs(isAnime bool) []int64 {
	if isAnime {
		return []int64{NYAA_ID}
	} else {
		return []int64{ONE337x_ID}
	}

}

// TorrentValidationResult holds the result of torrent validation
type TorrentValidationResult struct {
	IsValid    bool
	Confidence float64
	Reason     string
	Parsed     *ParsedTorrent
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

// scoreNameMatch scores how well the parsed show name matches the expected name
// Simplified to use substring matching since Prowlarr's search already filters relevant results
func (q *QbittHandler) scoreNameMatch(parsedName, expectedName string) float64 {
	parsedLower := strings.ToLower(parsedName)
	expectedLower := strings.ToLower(expectedName)

	// Exact match
	if parsedLower == expectedLower {
		return 1.0
	}

	// Substring match (bidirectional)
	if strings.Contains(parsedLower, expectedLower) || strings.Contains(expectedLower, parsedLower) {
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

func (q *QbittHandler) pickBestTorrent(ctx context.Context, matches []*models.TorrentMatch, mediaType string, isAnime bool) *models.TorrentMatch {
	if len(matches) == 0 {
		return nil
	}

	q.logger.InfoContext(ctx, "Picking the best torrent", "torrents", matches)

	// Get preferred uploaders
	preferred, err := q.repo.GetPreferredUploaders(ctx, mediaType, isAnime)
	if err != nil {
		q.logger.ErrorContext(ctx, "Error querying uploader preferences", "error", err)
	}

	if isAnime {
		preferred = append(preferred, "SubsPlease", "Erai-raws")
	}

	// Score each match
	scoredMatches := make([]*TorrentMatchWithScore, 0, len(matches))
	for _, match := range matches {
		// Re-validate to get confidence score and parsed info
		validationResult := q.validateTorrent(match.Torrent, match.Strategy)
		if !validationResult.IsValid {
			continue // Skip invalid matches
		}

		// Calculate final score with quality as highest priority
		finalScore := validationResult.Confidence

		// Quality boost (highest priority) - ensures quality dominates selection
		// Weights increased to ensure 2160p always beats 1080p even with preferred uploader
		qualityBoost := 0.0
		switch validationResult.Parsed.GetQualityTier() {
		case 5: // 4K/2160p
			qualityBoost = 2.0
		case 4: // 1080p
			qualityBoost = 0.6
		case 3: // 720p
			qualityBoost = 0.3
		}
		finalScore += qualityBoost

		// Uploader preference boost (second priority) - reduced weight to prioritize quality
		uploaderBoost := 0.0
		for i, uploader := range preferred {
			if strings.Contains(strings.ToLower(match.Torrent.Title), strings.ToLower(uploader)) {
				// Higher boost for earlier uploaders in the list
				uploaderBoost = 0.25 * (1.0 - float64(i)*0.1)
				if uploaderBoost < 0.1 {
					uploaderBoost = 0.1
				}
				break
			}
		}
		finalScore += uploaderBoost

		scoredMatches = append(scoredMatches, &TorrentMatchWithScore{
			TorrentMatch:     match,
			ValidationResult: validationResult,
			FinalScore:       finalScore,
		})
	}

	if len(scoredMatches) == 0 {
		return nil
	}

	// Sort by final score (descending)
	sort.SliceStable(scoredMatches, func(i, j int) bool {
		return scoredMatches[i].FinalScore > scoredMatches[j].FinalScore
	})

	bestMatch := scoredMatches[0]
	q.logger.InfoContext(ctx, "Selected best torrent",
		"title", bestMatch.Torrent.Title,
		"score", bestMatch.FinalScore,
		"confidence", bestMatch.ValidationResult.Confidence,
		"quality", bestMatch.ValidationResult.Parsed.Quality,
		"releaseGroup", bestMatch.ValidationResult.Parsed.ReleaseGroup,
	)

	return bestMatch.TorrentMatch
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
