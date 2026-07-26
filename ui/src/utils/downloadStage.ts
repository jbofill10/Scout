/**
 * Shared vocabulary for the stages a download moves through.
 *
 * A request travels: scheduled (waiting for release) → searching (looking for a
 * torrent, including retries) → downloading (in qBittorrent) → completed, or
 * failed at any point. The notification bell and the Activity page both render
 * these stages, so the labels and colours live here.
 */

export type DownloadStage =
  | "scheduled"
  | "searching"
  | "downloading"
  | "completed"
  | "failed";

/** Stages that mean work is still in flight. */
export const ACTIVE_STAGES: DownloadStage[] = [
  "downloading",
  "searching",
  "scheduled",
];

const STAGE_COLORS: Record<DownloadStage, string> = {
  scheduled: "#4F46E5", // indigo
  searching: "#FFA500", // amber
  downloading: "#8B5CF6", // purple
  completed: "#10B981", // green
  failed: "#EF4444", // red
};

const STAGE_DESCRIPTIONS: Record<DownloadStage, string> = {
  scheduled: "Waiting for release",
  searching: "Looking for a torrent",
  downloading: "Downloading",
  completed: "In your library",
  failed: "Gave up",
};

const FALLBACK_COLOR = "#9CA3AF"; // gray

export function getStageColor(stage: string | undefined): string {
  if (!stage) return FALLBACK_COLOR;
  return STAGE_COLORS[stage as DownloadStage] ?? FALLBACK_COLOR;
}

export function getStageLabel(stage: string | undefined): string {
  if (!stage) return "Unknown";
  return stage.charAt(0).toUpperCase() + stage.slice(1);
}

export function getStageDescription(stage: string | undefined): string {
  if (!stage) return "";
  return STAGE_DESCRIPTIONS[stage as DownloadStage] ?? "";
}

export function isActiveStage(stage: string | undefined): boolean {
  return ACTIVE_STAGES.includes(stage as DownloadStage);
}

/** "Season 01 Episode 04", "Episode 12" for anime, or "Movie". */
export function formatEpisodeLabel(
  season?: number,
  episode?: number,
  absoluteEpisode?: number,
  isAnime?: boolean,
): string {
  if (isAnime && absoluteEpisode) {
    return `Episode ${absoluteEpisode}`;
  }
  if (season !== undefined && episode !== undefined) {
    return `Season ${season.toString().padStart(2, "0")} Episode ${episode
      .toString()
      .padStart(2, "0")}`;
  }
  return "Episode";
}

/** Compact "3m ago" style timestamp, falling back to a date past a week. */
export function formatRelativeTime(timestamp: string): string {
  const date = new Date(timestamp);
  const diffMs = Date.now() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return "Just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

/** Forward-looking counterpart to formatRelativeTime: "in 12m", "in 3d". */
export function formatTimeUntil(timestamp: string): string {
  const date = new Date(timestamp);
  const diffMs = date.getTime() - Date.now();
  if (diffMs <= 0) return "any moment";

  const diffMins = Math.round(diffMs / 60000);
  if (diffMins < 60) return `in ${Math.max(diffMins, 1)}m`;

  const diffHours = Math.round(diffMs / 3600000);
  if (diffHours < 24) return `in ${diffHours}h`;

  const diffDays = Math.round(diffMs / 86400000);
  if (diffDays < 7) return `in ${diffDays}d`;
  return `on ${date.toLocaleDateString()}`;
}
