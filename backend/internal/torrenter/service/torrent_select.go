package service

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
)

// defaultMinQuality is the resolution floor applied when MIN_QUALITY is unset.
// 720p releases are the best-seeded listings on public indexers, so without a
// floor the seeder-aware picker drifts down to them for anything with a thin
// 1080p swarm.
const defaultMinQuality = "1080p"

// minQualityTier reads the resolution floor from MIN_QUALITY (480p, 720p,
// 1080p, 2160p or 4k). An unrecognised value falls back to the default rather
// than silently disabling the floor.
func minQualityTier() int {
	label := os.Getenv("MIN_QUALITY")
	if tier := QualityTier(label); label != "" && tier > 1 {
		return tier
	}
	return QualityTier(defaultMinQuality)
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

// seederBoost is the swarm-health term of a torrent's final score. Above a
// handful of seeders it grows with the log of the reported count, so a few
// more barely register but an order of magnitude does, capped just above the
// 1080p quality boost (0.6): a 720p release with 100+ seeders outscores a
// 1080p one with six, while a 1080p release with 16 or more still wins.
//
// The low end is deliberately harsh. Indexers report stale counts, and on
// 1337x a listing showing 2-5 seeders routinely turns out to have none: those
// picks sat in qBittorrent at 0% until the monitor gave up. So five or fewer
// is a penalty that outweighs one resolution tier, and zero is treated as
// dead, chosen only when nothing else passed validation.
//
//	seeders:  0     1      3      5    6     15   31   63   127+
//	boost:   -1.0  -0.32  -0.16   0.0  0.28  0.4  0.5  0.6  0.7
func seederBoost(seeders int) float64 {
	switch {
	case seeders <= 0:
		return -1.0
	case seeders <= suspectSeederCount:
		return -0.4 + 0.08*float64(seeders)
	}
	boost := 0.1 * math.Log2(float64(1+seeders))
	if boost > maxSeederBoost {
		return maxSeederBoost
	}
	return boost
}

const (
	// suspectSeederCount is the reported seeder count at or below which a
	// listing is assumed to be effectively dead.
	suspectSeederCount = 5
	maxSeederBoost     = 0.7
)

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
	belowFloor := 0
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

		// Resolution floor: anything below it, including a title whose
		// resolution could not be parsed, is not an acceptable copy no matter
		// how well seeded. Rejecting here (rather than scoring low) means an
		// indexer with only sub-floor releases yields no match, so the search
		// moves on to the fallback indexer tier instead of settling.
		if q.minQualityTier > 0 && qualityTier < q.minQualityTier {
			belowFloor++
			q.logger.WarnContext(ctx, "Torrent below minimum quality, skipping",
				"torrent_title", match.Torrent.Title,
				"quality", quality,
				"quality_tier", qualityTier,
				"min_quality_tier", q.minQualityTier)
			continue
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

		// Swarm health. Without this a 1080p listing with two seeders beat a
		// 720p one with two hundred, and the pick sat in qBittorrent at 0%.
		seedBoost := seederBoost(match.Torrent.Seeders)
		finalScore += seedBoost

		q.logger.InfoContext(ctx, "Calculated final score",
			"torrent_title", match.Torrent.Title,
			"final_score", finalScore,
			"uploader_boost", uploaderBoost,
			"matched_uploader", matchedUploader,
			"seeders", match.Torrent.Seeders,
			"seeder_boost", seedBoost)

		scoredMatches = append(scoredMatches, &TorrentMatchWithScore{
			TorrentMatch:     match,
			ValidationResult: validationResult,
			FinalScore:       finalScore,
		})
	}

	q.logger.InfoContext(ctx, "Torrent scoring complete",
		"total_evaluated", len(matches),
		"total_scored", len(scoredMatches),
		"total_rejected", len(matches)-len(scoredMatches),
		"below_quality_floor", belowFloor)

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
