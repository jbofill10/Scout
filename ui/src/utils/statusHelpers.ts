import type {
  StatusBatchResponse,
  SeasonStatus,
  MediaStatusBadge,
  MediaStatusInfo,
  EnrichedMedia,
  EpisodeCoverage,
} from '../types/MediaStatus';

/**
 * Sums library coverage across all seasons of a batch status response. Batch
 * status comes straight from the library database and carries no air dates,
 * so every episode it lists has aired and nothing is upcoming.
 *
 * @param seasons - Array of season status objects
 * @returns Downloaded and aired episode counts
 */
export function calculateEpisodeCount(seasons: SeasonStatus[]): EpisodeCoverage {
  let downloaded = 0;
  let aired = 0;

  for (const season of seasons) {
    aired += season.episodes.length;
    downloaded += season.episodes.filter((ep) => ep.downloaded).length;
  }

  return { downloaded, aired, upcoming: 0 };
}

/**
 * Gets the status badge for a specific media item from the batch response
 *
 * @param tvdbId - TVDB ID of the media item
 * @param statusData - Status batch response from the API
 * @param mediaType - Type of media ('show' or 'movie')
 * @returns MediaStatusBadge object or undefined if no status found
 */
export function getStatusBadgeForMedia(
  tvdbId: string,
  statusData: StatusBatchResponse | undefined,
  mediaType: 'show' | 'movie'
): MediaStatusBadge | undefined {
  if (!statusData) {
    return undefined;
  }

  if (mediaType === 'movie') {
    const movieStatus = statusData.movies.find((m) => m.tvdbId === tvdbId);
    if (!movieStatus) {
      return undefined;
    }

    // Only show badge if movie is in library
    if (movieStatus.inLibrary) {
      return {
        type: 'movie',
        inLibrary: true,
      };
    }

    return undefined;
  }

  if (mediaType === 'show') {
    const showStatus = statusData.shows.find((s) => s.tvdbId === tvdbId);
    if (!showStatus || !showStatus.seasons || showStatus.seasons.length === 0) {
      return undefined;
    }

    const episodeCount = calculateEpisodeCount(showStatus.seasons);

    // Only show badge if there are episodes
    if (episodeCount.aired > 0) {
      return {
        type: 'show',
        episodeCount,
      };
    }

    return undefined;
  }

  return undefined;
}

/**
 * Builds a MediaStatusBadge from enriched search MediaStatusInfo
 * Used to convert backend status format to UI badge format
 *
 * @param statusInfo - Status info from enriched search response
 * @returns MediaStatusBadge object for display in MediaCard
 */
export function buildStatusBadgeFromEnriched(
  statusInfo: MediaStatusInfo
): MediaStatusBadge | undefined {
  if (statusInfo.type === 'movie') {
    // Only show badge if movie is in library
    if (statusInfo.inLibrary) {
      return {
        type: 'movie',
        inLibrary: true,
      };
    }
    return undefined;
  }

  if (statusInfo.type === 'series') {
    const downloaded = statusInfo.downloaded || 0;
    const total = statusInfo.total || 0;
    // A backend without the aired count reports everything as aired; an
    // episode in the library has aired whatever TVDB's date says.
    const aired = Math.max(statusInfo.aired ?? total, downloaded);
    const upcoming = Math.max(total - aired, 0);

    // Only show badge once something has aired
    if (aired > 0) {
      return {
        type: 'show',
        episodeCount: { downloaded, aired, upcoming },
      };
    }
    return undefined;
  }

  return undefined;
}

/**
 * Gets the status badge preferring enriched data (accurate TVDB-based counts)
 * over batch status data (DB-only counts that may always show 100%).
 *
 * @param tvdbId - TVDB ID of the media item
 * @param statusData - Status batch response from the API
 * @param mediaType - Type of media ('show' or 'movie')
 * @param enrichedMedia - Optional enriched media data with accurate counts
 * @returns MediaStatusBadge object or undefined if no status found
 */
export function getStatusBadgeForMediaWithEnriched(
  tvdbId: string,
  statusData: StatusBatchResponse | undefined,
  mediaType: 'show' | 'movie',
  enrichedMedia?: EnrichedMedia | null
): MediaStatusBadge | undefined {
  // Prefer enriched data if available (has accurate TVDB-based counts)
  if (enrichedMedia?.status) {
    const badge = buildStatusBadgeFromEnriched(enrichedMedia.status);
    if (badge) return badge;
  }

  // Fall back to batch status
  return getStatusBadgeForMedia(tvdbId, statusData, mediaType);
}
