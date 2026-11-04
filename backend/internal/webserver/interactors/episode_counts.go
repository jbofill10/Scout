package interactors

import (
	"fmt"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// episodeHasAired reports whether a TVDB air date (YYYY-MM-DD) is today or
// earlier. An empty or unparseable date is not aired: TVDB leaves the date
// blank on announced episodes, and the download flow skips those too.
func episodeHasAired(aired string, now time.Time) bool {
	airDate, err := time.Parse("2006-01-02", aired)
	if err != nil {
		return false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return !airDate.After(today)
}

// calculateEpisodeCountsFromMetadata computes the library coverage of a show
// using TVDB metadata as the source of truth: downloaded is how many of
// TVDB's episodes Plex has, aired how many have aired by now, total how many
// TVDB lists. Specials (season 0) are left out of all three. Episodes are
// matched by TVDB id first, then season-episode key, then absolute number. An
// episode the library holds counts as aired whatever its date says.
// Without TVDB metadata it falls back to counting what the library holds,
// which is all aired by definition.
func calculateEpisodeCountsFromMetadata(
	media tvdb.Media,
	showStatus tvdb.ShowStatus,
	now time.Time,
) (downloaded, aired, total int) {
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
		return downloaded, total, total
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
	// Plex uses absolute episode numbering for anime (e.g., S01E25, S02E25, S03E48)
	// while TVDB uses standard per-season numbering (S02E01). The absolute number
	// from TVDB metadata bridges these two schemes.
	downloadedByAbsolute := make(map[int]bool)
	if media.Anime && usesAbsoluteNumbering(showStatus.Seasons) {
		for _, season := range showStatus.Seasons {
			if season.SeasonNum == 0 {
				continue
			}
			for _, ep := range season.Episodes {
				if ep.Downloaded {
					downloadedByAbsolute[ep.EpisodeNum] = true
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
		inLibrary := downloadedByTvdbId[tvdbIdStr] ||
			downloadedByKey[fmt.Sprintf("%d-%d", ep.SeasonNumber, ep.Number)] ||
			(ep.AbsoluteNumber > 0 && downloadedByAbsolute[ep.AbsoluteNumber])
		if inLibrary {
			downloaded++
		}
		// An episode the library holds has aired whatever TVDB's date says, so
		// the count never reads 12 of 11.
		if inLibrary || episodeHasAired(ep.Aired, now) {
			aired++
		}
	}

	return downloaded, aired, total
}

// usesAbsoluteNumbering detects whether Plex is using absolute episode numbering.
// For shows with 2+ non-specials seasons, if any season > 1 has a minimum
// episode number > 1, the show uses absolute numbering (e.g. JJK S2 starts at ep 25).
func usesAbsoluteNumbering(seasons []tvdb.SeasonStatus) bool {
	nonSpecials := 0
	for _, s := range seasons {
		if s.SeasonNum > 0 {
			nonSpecials++
		}
	}
	if nonSpecials < 2 {
		return false
	}
	for _, s := range seasons {
		if s.SeasonNum <= 1 {
			continue
		}
		minEp := 0
		for _, ep := range s.Episodes {
			if minEp == 0 || ep.EpisodeNum < minEp {
				minEp = ep.EpisodeNum
			}
		}
		if minEp > 1 {
			return true
		}
	}
	return false
}
