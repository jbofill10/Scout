import type { ActivityItem } from "../hooks/useActivity";
import type { UpcomingItem } from "../hooks/useUpcoming";
import { buildViews, fromActivityItem, fromUpcomingItem, groupRows, summarizeStages } from "./activityRows";

const NOW = new Date("2026-10-29T12:00:00Z");

const activityItem = (over: Partial<ActivityItem> = {}): ActivityItem => ({
  id: 1,
  tvdb_id: "1001",
  media_title: "The Bear",
  category: "series",
  season: 3,
  episode: 1,
  is_anime: false,
  status: "downloading",
  created_at: "2026-10-29T11:00:00Z",
  updated_at: "2026-10-29T11:58:00Z",
  can_cancel: true,
  ...over,
});

const upcomingItem = (over: Partial<UpcomingItem> = {}): UpcomingItem => ({
  id: 50,
  tvdb_id: "2001",
  media_title: "The Bear",
  category: "series",
  season: 3,
  episode: 5,
  is_anime: false,
  release_time: "2026-11-01T12:00:00Z",
  due_at: "2026-11-01T12:00:00Z",
  attempts: 0,
  ...over,
});

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
});

describe("fromActivityItem", () => {
  it("writes the short episode code and the stage in words", () => {
    const row = fromActivityItem(activityItem());
    expect(row.code).toBe("S03E01");
    expect(row.status).toBe("Downloading");
    expect(row.tone).toBe("default");
    expect(row.updated).toBe("2m ago");
  });

  it("uses the absolute number for anime and 'Movie' for films", () => {
    expect(fromActivityItem(activityItem({ is_anime: true, absolute_episode: 12 })).code).toBe("E12");
    expect(fromActivityItem(activityItem({ category: "movie", season: undefined, episode: undefined })).code).toBe("Movie");
  });

  it("leads with the retry time and keeps the reason for a retry wait", () => {
    const row = fromActivityItem(
      activityItem({ status: "searching", next_attempt_at: "2026-10-29T12:12:00Z", attempts: 2, reason: "No torrent found yet" }),
    );
    expect(row.status).toBe("Retrying in 12m (attempt 3) · No torrent found yet");
    expect(row.tone).toBe("warning");
  });

  it("shows the failure reason in the error tone", () => {
    const row = fromActivityItem(activityItem({ status: "failed", reason: "Not enough disk space" }));
    expect(row.status).toBe("Not enough disk space");
    expect(row.tone).toBe("error");
  });

  it("mentions the pending retry on a failed item the webserver re-armed", () => {
    const row = fromActivityItem(
      activityItem({ status: "failed", reason: "Lost track of the download", next_attempt_at: "2026-10-29T13:00:00Z", schedule_status: "pending" }),
    );
    expect(row.status).toBe("Lost track of the download · retrying in 1h");
    expect(row.tone).toBe("error");
  });
});

describe("fromUpcomingItem", () => {
  it("describes a release wait as scheduled and a retry wait as searching", () => {
    expect(fromUpcomingItem(upcomingItem())).toMatchObject({ stage: "scheduled", status: "Releases in 3d", canCancel: true, canRetry: false, canPick: false });
    expect(
      fromUpcomingItem(upcomingItem({ next_attempt_at: "2026-10-29T16:00:00Z", due_at: "2026-10-29T16:00:00Z", attempts: 1, last_failure_reason: "Indexer timed out" })),
    ).toMatchObject({ stage: "searching", status: "Retrying in 4h (attempt 2) · Indexer timed out", canRetry: true, canPick: true, tone: "warning" });
  });
});

