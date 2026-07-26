import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../contexts/ToastContext";
import { logError } from "../lib/logger";

/**
 * What the webserver reports back from POST /api/shows and /api/movies.
 *
 * The torrent search itself runs in the background, so these counts describe
 * what was accepted, not what finished: queued_now items are being searched
 * for right away, scheduled items are waiting on a release date, and skipped
 * items were specials, duplicates, or had no usable air date.
 */
export interface DownloadSummary {
  media_title: string;
  category: "series" | "movie";
  queued_now: number;
  scheduled: number;
  skipped: number;
  next_release?: string;
  trace_id?: string;
}

export interface DownloadRequestVars {
  /** The media payload the endpoint expects (enriched media when available). */
  media: unknown;
  mediaType: "series" | "movie";
  /** Used for logging and as a fallback title in the confirmation message. */
  title?: string;
  /** Where the request came from, for error logs. */
  component?: string;
}

function plural(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

/**
 * Turns a summary into the sentence shown to the user. This is the only
 * feedback for an action whose real work happens minutes later, so it names
 * what was accepted and when the rest lands.
 */
export function describeSummary(summary: DownloadSummary, fallbackTitle?: string): string {
  const title = summary.media_title || fallbackTitle || "Media";
  const isMovie = summary.category === "movie";
  const unit = isMovie ? "movie" : "episode";
  const parts: string[] = [];

  if (summary.queued_now > 0) {
    parts.push(
      isMovie
        ? "searching for a torrent now"
        : `searching for ${plural(summary.queued_now, unit)} now`,
    );
  }

  if (summary.scheduled > 0) {
    const when = summary.next_release
      ? ` — first on ${new Date(summary.next_release).toLocaleDateString()}`
      : "";
    parts.push(`${plural(summary.scheduled, unit)} scheduled for release${when}`);
  }

  if (parts.length === 0) {
    if (summary.skipped > 0) {
      return `${title}: already tracked — nothing new to download`;
    }
    return `${title}: nothing to download`;
  }

  return `${title}: ${parts.join(", ")}`;
}

async function postDownload({ media, mediaType }: DownloadRequestVars): Promise<DownloadSummary> {
  const endpoint = mediaType === "movie" ? "/api/movies" : "/api/shows";

  const response = await fetch(endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(media),
  });

  if (!response.ok) {
    throw new Error(`Download request failed: ${response.status} ${response.statusText}`);
  }

  return response.json();
}

/**
 * useDownloadRequest
 *
 * The single entry point for "download this" across the app. It reports the
 * outcome through the app toast, refreshes the activity and notification views
 * so the new work shows up immediately, and exposes isPending so the button
 * that triggered it can show progress.
 */
export function useDownloadRequest() {
  const showToast = useToast();
  const queryClient = useQueryClient();

  const mutation = useMutation<DownloadSummary, Error, DownloadRequestVars>({
    mutationFn: postDownload,
    // A failed request should surface immediately rather than after a retry.
    retry: 0,
    onSuccess: (summary, vars) => {
      const hasWork = summary.queued_now > 0 || summary.scheduled > 0;
      showToast(describeSummary(summary, vars.title), {
        severity: hasWork ? "success" : "info",
        linkToActivity: hasWork,
      });

      // The new work is already visible on the server; pull it in.
      queryClient.invalidateQueries({ queryKey: ["activity"] });
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
      queryClient.invalidateQueries({ queryKey: ["schedule", "weekly"] });
    },
    onError: (error, vars) => {
      logError("Download request failed", error, {
        component: vars.component ?? "unknown",
        mediaType: vars.mediaType,
        title: vars.title ?? "",
      });
      showToast(
        `Couldn't start the download${vars.title ? ` for "${vars.title}"` : ""}. Please try again.`,
        { severity: "error" },
      );
    },
  });

  return {
    requestDownload: mutation.mutate,
    isPending: mutation.isPending,
  };
}
