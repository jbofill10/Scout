package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
)

// matchVerdict is scoreMatch's answer for one validated match: a score, or the
// reason the selection rules exclude it.
type matchVerdict struct {
	score      float64
	quality    string
	rejection  string
	belowFloor bool
}

// preferredUploaders is the uploader list the picker favours: the configured
// preferences plus the groups that reliably release for the media type.
func (q *QbittHandler) preferredUploaders(ctx context.Context, mediaType string, isAnime bool) []string {
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
	return preferred
}

// scoreMatch applies the selection rules to one match that already passed
// validation and returns its final score, or a non-empty rejection when the
// relax-level confidence cutoff or the quality floor excludes it. pickBestTorrent
// and the release listing share it so the two never disagree about a torrent.
func (q *QbittHandler) scoreMatch(
	ctx context.Context,
	match *models.TorrentMatch,
	validationResult TorrentValidationResult,
	preferred []string,
) matchVerdict {
	// Relaxation guardrail: broad (RelaxLevel >= 2) queries cast a wide net, so
	// require a higher minimum confidence before accepting their matches. The
	// validators stay authoritative; this is an extra confidence gate.
	minConfidence := relaxConfidenceCutoff(match.Strategy)
	if validationResult.Confidence < minConfidence {
		q.logger.WarnContext(ctx, "Torrent below relax-level confidence cutoff, skipping",
			"torrent_title", match.Torrent.Title,
			"confidence", validationResult.Confidence,
			"min_confidence", minConfidence,
			"relax_level", relaxLevelOf(match.Strategy))
		return matchVerdict{rejection: fmt.Sprintf("match confidence %.2f below the %.2f needed for a relaxed query",
			validationResult.Confidence, minConfidence)}
	}

	// Calculate final score with quality as highest priority
	finalScore := validationResult.Confidence

	// Quality boost (highest priority) - year-aware quality selection
	// Recognizes that 4K didn't exist before 2012, prefers native HD for older content
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
	floor := q.qualityFloor()
	if floor > 0 && qualityTier < floor {
		q.logger.WarnContext(ctx, "Torrent below minimum quality, skipping",
			"torrent_title", match.Torrent.Title,
			"quality", quality,
			"quality_tier", qualityTier,
			"min_quality_tier", floor)
		shown := quality
		if shown == "" {
			shown = "unknown resolution"
		}
		return matchVerdict{quality: quality, belowFloor: true,
			rejection: fmt.Sprintf("%s is below the minimum quality", shown)}
	}

	// Calculate year-aware quality boost
	releaseYear := ""
	if match.Strategy != nil {
		releaseYear = match.Strategy.ReleaseYear
	}
	qualityBoost := calculateYearAwareQualityBoost(qualityTier, releaseYear)
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

	return matchVerdict{score: finalScore, quality: quality}
}