describe("buildViews", () => {
  it("shows a future episode once, as its schedule row", () => {
    const views = buildViews(
      { active: [activityItem({ id: 2, tvdb_id: "2001", status: "scheduled" })], recent: [], counts: {} },
      [upcomingItem({ tvdb_id: "2001" })],
    );
    expect(views.inFlight).toHaveLength(0);
    expect(views.upcoming.map((row) => row.itemId)).toEqual(["2001"]);
  });

  it("shows a retry wait once, as the in-flight notification", () => {
    const views = buildViews(
      { active: [activityItem({ status: "searching", next_attempt_at: "2026-10-29T13:00:00Z" })], recent: [], counts: {} },
      [upcomingItem({ tvdb_id: "1001", next_attempt_at: "2026-10-29T13:00:00Z" })],
    );
    expect(views.inFlight.map((row) => row.itemId)).toEqual(["1001"]);
    expect(views.upcoming).toHaveLength(0);
  });

  it("drops a pending row whose item is already downloading or searching", () => {
    const views = buildViews(
      { active: [activityItem({ tvdb_id: "7", status: "downloading" })], recent: [], counts: {} },
      [upcomingItem({ tvdb_id: "7" })],
    );
    expect(views.inFlight.map((row) => row.stage)).toEqual(["downloading"]);
    expect(views.upcoming).toHaveLength(0);
  });

  it("keeps a scheduled notification that has no schedule row", () => {
    const views = buildViews({ active: [activityItem({ status: "scheduled" })], recent: [], counts: {} }, []);
    expect(views.inFlight.map((row) => row.stage)).toEqual(["scheduled"]);
  });

  it("puts finished notifications in their own list and tolerates missing data", () => {
    expect(buildViews(undefined, undefined)).toEqual({ inFlight: [], upcoming: [], finished: [] });
    const views = buildViews({ active: [], recent: [activityItem({ status: "completed" })], counts: {} }, undefined);
    expect(views.finished).toHaveLength(1);
  });
});

describe("groupRows", () => {
  const rows = [
    fromActivityItem(activityItem({ id: 1, tvdb_id: "1", season: 3, episode: 2, status: "searching", updated_at: "2026-10-29T11:50:00Z" })),
    fromActivityItem(activityItem({ id: 2, tvdb_id: "2", season: 3, episode: 1, status: "downloading", updated_at: "2026-10-29T11:59:00Z" })),
    fromActivityItem(activityItem({ id: 3, tvdb_id: "3", season: 2, episode: 8, status: "downloading", updated_at: "2026-10-29T11:00:00Z" })),
    fromActivityItem(activityItem({ id: 4, tvdb_id: "4", media_title: "Dune: Part Two", category: "movie", season: undefined, episode: undefined, updated_at: "2026-10-29T11:55:00Z" })),
  ];

  it("folds a show's episodes into one group in episode order with stage counts", () => {
    const groups = groupRows(rows, "latest");
    expect(groups.map((group) => group.title)).toEqual(["The Bear", "Dune: Part Two"]);
    const bear = groups[0];
    expect(bear.rows.map((row) => row.code)).toEqual(["S02E08", "S03E01", "S03E02"]);
    expect(bear.stageCounts).toEqual({ downloading: 2, searching: 1 });
    expect(bear.seasonLabel).toBe("Seasons 2–3");
    expect(bear.lead.code).toBe("S03E01");
    expect(groups[1].seasonLabel).toBe("");
  });

  it("orders upcoming groups by the soonest row and leads with it", () => {
    const upcoming = [
      fromUpcomingItem(upcomingItem({ id: 1, tvdb_id: "a", media_title: "Late Show", release_time: "2026-11-05T12:00:00Z", due_at: "2026-11-05T12:00:00Z" })),
      fromUpcomingItem(upcomingItem({ id: 2, tvdb_id: "b", media_title: "Soon Show", episode: 7, release_time: "2026-11-02T12:00:00Z", due_at: "2026-11-02T12:00:00Z" })),
      fromUpcomingItem(upcomingItem({ id: 3, tvdb_id: "c", media_title: "Soon Show", episode: 6, release_time: "2026-10-30T12:00:00Z", due_at: "2026-10-30T12:00:00Z" })),
    ];
    const groups = groupRows(upcoming, "soonest");
    expect(groups.map((group) => group.title)).toEqual(["Soon Show", "Late Show"]);
    expect(groups[0].lead.code).toBe("S03E06");
    expect(groups[0].lead.status).toBe("Releases in 1d");
  });

  it("groups case-insensitively by title within a category", () => {
    const groups = groupRows(
      [fromActivityItem(activityItem({ id: 1, tvdb_id: "1" })), fromActivityItem(activityItem({ id: 2, tvdb_id: "2", media_title: "the bear ", episode: 2 }))],
      "latest",
    );
    expect(groups).toHaveLength(1);
    expect(groups[0].rows).toHaveLength(2);
  });
});

describe("summarizeStages", () => {
  it("lists the active stages first", () => {
    expect(summarizeStages({ failed: 1, downloading: 3, searching: 2 })).toBe("3 downloading · 2 searching · 1 failed");
    expect(summarizeStages({})).toBe("");
  });
});
