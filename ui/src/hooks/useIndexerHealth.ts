import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../contexts/ToastContext";
import { logError } from "../lib/logger";

export interface IndexerState {
  id: number;
  name: string;
  enabled: boolean;
  healthy: boolean;
  disabled_till?: string;
  failure?: string;
  /** The failure reads like the exit IP being refused; a relay change can fix it. */
  ip_blocked: boolean;
}

export interface RelayInfo {
  configured: boolean;
  connected: boolean;
  relay?: string;
  ip?: string;
  location?: string;
  last_rotation_at?: string;
  last_rotation_reason?: string;
  error?: string;
}

export interface HealthEvent {
  at: string;
  kind: "rotation" | "indexer_test" | "check_error" | string;
  message: string;
  indexer?: string;
  ok: boolean;
}

/** One indexer's search queue: what is running, what waits, and at what rate. */
export interface IndexerQueueStats {
  id: number;
  limits: { per_minute: number; burst: number };
  tokens: number;
  in_flight?: string;
  queued: number;
  queued_by_priority?: Record<string, number>;
  waiting?: string[];
  searches_last_minute: number;
  paused_until?: string;
  pause_reason?: string;
}

export interface IndexerHealthSnapshot {
  indexers: IndexerState[];
  relay: RelayInfo;
  events: HealthEvent[];
  auto_rotate: boolean;
  checked_at?: string;
  check_error?: string;
  /** Per-indexer search queue, lowest id first; absent on older servers. */
  queue?: IndexerQueueStats[];
}

/** "Searching Severance S01E04 · 3 waiting (manual 1, follow 2) · 2/6 per min". */
export function describeQueue(q: IndexerQueueStats): string {
  const parts: string[] = [];
  if (q.in_flight) parts.push(`searching ${q.in_flight}`);
  if (q.queued > 0) {
    const by = q.queued_by_priority
      ? ` (${Object.entries(q.queued_by_priority)
          .map(([k, v]) => `${k} ${v}`)
          .join(", ")})`
      : "";
    parts.push(`${q.queued} waiting${by}`);
  }
  parts.push(`${q.searches_last_minute}/${q.limits.per_minute} per min`);
  if (q.paused_until) {
    const until = new Date(q.paused_until);
    parts.push(`paused until ${until.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}${q.pause_reason ? ` (${q.pause_reason})` : ""}`);
  }
  return parts.join(" · ");
}

export const INDEXER_HEALTH_QUERY_KEY = ["indexers", "health"] as const;

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
  return response.json();
}

/** Indexer and relay state, refreshed every 30 seconds while shown. */
export function useIndexerHealth(enabled: boolean = true) {
  return useQuery({
    queryKey: INDEXER_HEALTH_QUERY_KEY,
    queryFn: () => request<IndexerHealthSnapshot>("/api/indexers/health"),
    enabled,
    staleTime: 10_000,
    refetchInterval: enabled ? 30_000 : false,
  });
}

/** Count of indexers that are enabled but currently disabled by Prowlarr. */
export function unhealthyCount(snapshot?: IndexerHealthSnapshot): number {
  return snapshot?.indexers.filter((i) => i.enabled && !i.healthy).length ?? 0;
}

export function useRotateRelay() {
  const queryClient = useQueryClient();
  const showToast = useToast();
  return useMutation<HealthEvent, Error, void>({
    mutationFn: () =>
      request<HealthEvent>("/api/indexers/rotate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason: "requested from the UI" }),
      }),
    retry: 0,
    onSuccess: (ev) => {
      queryClient.invalidateQueries({ queryKey: INDEXER_HEALTH_QUERY_KEY });
      showToast(ev.message, { severity: ev.ok ? "success" : "warning" });
    },
    onError: (error) => {
      logError("Relay rotation failed", error, { component: "useRotateRelay" });
      showToast(error.message || "Couldn't rotate the relay.", { severity: "error" });
      queryClient.invalidateQueries({ queryKey: INDEXER_HEALTH_QUERY_KEY });
    },
  });
}

export function useTestIndexer() {
  const queryClient = useQueryClient();
  const showToast = useToast();
  return useMutation<{ ok: boolean }, Error, IndexerState>({
    mutationFn: (indexer) => request<{ ok: boolean }>(`/api/indexers/${indexer.id}/test`, { method: "POST" }),
    retry: 0,
    onSuccess: (_d, indexer) => {
      queryClient.invalidateQueries({ queryKey: INDEXER_HEALTH_QUERY_KEY });
      showToast(`${indexer.name} passed its test`, { severity: "success" });
    },
    onError: (error, indexer) => {
      logError("Indexer test failed", error, { component: "useTestIndexer", indexer: indexer.name });
      showToast(`${indexer.name}: ${error.message || "test failed"}`, { severity: "error" });
      queryClient.invalidateQueries({ queryKey: INDEXER_HEALTH_QUERY_KEY });
    },
  });
}
