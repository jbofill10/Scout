import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../contexts/ToastContext";
import { logError } from "../lib/logger";

export interface UpgradeAttempt {
  item_key: string;
  kind: "episode" | "movie";
  title: string;
  season?: number;
  episode?: number;
  from_quality: string;
  to_quality: string;
  status: "searching" | "downloading" | "completed" | "no_release" | "failed" | string;
  reason?: string;
  attempts: number;
  created_at: string;
  updated_at: string;
}

export interface UpgradeCandidate {
  item_key: string;
  kind: "episode" | "movie";
  title: string;
  season?: number;
  episode?: number;
  year?: number;
  current_quality: string;
  target_quality: string;
  files: string[];
  last_attempt?: UpgradeAttempt;
}

export interface UpgradeCandidatesView {
  enabled: boolean;
  cutoff: string;
  batch: number;
  total: number;
  candidates: UpgradeCandidate[];
  deferred: number;
}

export interface UpgradeRunResult {
  started: number;
  items: string[];
  message?: string;
}

export const UPGRADE_CANDIDATES_KEY = ["upgrades", "candidates"] as const;
export const UPGRADE_ATTEMPTS_KEY = ["upgrades", "attempts"] as const;

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

export function useUpgradeCandidates(enabled: boolean = true) {
  return useQuery({
    queryKey: UPGRADE_CANDIDATES_KEY,
    queryFn: () => request<UpgradeCandidatesView>("/api/upgrades/candidates"),
    enabled,
    staleTime: 60_000,
  });
}

export function useUpgradeAttempts(enabled: boolean = true) {
  return useQuery({
    queryKey: UPGRADE_ATTEMPTS_KEY,
    queryFn: () => request<UpgradeAttempt[]>("/api/upgrades/attempts"),
    enabled,
    staleTime: 30_000,
    refetchInterval: enabled ? 60_000 : false,
  });
}

/** "Some Show S01E05" or "A Movie (1994)". */
export function describeAttempt(a: { kind: string; title: string; season?: number; episode?: number; year?: number }): string {
  if (a.kind === "movie") return a.year ? `${a.title} (${a.year})` : a.title;
  if (a.season !== undefined && a.episode !== undefined) {
    return `${a.title} S${String(a.season).padStart(2, "0")}E${String(a.episode).padStart(2, "0")}`;
  }
  return a.title;
}

export function useRunUpgrades() {
  const queryClient = useQueryClient();
  const showToast = useToast();
  return useMutation<UpgradeRunResult, Error, void>({
    mutationFn: () => request<UpgradeRunResult>("/api/upgrades/run", { method: "POST" }),
    retry: 0,
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["upgrades"] });
      queryClient.invalidateQueries({ queryKey: ["activity"] });
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
      showToast(
        result.started > 0
          ? `Looking for better copies of ${result.started} item${result.started === 1 ? "" : "s"}: ${result.items.join(", ")}`
          : result.message || "Nothing to upgrade right now",
        { severity: result.started > 0 ? "success" : "info", linkToActivity: result.started > 0 },
      );
    },
    onError: (error) => {
      logError("Upgrade run failed", error, { component: "useRunUpgrades" });
      showToast(error.message || "Couldn't start the upgrade pass.", { severity: "error" });
    },
  });
}
