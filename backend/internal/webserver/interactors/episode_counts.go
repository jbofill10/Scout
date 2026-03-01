package interactors

import (
	"fmt"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// calculateEpisodeCountsFromMetadata computes downloaded/total episode counts using TVDB metadata
// as the source of truth for total count, and matches downloaded episodes by TVDB ID (primary)
// or season-episode key (fallback). Season 0 (specials) is excluded from counts.
// Falls back to DB-only counting if no TVDB metadata is available.
func calculateEpisodeCountsFromMetadata(media tvdb.Media, showStatus tvdb.ShowStatus) (downloaded, total int) {
	episodes := media.Metadata.Episodes
	if len(episodes) == 0 {
		// No TVDB metadata available, fall back to DB-only counting
		for _, season := range showStatus.Seasons {
			for _, ep := range season.Episodes {
				total++
				if ep.Downloaded {
					downloaded++
				}
			}
		}
		return downloaded, total
	}

	// Build lookup sets from status data (DB episodes)
	downloadedByTvdbId := make(map[string]bool)
	downloadedByKey := make(map[string]bool)
	for _, season := range showStatus.Seasons {
		for _, ep := range season.Episodes {
			if ep.Downloaded {
				if ep.TvdbId != "" {
					downloadedByTvdbId[ep.TvdbId] = true
				}
				key := fmt.Sprintf("%d-%d", season.SeasonNum, ep.EpisodeNum)
				downloadedByKey[key] = true
			}
		}
	}

	// Build absolute number lookup for anime shows.
	// Anime in Plex stores all episodes under Season 1 with absolute numbering
	// (e.g., S01E25) while TVDB uses standard seasonal numbering (S02E01).
	// The absolute number bridges these two schemes.
	downloadedByAbsolute := make(map[int]bool)
	if media.Anime && hasOnlySeason1(showStatus.Seasons) {
		for _, season := range showStatus.Seasons {
			if season.SeasonNum == 1 {
				for _, ep := range season.Episodes {
					if ep.Downloaded {
						downloadedByAbsolute[ep.EpisodeNum] = true
					}
				}
			}
		}
	}

	// Count using TVDB metadata as source of truth
	for _, ep := range episodes {
		if ep.SeasonNumber == 0 {
			continue // Skip specials
		}
		total++

		// Match by TVDB ID first, then season-episode key, then absolute number
		tvdbIdStr := fmt.Sprintf("%d", ep.Id)
		if downloadedByTvdbId[tvdbIdStr] {
			downloaded++
		} else if downloadedByKey[fmt.Sprintf("%d-%d", ep.SeasonNumber, ep.Number)] {
			downloaded++
		} else if ep.AbsoluteNumber > 0 && downloadedByAbsolute[ep.AbsoluteNumber] {
			downloaded++
		}
	}

	return downloaded, total
}

// hasOnlySeason1 returns true when the only non-specials season is Season 1.
// This is the characteristic pattern of Plex storing anime with absolute numbering.
func hasOnlySeason1(seasons []tvdb.SeasonStatus) bool {
	for _, s := range seasons {
		if s.SeasonNum != 0 && s.SeasonNum != 1 {
			return false
		}
	}
	return true
}
