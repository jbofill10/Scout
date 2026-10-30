import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../contexts/ToastContext";
import { logError } from "../lib/logger";

/** The settings as the API presents them; durations are Go strings ("2m"). */
export interface SettingsValues {
  min_quality: string;
  /** Gigabytes kept free on the downloads volume. */
  min_free_space_gb: number;
  subtitle_languages: string[];
  subtitle_prefer_sdh: boolean;
  plex_sync_delay: string;
  plex_sync_interval: string;
  /** Let the torrenter rotate the Mullvad relay when an indexer blocks the exit IP. */
  auto_rotate_relay: boolean;
  /** Quality upgrades: replace library copies below the cutoff, batch items per pass. */
  upgrades_enabled: boolean;
  upgrade_cutoff: string;
  upgrade_batch: number;
  /** Season packs: one torrent for a season when a request covers enough of it. */
  season_packs: boolean;
  season_pack_min_episodes: number;
  /** Search pacing: per-indexer sustained rate and burst, optional per-indexer overrides keyed by Prowlarr id. */
  search_rate_per_minute: number;
  search_burst: number;
  indexer_rate_limits: Record<string, RateLimit>;
  /** Episodes a scheduled follow check may request per run. */
  bulk_request_batch: number;
}

export interface RateLimit {
  per_minute: number;
  burst: number;
}

export type SettingKey = keyof SettingsValues;

export interface SettingsView {
  values: SettingsValues;
  defaults: SettingsValues;
  overridden: Partial<Record<SettingKey, boolean>>;
  options: {
    min_quality: string[];
    subtitles_enabled: boolean;
    /** Free bytes on the downloads volume right now; -1 when unknown. */
    free_space_bytes: number;
    /** Whether a relay agent is configured, so rotation is possible at all. */
    relay_agent_configured: boolean;
  };
}

/** PUT body: any subset of keys, plus keys to reset to their default. */
export interface SettingsUpdate extends Partial<SettingsValues> {
  reset?: SettingKey[];
}

export interface UploaderPreference {
  id: number;
  media_type: "series" | "movie";
  is_anime: boolean;
  uploader: string;
}

export type NewUploaderPreference = Omit<UploaderPreference, "id">;

export const SETTINGS_QUERY_KEY = ["settings"] as const;
export const UPLOADERS_QUERY_KEY = ["settings", "uploaders"] as const;

/** An HTTP error carrying the server's message and, for validation, the field. */
export class SettingsError extends Error {
  status: number;
  field?: string;
  constructor(status: number, message: string, field?: string) {
    super(message);
    this.status = status;
    this.field = field;
  }
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init);
  if (!response.ok) {
    let message = response.statusText;
    let field: string | undefined;
    try {
      const data = await response.json();
      if (data && typeof data.error === "string") message = data.error;
      if (data && typeof data.field === "string") field = data.field;
    } catch {
      // keep the status text
    }
    throw new SettingsError(response.status, message, field);
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}

export function useSettings() {
  return useQuery({
    queryKey: SETTINGS_QUERY_KEY,
    queryFn: () => request<SettingsView>("/api/settings"),
    staleTime: 30_000,
  });
}

/**
 * Saves a subset of settings. The server answers with the full view, which
 * replaces the cached one so the page reflects the change at once.
 */
export function useUpdateSettings() {
  const queryClient = useQueryClient();
  const showToast = useToast();

  return useMutation<SettingsView, Error, SettingsUpdate>({
    mutationFn: (update) =>
      request<SettingsView>("/api/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(update),
      }),
    retry: 0,
    onSuccess: (view) => {
      queryClient.setQueryData(SETTINGS_QUERY_KEY, view);
      showToast("Settings saved", { severity: "success" });
    },
    onError: (error) => {
      logError("Settings update failed", error, { component: "useUpdateSettings" });
      showToast(error.message || "Couldn't save settings.", { severity: "error" });
    },
  });
}

export function useUploaderPreferences() {
  return useQuery({
    queryKey: UPLOADERS_QUERY_KEY,
    queryFn: () => request<UploaderPreference[]>("/api/settings/uploaders"),
    staleTime: 30_000,
  });
}

export function useAddUploader() {
  const queryClient = useQueryClient();
  const showToast = useToast();

  return useMutation<UploaderPreference, Error, NewUploaderPreference>({
    mutationFn: (pref) =>
      request<UploaderPreference>("/api/settings/uploaders", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(pref),
      }),
    retry: 0,
    onSuccess: (pref) => {
      queryClient.invalidateQueries({ queryKey: UPLOADERS_QUERY_KEY });
      showToast(`Preferring ${pref.uploader}`, { severity: "success" });
    },
    onError: (error) => {
      logError("Add uploader failed", error, { component: "useAddUploader" });
      showToast(error.message || "Couldn't add that uploader.", { severity: "error" });
    },
  });
}

export function useRemoveUploader() {
  const queryClient = useQueryClient();
  const showToast = useToast();

  return useMutation<void, Error, UploaderPreference>({
    mutationFn: (pref) => request<void>(`/api/settings/uploaders/${pref.id}`, { method: "DELETE" }),
    retry: 0,
    onSuccess: (_data, pref) => {
      queryClient.invalidateQueries({ queryKey: UPLOADERS_QUERY_KEY });
      showToast(`Removed ${pref.uploader}`, { severity: "info" });
    },
    onError: (error) => {
      logError("Remove uploader failed", error, { component: "useRemoveUploader" });
      showToast(error.message || "Couldn't remove that uploader.", { severity: "error" });
      queryClient.invalidateQueries({ queryKey: UPLOADERS_QUERY_KEY });
    },
  });
}
