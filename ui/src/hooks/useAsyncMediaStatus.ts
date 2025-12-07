import { useQuery } from '@tanstack/react-query';
import type { SearchResult } from '../components/SearchResultsList';
import type {
  ExtendedBatchRequest,
  ExtendedMediaData,
  StatusRequest,
  StatusBatchResponse,
} from '../types/MediaStatus';

/**
 * Hook for progressive loading of media status in search results
 *
 * This hook orchestrates two sequential API calls:
 * 1. POST /api/media/batch-extended - Gets full media data with TVDB IDs
 * 2. POST /api/status/batch - Gets download status using TVDB IDs
 *
 * The status call only fires after extended data is successfully fetched,
 * ensuring we have proper TVDB IDs for the status request.
 *
 * @param searchResults - Array of search results from initial search
 * @param mediaType - Type of media being searched ('series' or 'movie')
 * @returns Object with loading states, status data, and error state
 */
export function useAsyncMediaStatus(
  searchResults: SearchResult[],
  mediaType: 'series' | 'movie'
) {
  // Step 1: Fetch extended info to get TVDB IDs
  const {
    data: extendedData,
    isLoading: isLoadingExtended,
    error: extendedError,
  } = useQuery<ExtendedMediaData[], Error>({
    queryKey: ['batch-extended', searchResults.map((r) => r.id), mediaType],
    queryFn: async (): Promise<ExtendedMediaData[]> => {
      if (searchResults.length === 0) {
        return [];
      }

      const requests: ExtendedBatchRequest[] = searchResults.map((r) => ({
        id: r.id,
        mediaType,
      }));

      const response = await fetch('/api/media/batch-extended', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requests),
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch extended media info: ${response.statusText}`);
      }

      const result = await response.json();
      // API returns { data: ExtendedMediaData[] }
      return result.data || [];
    },
    enabled: searchResults.length > 0,
    staleTime: 5 * 60 * 1000, // 5 minutes - extended info changes rarely
    retry: 2,
  });

  // Step 2: Fetch status (depends on extended data)
  const {
    data: statusData,
    isLoading: isLoadingStatus,
    error: statusError,
  } = useQuery<StatusBatchResponse, Error>({
    queryKey: ['media-status-batch', extendedData, mediaType],
    queryFn: async (): Promise<StatusBatchResponse> => {
      if (!extendedData || extendedData.length === 0) {
        return { shows: [], movies: [] };
      }

      const requests: StatusRequest[] = extendedData.map((media) => ({
        tvdbId: String(media.id),
        mediaType: mediaType === 'series' ? 'show' : 'movie',
      }));

      const response = await fetch('/api/status/batch', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requests),
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch media status: ${response.statusText}`);
      }

      return response.json();
    },
    enabled: !!extendedData && extendedData.length > 0,
    staleTime: 2 * 60 * 1000, // 2 minutes - status can change more frequently
    retry: 2,
  });

  return {
    isLoadingExtended,
    isLoadingStatus,
    statusData,
    error: extendedError || statusError,
    isLoading: isLoadingExtended || isLoadingStatus,
  };
}
