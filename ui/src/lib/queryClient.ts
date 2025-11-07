import { QueryClient } from '@tanstack/react-query';

/**
 * TanStack Query Client Configuration
 *
 * Configured for Scout's Netflix-style UI with optimized caching
 * for media search and genre data.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Data is considered fresh for 5 minutes
      staleTime: 5 * 60 * 1000,

      // Cache data for 10 minutes before garbage collection
      gcTime: 10 * 60 * 1000,

      // Retry failed requests once
      retry: 1,

      // Don't refetch on window focus by default (can be overridden per query)
      refetchOnWindowFocus: false,

      // Don't refetch on reconnect by default
      refetchOnReconnect: false,
    },
    mutations: {
      // Retry mutations once on failure
      retry: 1,
    },
  },
});
