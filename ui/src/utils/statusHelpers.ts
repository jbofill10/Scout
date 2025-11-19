import type {
  StatusBatchResponse,
  SeasonStatus,
  MediaStatusBadge,
} from '../types/MediaStatus';

/**
 * Calculates the total number of downloaded and total episodes across all seasons
 *
 * @param seasons - Array of season status objects
 * @returns Object with downloaded and total episode counts
 */
export function calculateEpisodeCount(
  seasons: SeasonStatus[]
): { downloaded: number; total: number } {
  let downloaded = 0;
  let total = 0;

  for (const season of seasons) {
    total += season.episodes.length;
    downloaded += season.episodes.filter((ep) => ep.downloaded).length;
  }

  return { downloaded, total };
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
    if (episodeCount.total > 0) {
      return {
        type: 'show',
        episodeCount,
      };
    }

    return undefined;
  }

  return undefined;
}
