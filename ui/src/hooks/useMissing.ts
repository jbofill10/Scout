import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../contexts/ToastContext";
import { logError } from "../lib/logger";
import { describeSummary, type DownloadSummary } from "./useDownloadRequest";
import type { EpisodeWithStatus } from "./useLibrary";

/** An aired episode a library show lacks; requested when Scout already has it in hand. */
export interface MissingEpisode extends EpisodeWithStatus {
  requested: boolean;
}

export interface MissingShow {
  tvdbId: string;
  title: string;
  thumb: string;
  episodes: MissingEpisode[];
}

export const MISSING_QUERY_KEY = ["library", "missing"] as const;

async function fetchMissing(): Promise<MissingShow[]> {
  const response = await fetch("/api/library/missing");
  if (!response.ok) {
    throw new Error(`Failed to fetch missing episodes: ${response.statusText}`);
  }
  const data = await response.json();
  return Array.isArray(data) ? data : [];
}

/** Every library show with gaps. */
export function useMissingEpisodes(enabled: boolean = true) {
  return useQuery({
    queryKey: MISSING_QUERY_KEY,
    queryFn: fetchMissing,
    enabled,
    staleTime: 60_000,
  });
}

/** How many of a show's missing episodes still need requesting. */
export function unrequestedCount(show: MissingShow): number {
  return show.episodes.filter((ep) => !ep.requested).length;
}

/** Requests a show's missing episodes; the server skips ones already requested. */
export function useFillMissing() {
  const queryClient = useQueryClient();
  const showToast = useToast();

  return useMutation<DownloadSummary, Error, { tvdbId: string; title: string }>({
    mutationFn: async ({ tvdbId }) => {
      const response = await fetch(`/api/library/shows/${encodeURIComponent(tvdbId)}/fill-missing`, { method: "POST" });
      if (!response.ok) {
        let message = response.statusText;
        try {
          const data = await response.json();
          if (data && typeof data.error === "string") message = data.error;
        } catch {
          // keep the status text
        }
        throw new Error(message);
      }
      return response.json();
    },
    retry: 0,
    onSuccess: (summary, vars) => {
      const hasWork = summary.queued_now > 0 || summary.scheduled > 0;
      showToast(describeSummary(summary, vars.title), { severity: hasWork ? "success" : "info", linkToActivity: hasWork });
      queryClient.invalidateQueries({ queryKey: MISSING_QUERY_KEY });
      queryClient.invalidateQueries({ queryKey: ["activity"] });
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
      queryClient.invalidateQueries({ queryKey: ["schedule"] });
    },
    onError: (error, vars) => {
      logError("Fill missing failed", error, { component: "useFillMissing", tvdbId: vars.tvdbId });
      showToast(error.message || `Couldn't request the missing episodes of ${vars.title}.`, { severity: "error" });
    },
  });
}
