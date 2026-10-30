import type { ActivityItem, ActivitySnapshot } from "../hooks/useActivity";
import type { UpcomingItem } from "../hooks/useUpcoming";
import {
  formatEpisodeCode,
  formatRelativeTime,
  formatTimeUntil,
  getStageDescription,
  type DownloadStage,
} from "./downloadStage";

/**
 * The Activity page's view model.
 *
 * Two endpoints feed the page: `/api/activity` (notifications, one per episode
 * or movie, with the stage each has reached) and `/api/schedule/upcoming`
 * (pending schedule rows with release and retry times). Both are flattened
 * into the same row shape here, split into the three tabs without showing an
 * item twice, and folded into one group per show or movie so a whole season
 * is a single line until it is opened.
 */

export type RowTone = "default" | "warning" | "error";

export type MediaCategory = "series" | "movie";

/** One episode or movie, whichever endpoint it came from. */
export interface ActivityRowModel {
  /** Unique within the page; the React key. */
  key: string;
  /** The item key every download control takes (episode or movie tvdb id). */
  itemId: string;
  title: string;
  category: MediaCategory;
  season?: number;
  episode?: number;
  absoluteEpisode?: number;
  isAnime: boolean;
  posterUrl?: string;
  stage: DownloadStage;
  /** "S03E01", "E12" for anime, "Movie". */
  code: string;
  /** Why it is waiting, when it retries or releases, or the stage in words. */
  status: string;
  tone: RowTone;
  /** "3m ago" for notifications; empty for schedule rows that have not run yet. */
  updated: string;
  traceId?: string;
  /** Epoch ms used to order groups: the last update, or the due time for upcoming rows. */
  sortAt: number;
  canCancel: boolean;
  canRetry: boolean;
  canPick: boolean;
}

/** Everything the page knows about one show or movie, across its rows. */
export interface ActivityGroup {
  /** Stable across polls: category plus the normalised title. */
  key: string;
  title: string;
  category: MediaCategory;
  posterUrl?: string;
  /** In season and episode order. */
  rows: ActivityRowModel[];
  stageCounts: Partial<Record<DownloadStage, number>>;
  /** "Season 3", "Seasons 1–3", or empty for movies. */
  seasonLabel: string;
  /** The row whose status best describes the group right now. */
  lead: ActivityRowModel;
  sortAt: number;
}

export interface ActivityViews {
  /** Downloading, searching and retry waits: what Scout is doing now. */
  inFlight: ActivityRowModel[];
  /** Waiting for a release date, and retries the in-flight list does not already show. */
  upcoming: ActivityRowModel[];
  /** Completed, failed and cancelled. */
  finished: ActivityRowModel[];
}

export type GroupOrder = "latest" | "soonest";

/** The order stages appear in summaries and bars: most active first. */
export const STAGE_ORDER: DownloadStage[] = [
  "downloading",
  "searching",
  "scheduled",
  "completed",
  "failed",
  "cancelled",
];

function describe(category: MediaCategory, season?: number, episode?: number, absolute?: number, anime?: boolean): string {
  if (category === "movie") return "Movie";
  return formatEpisodeCode(season, episode, absolute, anime) || "Episode";
}

function retryLine(nextAttemptAt: string, attempts?: number): string {
  const attempt = attempts ? ` (attempt ${attempts + 1})` : "";
  return `Retrying ${formatTimeUntil(nextAttemptAt)}${attempt}`;
}

/** Flattens a notification plus its schedule metadata into a row. */
export function fromActivityItem(item: ActivityItem): ActivityRowModel {
  const failed = item.status === "failed";
  const retrying = !failed && !!item.next_attempt_at;

  // The most useful "what now?" line: why it failed; when it retries and why it
  // has to; when it releases; otherwise the stage in words.
  let status: string;
  if (failed) {
    status = item.reason || item.last_failure_reason || getStageDescription(item.status);
    // A lost download the webserver has re-armed: say so, or the row reads as given up.
    if (item.next_attempt_at && item.schedule_status === "pending") {
      status += ` · retrying ${formatTimeUntil(item.next_attempt_at)}`;
    }
  } else if (retrying) {
    const why = item.reason || item.last_failure_reason;
    status = `${retryLine(item.next_attempt_at as string, item.attempts)}${why ? ` · ${why}` : ""}`;
  } else if (item.status === "scheduled" && item.release_time) {
    status = `Releases ${formatTimeUntil(item.release_time)}`;
  } else {
    status = item.reason || getStageDescription(item.status);
  }

  return {
    key: `activity-${item.id}-${item.tvdb_id}`,
    itemId: item.tvdb_id,
    title: item.media_title,
    category: item.category,
    season: item.season,
    episode: item.episode,
    absoluteEpisode: item.absolute_episode,
    isAnime: item.is_anime,
    posterUrl: item.poster_url || undefined,
    stage: item.status,
    code: describe(item.category, item.season, item.episode, item.absolute_episode, item.is_anime),
    status,
    tone: failed ? "error" : retrying ? "warning" : "default",
    updated: formatRelativeTime(item.updated_at),
    traceId: item.trace_id || undefined,
    sortAt: Date.parse(item.updated_at) || 0,
    canCancel: !!item.can_cancel,
    canRetry: !!item.can_retry,
    canPick: !!item.can_pick_release,
  };
}

