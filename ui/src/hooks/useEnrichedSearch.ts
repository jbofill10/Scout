import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { EnrichedMedia } from "../types/MediaStatus";

export type SearchMediaType = "series" | "movie";

/**
 * Fetches enriched search results (media + library status) from the webserver.
 * The signal lets TanStack Query abort a request the user has already typed
 * past, which frees the backend to work on the current query instead.
 */
export async function fetchEnrichedSearch(
  query: string,
  mediaType: SearchMediaType,
  signal?: AbortSignal,
): Promise<EnrichedMedia[]> {
  const params = new URLSearchParams({ query, media_type: mediaType });
  const response = await fetch(`/api/search/enriched?${params}`, { signal });
  if (!response.ok) {
    throw new Error(`Search failed: ${response.status} ${response.statusText}`);
  }
  return response.json();
}

/**
 * useEnrichedSearch
 *
 * One query per (media type, normalised term). Compared with a hand-rolled
 * fetch-in-effect this gives us:
 * - no stale-response races: a slow response for an older term never
 *   overwrites the results for the current one
 * - cancellation of abandoned requests
 * - a cache, so retyping or toggling back to a recent term is instant
 * - keepPreviousData, so refining a term does not flash an empty grid
 */
export function useEnrichedSearch(query: string, mediaType: SearchMediaType) {
  const term = query.trim();
  const enabled = term.length > 0;

  const result = useQuery<EnrichedMedia[], Error>({
    queryKey: ["search", mediaType, term.toLowerCase()],
    queryFn: ({ signal }) => fetchEnrichedSearch(term, mediaType, signal),
    enabled,
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  });

  return {
    /** Empty while there is no term, even if an older result is cached. */
    results: enabled ? (result.data ?? []) : [],
    /** True whenever a request for the current key is in flight. */
    isFetching: enabled && result.isFetching,
    /** True while the grid is showing results from a previous term. */
    isPlaceholderData: enabled && result.isPlaceholderData,
    error: enabled ? result.error : null,
  };
}
