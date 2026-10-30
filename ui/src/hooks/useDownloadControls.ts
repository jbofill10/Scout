import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../contexts/ToastContext";
import { logError } from "../lib/logger";

/** One torrent an indexer has for an item, with Scout's verdict on it. */
export interface ReleaseCandidate {
  title: string;
  indexer: string;
  indexer_id: number;
  guid: string;
  download_url?: string;
  info_hash?: string;
  size: number;
  seeders: number;
  leechers: number;
  publish_date: string;
  quality?: string;
  source?: string;
  codec?: string;
  release_group?: string;
  /** Whether Scout would pick this on its own; reason explains a rejection. */
  accepted: boolean;
  reason?: string;
  score: number;
  /** Scout already tried this release for the item and gave up on it. */
  previously_failed: boolean;
  query: string;
}

export interface ReleasesResponse {
  item_id: string;
  media_title: string;
  queries: number;
  candidates: ReleaseCandidate[];
}

/** What the grab endpoint needs to hand a candidate to qBittorrent. */
export interface ReleaseSelection {
  guid: string;
  download_url: string;
  info_hash: string;
  title: string;
  indexer: string;
  indexer_id: number;
  size: number;
  seeders: number;
  leechers: number;
}

/** The torrenter's per-item result for a manual grab. */
export interface GrabResult {
  tvdb_id: string;
  outcome: "downloading" | "exists" | "failed";
  code?: string;
  reason?: string;
}

export function toSelection(c: ReleaseCandidate): ReleaseSelection {
  return {
    guid: c.guid,
    download_url: c.download_url ?? "",
    info_hash: c.info_hash ?? "",
    title: c.title,
    indexer: c.indexer,
    indexer_id: c.indexer_id,
    size: c.size,
    seeders: c.seeders,
    leechers: c.leechers,
  };
}

/** An HTTP error with the status attached, so callers can word 404 and 409. */
export class ControlError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function postControl<T>(url: string, body?: unknown): Promise<T> {
  const response = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    let message = response.statusText;
    try {
      const data = await response.json();
      if (data && typeof data.error === "string") message = data.error;
    } catch {
      // keep the status text
    }
    throw new ControlError(response.status, message);
  }
  return response.json();
}

async function fetchReleases(itemId: string): Promise<ReleasesResponse> {
  const response = await fetch(`/api/downloads/${encodeURIComponent(itemId)}/releases`);
  if (!response.ok) {
    let message = response.statusText;
    try {
      const data = await response.json();
      if (data && typeof data.error === "string") message = data.error;
    } catch {
      // keep the status text
    }
    throw new ControlError(response.status, message);
  }
  const data = await response.json();
  return { ...data, candidates: data.candidates ?? [] };
}

/**
 * The candidate releases for an item. Disabled until an item is given, so a
 * dialog can mount closed without searching. Cached briefly: a listing hits
 * every indexer and counts against their rate limits.
 */
export function useReleases(itemId: string | null) {
  return useQuery({
    queryKey: ["releases", itemId],
    queryFn: () => fetchReleases(itemId as string),
    enabled: itemId !== null && itemId !== "",
    staleTime: 5 * 60 * 1000,
    retry: 0,
  });
}

export interface ControlVars {
  itemId: string;
  /** For the confirmation toast. */
  title: string;
}

export interface GrabVars extends ControlVars {
  release: ReleaseSelection;
}

function explain(error: unknown, fallback: string): string {
  if (error instanceof ControlError) {
    if (error.status === 404) return "Nothing left to act on for that item.";
    if (error.status === 409) return "That item is being dispatched right now; try again in a moment.";
  }
  return fallback;
}

/**
 * useDownloadControls
 *
 * Cancel, retry-now and manual grab for a requested item, keyed by the item's
 * tvdb id (the one on every activity row). Each action reports through the app
 * toast and refreshes the activity, notification and schedule views.
 */
export function useDownloadControls() {
  const showToast = useToast();
  const queryClient = useQueryClient();

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ["activity"] });
    queryClient.invalidateQueries({ queryKey: ["notifications"] });
    queryClient.invalidateQueries({ queryKey: ["schedule"] });
  };

  const cancel = useMutation<unknown, Error, ControlVars>({
    mutationFn: ({ itemId }) => postControl(`/api/downloads/${encodeURIComponent(itemId)}/cancel`),
    retry: 0,
    onSuccess: (_data, vars) => {
      showToast(`Cancelled ${vars.title}`, { severity: "info" });
      refresh();
    },
    onError: (error, vars) => {
      logError("Cancel failed", error, { component: "useDownloadControls", itemId: vars.itemId });
      showToast(explain(error, `Couldn't cancel ${vars.title}.`), { severity: "error" });
      refresh();
    },
  });

  const retryNow = useMutation<unknown, Error, ControlVars>({
    mutationFn: ({ itemId }) => postControl(`/api/downloads/${encodeURIComponent(itemId)}/retry`),
    retry: 0,
    onSuccess: (_data, vars) => {
      showToast(`Retrying ${vars.title} now`, { severity: "success", linkToActivity: true });
      refresh();
    },
    onError: (error, vars) => {
      logError("Retry failed", error, { component: "useDownloadControls", itemId: vars.itemId });
      showToast(explain(error, `Couldn't retry ${vars.title}.`), { severity: "error" });
      refresh();
    },
  });

  const grab = useMutation<GrabResult, Error, GrabVars>({
    mutationFn: ({ itemId, release }) =>
      postControl<GrabResult>(`/api/downloads/${encodeURIComponent(itemId)}/releases`, release),
    retry: 0,
    onSuccess: (result, vars) => {
      if (result.outcome === "downloading") {
        showToast(`Downloading ${vars.release.title}`, { severity: "success", linkToActivity: true });
      } else {
        showToast(result.reason || `Couldn't start ${vars.release.title}.`, { severity: "warning" });
      }
      refresh();
    },
    onError: (error, vars) => {
      logError("Manual grab failed", error, { component: "useDownloadControls", itemId: vars.itemId });
      showToast(explain(error, `Couldn't start ${vars.release.title}.`), { severity: "error" });
      refresh();
    },
  });

  const pendingFor = (mutation: { isPending: boolean; variables?: ControlVars }, itemId: string) =>
    mutation.isPending && mutation.variables?.itemId === itemId;

  return {
    cancel: cancel.mutate,
    retryNow: retryNow.mutate,
    grab: grab.mutate,
    isCancelling: (itemId: string) => pendingFor(cancel, itemId),
    isRetrying: (itemId: string) => pendingFor(retryNow, itemId),
    isGrabbing: (itemId: string) => pendingFor(grab, itemId),
  };
}
