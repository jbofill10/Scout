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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	ONE337x_ID = int64(5) // General
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
		IndexerIDs: q.calcIndexerIDs(category, isAnime),
	})

	if err != nil {
		q.logger.Error("Error searching Prowlarr", "error", err)
		return nil, err
	}

	return resp, nil
}

func (q *QbittHandler) HandleDownload(ctx context.Context, req *tvdb.Media, done chan<- models.TorrentCompleteEvent) error {

	searchStrategies := [][]*models.SearchStrategy{}
	if req.Category == "series" {
		// Pre-download filtering: Check if episodes already exist in Plex
		ctx, span := torrentTracer.Start(ctx, "filterExistingEpisodes")
		span.SetAttributes(
			attribute.String("show_name", req.Name),
			attribute.String("tvdb_id", req.Id),
			attribute.Int("total_episodes", len(req.Metadata.Episodes)),
		)

		episodesToDownload := []tvdb.Episode{}
		for _, episode := range req.Metadata.Episodes {
			// First try matching by TVDB ID (most reliable)
			exists, err := q.repo.EpisodeExistsByTvdbId(ctx, req.Id, episode.SeasonNumber, episode.Number)
			if err != nil {
				q.logger.WarnContext(ctx, "Failed to check episode existence by TVDB ID, falling back to title match",
					telemetry.WithTraceContext(ctx, "tvdb_id", req.Id, "season", episode.SeasonNumber, "episode", episode.Number, "error", err.Error())...)
				// Fallback to name-based matching if TVDB ID check fails
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
		span.End()

		if len(episodesToDownload) == 0 {
			q.logger.InfoContext(ctx, "All episodes already exist in Plex, nothing to download", telemetry.WithTraceContext(ctx, "show", req.Name)...)
			return nil
		}

		q.logger.InfoContext(ctx, "Episodes to download after filtering", telemetry.WithTraceContext(ctx, "show", req.Name,
			"total", len(req.Metadata.Episodes), "to_download", len(episodesToDownload), "show_aliases", req.Aliases)...)

		for _, episode := range episodesToDownload {
			q.logger.InfoContext(ctx, "Processing episode", telemetry.WithTraceContext(ctx, "name", req.Name, "season", episode.SeasonNumber,
				"episode", episode.Number)...)

			searchStrategies = append(searchStrategies, q.createSearchStrategy(req, &episode))
		}

		// Use WaitGroup to track all monitoring goroutines
		var wg sync.WaitGroup

		// Create parent span for the entire search operation
		ctx, searchSpan := torrentTracer.Start(ctx, "searchForEpisodes")
		searchSpan.SetAttributes(
			attribute.String("show_name", req.Name),
			attribute.String("tvdb_id", req.Id),
			attribute.Int("episode_count", len(searchStrategies)),
			attribute.Bool("is_anime", req.Anime),
		)
		defer searchSpan.End()

		for episodeIdx, strategies := range searchStrategies {
			bestMatches := []*models.TorrentMatch{}
			for strategyIdx, ss := range strategies {
				// Create span for each search strategy attempt
				strategyCtx, strategySpan := torrentTracer.Start(ctx, "searchStrategy")
				strategySpan.SetAttributes(
					attribute.Int("episode_index", episodeIdx),
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
					return fmt.Errorf("failed to search Prowlarr: %w", err)
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
					// TODO: Store in DB
					continue
				}

				// Track filtering statistics
				totalTorrents := len(pResp)
				rejectionReasons := make(map[string]int)

				possibleTorrents := []*models.TorrentMatch{}
				for _, torrent := range pResp {
					if q.isCorrectTorrent(strategyCtx, torrent, ss) {
						possibleTorrents = append(possibleTorrents, &models.TorrentMatch{Strategy: ss, Torrent: torrent})
					} else {
						// Track rejection reason for statistics
						result := q.validateTorrent(torrent, ss)
						rejectionReasons[result.Reason]++
					}
				}

				validTorrents := len(possibleTorrents)
				rejectedTorrents := totalTorrents - validTorrents

				// Log filtering summary with rejection breakdown
				summaryAttrs := []any{
					"total_torrents", totalTorrents,
					"valid_torrents", validTorrents,
					"rejected_torrents", rejectedTorrents,
					"query", ss.Query,
				}

				// Add rejection reason breakdown if any torrents were rejected
				if rejectedTorrents > 0 {
					for reason, count := range rejectionReasons {
						summaryAttrs = append(summaryAttrs, fmt.Sprintf("rejected_%s", sanitizeReasonKey(reason)), count)
					}
				}

				q.logger.InfoContext(strategyCtx, "Torrent filtering summary", telemetry.WithTraceContext(strategyCtx, summaryAttrs...)...)

				strategySpan.SetAttributes(
					attribute.Int("valid_matches", validTorrents),
					attribute.Int("rejected_matches", rejectedTorrents),
				)

				if len(possibleTorrents) == 0 {
					q.logger.InfoContext(strategyCtx, "no matching torrents found", "query", ss.Query)
					err := q.repo.InsertDownloadHistory(strategyCtx, req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, "", "failure", "no matching torrents found")
					if err != nil {
						q.logger.ErrorContext(strategyCtx, "Failed to insert download history", "error", err)
					}
					strategySpan.SetAttributes(attribute.Bool("success", false))
					strategySpan.SetStatus(codes.Ok, "no matching torrents found")
				} else {
					bestMatches = append(bestMatches, possibleTorrents...)
					strategySpan.SetAttributes(attribute.Bool("success", true))
					strategySpan.SetStatus(codes.Ok, "found matching torrents")
				}
				strategySpan.End()

			}

			q.sortTorrentsByQuality(bestMatches)
			match := q.pickBestTorrent(ctx, bestMatches, req.Category, req.Anime)
			if match == nil {
				q.logger.WarnContext(ctx, "No suitable torrent found", "show", req.Name)
				ss := strategies[0]
				err := q.repo.InsertDownloadHistory(ctx, req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, "", "failure", "no suitable torrent found")
				if err != nil {
					q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
				}
				continue
			}

			ss := match.Strategy
			err := q.repo.InsertDownloadHistory(ctx, req.Name, ss.Season, ss.Episode, ss.EpisodeMeta.AbsoluteNumber, match.Torrent.InfoHash, "downloading", "")
			if err != nil {
				q.logger.ErrorContext(ctx, "Failed to insert download history", "error", err)
			}

			q.downloadTorrent(ctx, match.Torrent)
			// watch torrent to complete
			wg.Add(1)
			go func(ctx context.Context) {
				defer wg.Done()
				ranRecheck := false
				retries := 1
				for {
					time.Sleep(10 * time.Second)
					filter := qbittorrent.TorrentFilterOptions{
						Category: scoutTag,
						Hashes:   []string{match.Torrent.InfoHash},
					}

					// Log the request details before making the API call
					q.logger.InfoContext(ctx, "Fetching torrent status from qBittorrent",
						telemetry.WithTraceContext(ctx,
							"torrent_title", match.Torrent.Title,
							"torrent_hash", match.Torrent.InfoHash,
							"category_filter", scoutTag,
							"attempt", retries)...)

					torrents, err := q.c.GetTorrentsCtx(ctx, filter)
					if err != nil {
						// Enhanced error logging with full context
						q.logger.ErrorContext(ctx, "Error fetching torrents from qBittorrent API",
							telemetry.WithTraceContext(ctx,
								"error", err.Error(),
								"error_type", fmt.Sprintf("%T", err),
								"torrent_title", match.Torrent.Title,
								"torrent_hash", match.Torrent.InfoHash,
								"category_filter", scoutTag,
								"attempt", retries)...)
						retries++
						time.Sleep(6 * time.Second)
						if retries > 6 {
							q.logger.WarnContext(ctx, "Max retries reached, stopping watch", "torrent", match.Torrent.Title)
							return
						}
						continue
					}

					// Log successful response details
					q.logger.InfoContext(ctx, "Successfully fetched torrent status",
						telemetry.WithTraceContext(ctx,
							"torrent_title", match.Torrent.Title,
							"torrent_hash", match.Torrent.InfoHash,
							"torrents_returned", len(torrents))...)
					if len(torrents) == 0 {
						q.logger.InfoContext(ctx, "Torrent not found, continuing watch", "torrent", match.Torrent.Title)
						continue
					}
					torrent := torrents[0]
					if q.didTorrentComplete(&torrent) {

						if !ranRecheck {
							ranRecheck = true
							q.c.RecheckCtx(ctx, []string{torrent.Hash})
							continue
						}

						q.logger.InfoContext(ctx, "Torrent completed", "title", match.Torrent.Title)
						done <- models.TorrentCompleteEvent{
							SavePath: torrent.SavePath,
							Req:      match.Strategy,
							Hash:     match.Torrent.InfoHash,
						}
						break
					} else {
						q.logger.InfoContext(ctx, "Torrent downloading", "title", match.Torrent.Title, "progress", torrent.Progress*100)
					}
				}
			}(ctx)

		}

		// Close the channel after all monitoring goroutines complete
		go func() {
			wg.Wait()
			close(done)
			q.logger.InfoContext(ctx, "All torrents processed, channel closed")
		}()
	}
	return nil
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

func (q *QbittHandler) calcIndexerIDs(_ string, isAnime bool) []int64 {
	if isAnime {
		return []int64{NYAA_ID}
	} else {
		return []int64{NYAA_ID, ONE337x_ID}
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

// validateSeasonalFormatForAnime checks if a seasonal format torrent matches the expected absolute episode
func (q *QbittHandler) validateSeasonalFormatForAnime(parsed *ParsedTorrent, strategy *models.SearchStrategy) bool {
	if strategy.EpisodeMeta == nil {
		// No metadata available, reject for safety
		return false
	}

	// Check if the season and episode numbers from the torrent match the metadata
	// The EpisodeMeta should have both AbsoluteNumber and SeasonNumber/Number
	if strategy.EpisodeMeta.SeasonNumber == parsed.Season &&
		strategy.EpisodeMeta.Number == parsed.Episode &&
		strategy.EpisodeMeta.AbsoluteNumber == strategy.Episode {
		return true
	}

	return false
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

// Helper functions
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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

func (q *QbittHandler) downloadTorrent(ctx context.Context, torrent *prowlarr.Search) error {
	q.logger.InfoContext(ctx, "Downloading torrent", "filename", torrent.FileName, "hash", torrent.InfoHash, "guid", torrent.GUID)

	torrentSavePath := baseSavePath + "/" + torrent.Title
	q.logger.InfoContext(ctx, "Torrent save path", "path", torrentSavePath)

	if err := os.Mkdir(torrentSavePath, 0777); err != nil && !os.IsExist(err) {
		q.logger.ErrorContext(ctx, "Error creating directory", "value", torrentSavePath, "error", err)
		return err
	}

	// Prepare torrent add options using new library
	addOpts := qbittorrent.TorrentAddOptions{
		SavePath: torrentSavePath,
		Category: scoutTag,
	}
	options := addOpts.Prepare()

	// Add torrent using new library's context-aware method
	// Log exactly what we're sending to qBittorrent
	q.logger.Info("Sending to qBittorrent", "magnet_link", torrent.GUID)

	if err := q.c.AddTorrentFromUrlCtx(ctx, torrent.GUID, options); err != nil {
		q.logger.ErrorContext(ctx, "Error downloading torrent", "error", err)
		return err
	}

	return nil
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

	// Get preferred uploaders
	preferred, err := q.repo.GetPreferredUploaders(ctx, mediaType, isAnime)
	if err != nil {
		q.logger.ErrorContext(ctx, "Error querying uploader preferences", "error", err)
	}
	preferred = append(preferred, "SubsPlease", "EMBER", "Erai-raws")

	// Score each match
	scoredMatches := make([]*TorrentMatchWithScore, 0, len(matches))
	for _, match := range matches {
		// Re-validate to get confidence score and parsed info
		validationResult := q.validateTorrent(match.Torrent, match.Strategy)
		if !validationResult.IsValid {
			continue // Skip invalid matches
		}

		// Calculate final score with uploader preference
		finalScore := validationResult.Confidence

		// Boost score for preferred uploaders (50% weight)
		uploaderBoost := 0.0
		for i, uploader := range preferred {
			if strings.Contains(strings.ToLower(match.Torrent.Title), strings.ToLower(uploader)) {
				// Higher boost for earlier uploaders in the list
				uploaderBoost = 0.5 * (1.0 - float64(i)*0.1)
				if uploaderBoost < 0 {
					uploaderBoost = 0.1
				}
				break
			}
		}
		finalScore += uploaderBoost

		// Additional quality boost (25% weight on top of existing quality scoring)
		qualityBoost := 0.0
		switch validationResult.Parsed.GetQualityTier() {
		case 5: // 4K/2160p
			qualityBoost = 0.25
		case 4: // 1080p
			qualityBoost = 0.20
		case 3: // 720p
			qualityBoost = 0.15
		}
		finalScore += qualityBoost

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
