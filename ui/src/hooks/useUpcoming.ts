import { useQuery } from "@tanstack/react-query";

/**
 * One pending scheduled row: an episode or movie Scout will act on later,
 * either when it releases or at its next retry.
 */
export interface UpcomingItem {
  id: number;
  /** The item key every download control takes. */
  tvdb_id: string;
  media_title: string;
  category: "series" | "movie";
  season: number;
  episode: number;
  absolute_episode?: number;
  poster_url?: string;
  is_anime: boolean;
  release_time: string;
  next_attempt_at?: string;
  /** Next retry when one is set, otherwise the release time. */
  due_at: string;
  attempts: number;
  last_failure_code?: string;
  last_failure_reason?: string;
}

export const UPCOMING_QUERY_KEY = ["schedule", "upcoming"] as const;

async function fetchUpcoming(): Promise<UpcomingItem[]> {
  const response = await fetch("/api/schedule/upcoming");
  if (!response.ok) {
    throw new Error(`Failed to fetch upcoming downloads: ${response.statusText}`);
  }
  const data = await response.json();
  return Array.isArray(data) ? data : [];
}

/** Every pending scheduled row, soonest first. Refreshes once a minute. */
export function useUpcoming(enabled: boolean = true) {
  return useQuery({
    queryKey: UPCOMING_QUERY_KEY,
    queryFn: fetchUpcoming,
    enabled,
    staleTime: 30_000,
    refetchInterval: enabled ? 60_000 : false,
    refetchOnWindowFocus: true,
  });
}