/** Flattens a pending schedule row into a row. */
export function fromUpcomingItem(item: UpcomingItem): ActivityRowModel {
  const retrying = !!item.next_attempt_at;
  const status = retrying
    ? `${retryLine(item.next_attempt_at as string, item.attempts)}${item.last_failure_reason ? ` · ${item.last_failure_reason}` : ""}`
    : `Releases ${formatTimeUntil(item.release_time)}`;

  return {
    key: `upcoming-${item.id}-${item.tvdb_id}`,
    itemId: item.tvdb_id,
    title: item.media_title,
    category: item.category,
    season: item.season,
    episode: item.episode,
    absoluteEpisode: item.absolute_episode,
    isAnime: item.is_anime,
    posterUrl: item.poster_url || undefined,
    stage: retrying ? "searching" : "scheduled",
    code: describe(item.category, item.season, item.episode, item.absolute_episode, item.is_anime),
    status,
    tone: retrying ? "warning" : "default",
    updated: "",
    sortAt: Date.parse(item.due_at || item.release_time) || 0,
    canCancel: true,
    canRetry: retrying,
    canPick: retrying,
  };
}

/**
 * Splits the two feeds into the three tabs so that every item appears exactly
 * once. A future episode has both a `scheduled` notification and a pending
 * schedule row; the row wins because it knows the release time and can be
 * cancelled. A retry wait has a `searching` notification and a pending row;
 * the notification wins because it is what Scout is working on.
 */
export function buildViews(snapshot: ActivitySnapshot | undefined, upcoming: UpcomingItem[] | undefined): ActivityViews {
  const active = snapshot?.active ?? [];
  const recent = snapshot?.recent ?? [];
  const pending = upcoming ?? [];

  const pendingIds = new Set(pending.map((u) => u.tvdb_id));
  const inFlightItems = active.filter((item) => item.status !== "scheduled" || !pendingIds.has(item.tvdb_id));
  const inFlightIds = new Set(inFlightItems.map((item) => item.tvdb_id));

  return {
    inFlight: inFlightItems.map(fromActivityItem),
    upcoming: pending.filter((u) => !inFlightIds.has(u.tvdb_id)).map(fromUpcomingItem),
    finished: recent.map(fromActivityItem),
  };
}

export function countByStage(rows: ActivityRowModel[]): Partial<Record<DownloadStage, number>> {
  const counts: Partial<Record<DownloadStage, number>> = {};
  for (const row of rows) {
    counts[row.stage] = (counts[row.stage] ?? 0) + 1;
  }
  return counts;
}

/** "3 downloading · 5 searching", most active stage first. */
export function summarizeStages(counts: Partial<Record<DownloadStage, number>>): string {
  return STAGE_ORDER.filter((stage) => counts[stage])
    .map((stage) => `${counts[stage]} ${stage}`)
    .join(" · ");
}

function seasonLabel(rows: ActivityRowModel[]): string {
  const seasons = Array.from(
    new Set(rows.map((row) => row.season).filter((s): s is number => s !== undefined)),
  ).sort((a, b) => a - b);
  if (seasons.length === 0) return "";
  if (seasons.length === 1) return `Season ${seasons[0]}`;
  return `Seasons ${seasons[0]}–${seasons[seasons.length - 1]}`;
}

function episodeOrder(a: ActivityRowModel, b: ActivityRowModel): number {
  const seasonDiff = (a.season ?? 0) - (b.season ?? 0);
  if (seasonDiff !== 0) return seasonDiff;
  const episodeDiff = (a.episode ?? a.absoluteEpisode ?? 0) - (b.episode ?? b.absoluteEpisode ?? 0);
  if (episodeDiff !== 0) return episodeDiff;
  return a.title.localeCompare(b.title);
}

export function groupKey(category: MediaCategory, title: string): string {
  return `${category}:${title.trim().toLowerCase()}`;
}

/**
 * One group per show or movie. `latest` orders groups by their most recent
 * change (in flight, finished); `soonest` by their earliest due time (upcoming).
 * The lead row is the one that set the group's time, so the group line can say
 * "S03E04 retrying in 12m" or "next: S03E05 releases in 3d".
 */
export function groupRows(rows: ActivityRowModel[], order: GroupOrder): ActivityGroup[] {
  const byKey = new Map<string, ActivityRowModel[]>();
  for (const row of rows) {
    const key = groupKey(row.category, row.title);
    const bucket = byKey.get(key);
    if (bucket) bucket.push(row);
    else byKey.set(key, [row]);
  }

  const pick = order === "latest"
    ? (a: ActivityRowModel, b: ActivityRowModel) => (b.sortAt >= a.sortAt ? b : a)
    : (a: ActivityRowModel, b: ActivityRowModel) => (b.sortAt < a.sortAt ? b : a);

  const groups: ActivityGroup[] = [];
  byKey.forEach((bucket, key) => {
    const lead = bucket.reduce(pick);
    const sorted = [...bucket].sort(episodeOrder);
    groups.push({
      key,
      title: bucket[0].title,
      category: bucket[0].category,
      posterUrl: bucket.find((row) => row.posterUrl)?.posterUrl,
      rows: sorted,
      stageCounts: countByStage(sorted),
      seasonLabel: bucket[0].category === "series" ? seasonLabel(sorted) : "",
      lead,
      sortAt: lead.sortAt,
    });
  });

  groups.sort((a, b) => (order === "latest" ? b.sortAt - a.sortAt : a.sortAt - b.sortAt));
  return groups;
}
