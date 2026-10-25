import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import type { MediaDetails } from '../types/MediaStatus';

/**
 * Fetches the full detail record (overview, genres, runtime, rating, air dates)
 * for a single media item.
 *
 * List endpoints only carry enough data to render a poster grid, so the detail
 * view fetches the rest on demand when a card is opened. Results are cached for
 * an hour — TVDB metadata rarely changes within a session.
 *
 * @param tvdbId - TVDB id of the media item, or null to skip the fetch
 * @param mediaType - "series" or "movie"
 * @param enabled - Whether the query should run (default: true)
 */
export function useMediaDetails(
  tvdbId: string | null,
  mediaType: 'series' | 'movie',
  enabled: boolean = true
): UseQueryResult<MediaDetails, Error> {
  return useQuery<MediaDetails, Error>({
    queryKey: ['media-details', tvdbId, mediaType],
    queryFn: async (): Promise<MediaDetails> => {
      const params = new URLSearchParams({ mediaType });
      const response = await fetch(`/api/media/${tvdbId}/details?${params}`);

      if (!response.ok) {
        throw new Error(`Failed to fetch media details: ${response.statusText}`);
      }

      return response.json();
    },
    staleTime: 60 * 60 * 1000, // 1 hour
    retry: 1,
    enabled: enabled && !!tvdbId,
  });
}
