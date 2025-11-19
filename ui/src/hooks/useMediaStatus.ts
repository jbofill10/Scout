import { useQuery } from '@tanstack/react-query';
import type { UseQueryResult } from '@tanstack/react-query';
import type { StatusRequest, StatusBatchResponse } from '../types/MediaStatus';

/**
 * Fetches download status for multiple media items in a batch
 *
 * @param requests - Array of media items to fetch status for
 * @param enabled - Whether the query should run (default: true)
 * @returns TanStack Query result with StatusBatchResponse
 */
export function useMediaStatus(
  requests: StatusRequest[],
  enabled: boolean = true
): UseQueryResult<StatusBatchResponse, Error> {
  return useQuery<StatusBatchResponse, Error>({
    queryKey: ['media-status', requests],
    queryFn: async (): Promise<StatusBatchResponse> => {
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
    staleTime: 2 * 60 * 1000, // 2 minutes
    enabled: enabled && requests.length > 0,
  });
}
