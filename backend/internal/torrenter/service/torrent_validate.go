package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"golift.io/starr/prowlarr"
)

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
