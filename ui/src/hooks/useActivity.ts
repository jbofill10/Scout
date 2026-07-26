import { useQuery } from "@tanstack/react-query";
import type { DownloadStage } from "../utils/downloadStage";

/**
 * One tracked download: a single episode, or a movie. Stage comes from the
 * notification; attempts/next_attempt_at come from the scheduled downloads
 * table and are only present while an item is still queued or retrying.
 */
export interface ActivityItem {
  id: number;
  tvdb_id: string;
  media_title: string;
  category: "series" | "movie";
  season?: number;
  episode?: number;
  absolute_episode?: number;
  poster_url?: string;
  is_anime: boolean;
  status: DownloadStage;
  reason?: string;
  trace_id?: string;
  created_at: string;
  updated_at: string;
  attempts?: number;
  next_attempt_at?: string;
  release_time?: string;
  schedule_status?: string;
  last_failure_code?: string;
  last_failure_reason?: string;
}

export interface ActivitySnapshot {
  active: ActivityItem[];
  recent: ActivityItem[];
  counts: Partial<Record<DownloadStage, number>>;
}

async function fetchActivity(): Promise<ActivitySnapshot> {
  const response = await fetch("/api/activity");
  if (!response.ok) {
    throw new Error(`Failed to fetch activity: ${response.statusText}`);
  }
  const data = await response.json();
  return {
    active: data.active ?? [],
    recent: data.recent ?? [],
    counts: data.counts ?? {},
  };
}

/**
 * Polls the activity snapshot. Downloads move through stages on the order of
 * seconds, so this refreshes far more eagerly than the rest of the app.
 *
 * @param enabled set false to stop polling while the view is hidden.
 */
export function useActivity(enabled: boolean = true) {
  return useQuery({
    queryKey: ["activity"],
    queryFn: fetchActivity,
    enabled,
    staleTime: 0,
    refetchInterval: enabled ? 5000 : false,
    refetchOnWindowFocus: true,
  });
}

/**
 * Count of in-flight downloads, for the navbar badge. Polls on a slower cadence
 * than the full page since it only drives a number.
 */
export function useActiveDownloadCount() {
  return useQuery({
    queryKey: ["activity"],
    queryFn: fetchActivity,
    staleTime: 0,
    refetchInterval: 15000,
    refetchOnWindowFocus: true,
    select: (data: ActivitySnapshot) => data.active.length,
  });
}
