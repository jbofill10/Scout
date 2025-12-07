import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import type { EnrichedMedia } from '../types/MediaStatus';

/**
 * Enriched media response from popular endpoints
 * Matches backend EnrichedMedia structure
 */
export type EnrichedPopularResponse = EnrichedMedia[];

/**
 * Fetches enriched popular content (with episode metadata, extended info, and download status)
 * Uses progressive enhancement pattern: call this after initial basic content load
 *
 * @param genre - Genre to filter by (e.g., "Action", "Drama")
 * @param limit - Maximum number of items to return
 * @param mediaType - Type of media ("series" or "movie")
 * @param enabled - Whether the query should run (default: true)
 * @returns TanStack Query result with EnrichedMedia array
 */
export function useEnrichedPopularShows(
  genre: string,
  limit: number,
  enabled: boolean = true
): UseQueryResult<EnrichedPopularResponse, Error> {
  return useQuery<EnrichedPopularResponse, Error>({
    queryKey: ['popular-shows-enriched', genre, limit],
    queryFn: async (): Promise<EnrichedPopularResponse> => {
      const params = new URLSearchParams({
        limit: limit.toString(),
      });

      if (genre) {
        params.append('genre', genre);
      }

      const response = await fetch(`/api/popular/shows/enriched?${params}`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch enriched popular shows: ${response.statusText}`);
      }

      return response.json();
    },
    staleTime: 5 * 60 * 1000, // 5 minutes (same as basic popular content)
    enabled: enabled,
  });
}

/**
 * Fetches enriched popular movies (with extended info and download status)
 *
 * @param genre - Genre to filter by (e.g., "Action", "Drama")
 * @param limit - Maximum number of items to return
 * @param enabled - Whether the query should run (default: true)
 * @returns TanStack Query result with EnrichedMedia array
 */
export function useEnrichedPopularMovies(
  genre: string,
  limit: number,
  enabled: boolean = true
): UseQueryResult<EnrichedPopularResponse, Error> {
  return useQuery<EnrichedPopularResponse, Error>({
    queryKey: ['popular-movies-enriched', genre, limit],
    queryFn: async (): Promise<EnrichedPopularResponse> => {
      const params = new URLSearchParams({
        limit: limit.toString(),
      });

      if (genre) {
        params.append('genre', genre);
      }

      const response = await fetch(`/api/popular/movies/enriched?${params}`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch enriched popular movies: ${response.statusText}`);
      }

      return response.json();
    },
    staleTime: 5 * 60 * 1000, // 5 minutes (same as basic popular content)
    enabled: enabled,
  });
}

/**
 * Generic hook for enriched popular content (auto-selects show/movie endpoint)
 *
 * @param genre - Genre to filter by
 * @param limit - Maximum number of items to return
 * @param mediaType - Type of media ("series" or "movie")
 * @param enabled - Whether the query should run (default: true)
 * @returns TanStack Query result with EnrichedMedia array
 */
export function useEnrichedPopular(
  genre: string,
  limit: number,
  mediaType: 'series' | 'movie',
  enabled: boolean = true
): UseQueryResult<EnrichedPopularResponse, Error> {
  // Call both hooks unconditionally to satisfy Rules of Hooks
  const showsQuery = useEnrichedPopularShows(genre, limit, enabled && mediaType === 'series');
  const moviesQuery = useEnrichedPopularMovies(genre, limit, enabled && mediaType === 'movie');

  // Return the appropriate result based on mediaType
  return mediaType === 'series' ? showsQuery : moviesQuery;
}
