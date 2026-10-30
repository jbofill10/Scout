import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../contexts/ToastContext";
import { logError } from "../lib/logger";

/** A show Scout re-checks for new episodes on its own. */
export interface FollowedShow {
  tvdb_id: string;
  title: string;
  poster_url?: string;
  followed_at: string;
  include_missing: boolean;
  last_checked_at?: string;
  last_check_error?: string;
  episodes_added: number;
}

/** What one check did for one show. */
export interface FollowCheckResult {
  tvdb_id: string;
  title: string;
  listed: number;
  new: number;
  queued_now: number;
  scheduled: number;
  error?: string;
  checked_at: string;
}

export interface FollowCheckReport {
  checked: number;
  added: number;
  results: FollowCheckResult[];
}

export interface FollowResponse {
  show: FollowedShow;
  check: FollowCheckResult;
}

/** The slice of a media record the follow needs; the server fills in the rest. */
export interface FollowMedia {
  id: string;
  mediaName: string;
  image_url?: string;
}

export interface FollowVars {
  media: FollowMedia;
  includeMissing: boolean;
}

export const FOLLOWS_QUERY_KEY = ["follows"] as const;

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init);
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
  if (response.status === 204) return undefined as T;
  return response.json();
}

export function useFollows(enabled: boolean = true) {
  return useQuery({
    queryKey: FOLLOWS_QUERY_KEY,
    queryFn: () => request<FollowedShow[]>("/api/follows"),
    enabled,
    staleTime: 60_000,
  });
}

/** Wording for what a check found. */
export function describeCheck(result: FollowCheckResult): string {
  if (result.error) return `Check failed: ${result.error}`;
  if (result.new === 0) return "Nothing new to request";
  const parts: string[] = [];
  if (result.queued_now > 0) parts.push(`searching for ${result.queued_now} now`);
  if (result.scheduled > 0) parts.push(`${result.scheduled} scheduled`);
  return parts.length > 0 ? parts.join(", ") : `${result.new} new`;
}

export function useFollowShow() {
  const queryClient = useQueryClient();
  const showToast = useToast();

  return useMutation<FollowResponse, Error, FollowVars>({
    mutationFn: ({ media, includeMissing }) =>
      request<FollowResponse>("/api/follows", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          media: { id: media.id, mediaName: media.mediaName, image_url: media.image_url ?? "", type: "series" },
          include_missing: includeMissing,
        }),
      }),
    retry: 0,
    onSuccess: (resp) => {
      queryClient.invalidateQueries({ queryKey: FOLLOWS_QUERY_KEY });
      queryClient.invalidateQueries({ queryKey: ["activity"] });
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
      queryClient.invalidateQueries({ queryKey: ["schedule"] });
      const hasWork = resp.check.new > 0;
      showToast(`Following ${resp.show.title}: ${describeCheck(resp.check)}`, {
        severity: resp.check.error ? "warning" : "success",
        linkToActivity: hasWork,
      });
    },
    onError: (error, vars) => {
      logError("Follow failed", error, { component: "useFollowShow", tvdbId: vars.media.id });
      showToast(error.message || `Couldn't follow ${vars.media.mediaName}.`, { severity: "error" });
    },
  });
}

export function useUnfollowShow() {
  const queryClient = useQueryClient();
  const showToast = useToast();

  return useMutation<void, Error, { tvdbId: string; title: string }>({
    mutationFn: ({ tvdbId }) => request<void>(`/api/follows/${encodeURIComponent(tvdbId)}`, { method: "DELETE" }),
    retry: 0,
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: FOLLOWS_QUERY_KEY });
      showToast(`Unfollowed ${vars.title}`, { severity: "info" });
    },
    onError: (error, vars) => {
      logError("Unfollow failed", error, { component: "useUnfollowShow", tvdbId: vars.tvdbId });
      showToast(error.message || `Couldn't unfollow ${vars.title}.`, { severity: "error" });
      queryClient.invalidateQueries({ queryKey: FOLLOWS_QUERY_KEY });
    },
  });
}

/** Re-check one followed show (tvdbId) or all of them (null). */
export function useCheckFollows() {
  const queryClient = useQueryClient();
  const showToast = useToast();

  return useMutation<FollowCheckReport | FollowCheckResult, Error, { tvdbId: string | null }>({
    mutationFn: ({ tvdbId }) =>
      request(tvdbId ? `/api/follows/${encodeURIComponent(tvdbId)}/check` : "/api/follows/check", { method: "POST" }),
    retry: 0,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: FOLLOWS_QUERY_KEY });
      queryClient.invalidateQueries({ queryKey: ["activity"] });
      queryClient.invalidateQueries({ queryKey: ["schedule"] });
      if ("results" in data) {
        showToast(
          data.checked === 0
            ? "No followed shows to check"
            : `Checked ${data.checked} show${data.checked === 1 ? "" : "s"}: ${data.added} new episode${data.added === 1 ? "" : "s"}`,
          { severity: "success", linkToActivity: data.added > 0 },
        );
      } else {
        showToast(`${data.title}: ${describeCheck(data)}`, {
          severity: data.error ? "warning" : "success",
          linkToActivity: data.new > 0,
        });
      }
    },
    onError: (error) => {
      logError("Follow check failed", error, { component: "useCheckFollows" });
      showToast(error.message || "Couldn't check followed shows.", { severity: "error" });
    },
  });
}
