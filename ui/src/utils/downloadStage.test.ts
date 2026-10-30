import {
  ACTIVE_STAGES,
  formatEpisodeLabel,
  formatRelativeTime,
  formatTimeUntil,
  getStageColor,
  getStageDescription,
  getStageLabel,
  isActiveStage,
} from "./downloadStage";

const stages = ["scheduled", "searching", "downloading", "completed", "failed", "cancelled"] as const;

describe("stage lookups", () => {
  it.each([
    ["scheduled", "#4F46E5", "Scheduled", "Waiting for release", true],
    ["searching", "#FFA500", "Searching", "Looking for a torrent", true],
    ["downloading", "#8B5CF6", "Downloading", "Downloading", true],
    ["completed", "#10B981", "Completed", "In your library", false],
    ["failed", "#EF4444", "Failed", "Gave up", false],
    ["cancelled", "#64748B", "Cancelled", "Cancelled", false],
  ])("%s", (stage, color, label, description, active) => {
    expect(getStageColor(stage)).toBe(color);
    expect(getStageLabel(stage)).toBe(label);
    expect(getStageDescription(stage)).toBe(description);
    expect(isActiveStage(stage)).toBe(active);
  });

  it("covers every stage", () => {
    for (const s of stages) expect(getStageColor(s)).not.toBe("#9CA3AF");
  });

  it.each([undefined, "", "bogus", "COMPLETED"])("handles unknown input %j", (input) => {
    expect(getStageColor(input)).toBe("#9CA3AF");
    expect(getStageDescription(input)).toBe("");
    expect(isActiveStage(input)).toBe(false);
  });

  it("labels unknown input", () => {
    expect(getStageLabel(undefined)).toBe("Unknown");
    expect(getStageLabel("")).toBe("Unknown");
    expect(getStageLabel("bogus")).toBe("Bogus");
  });

  it("ACTIVE_STAGES lists the in-flight stages", () => {
    expect([...ACTIVE_STAGES].sort()).toEqual(["downloading", "scheduled", "searching"]);
  });
});

describe("formatEpisodeLabel", () => {
  it.each([
    [[1, 4], "Season 01 Episode 04"],
    [[12, 103], "Season 12 Episode 103"],
    [[0, 0], "Season 00 Episode 00"],
    [[1, 4, 30, false], "Season 01 Episode 04"],
    [[1, 4, 30, true], "Episode 30"],
    [[undefined, undefined, 12, true], "Episode 12"],
    [[1, 4, undefined, true], "Season 01 Episode 04"],
    [[1, 4, 0, true], "Season 01 Episode 04"],
    [[undefined, 4], "Episode"],
    [[1, undefined], "Episode"],
    [[], "Episode"],
  ] as const)("%j -> %s", (args, expected) => {
    expect(formatEpisodeLabel(...(args as unknown as Parameters<typeof formatEpisodeLabel>))).toBe(expected);
  });
});

describe("relative time", () => {
  const now = new Date("2026-06-15T12:00:00Z");
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(now);
  });
  afterEach(() => vi.useRealTimers());

  const offset = (ms: number) => new Date(now.getTime() + ms).toISOString();
  const MIN = 60_000;
  const HOUR = 60 * MIN;
  const DAY = 24 * HOUR;

  it.each([
    [0, "Just now"],
    [-30_000, "Just now"],
    [-1 * MIN, "1m ago"],
    [-59 * MIN, "59m ago"],
    [-60 * MIN, "1h ago"],
    [-23 * HOUR, "23h ago"],
    [-24 * HOUR, "1d ago"],
    [-6 * DAY, "6d ago"],
  ])("formatRelativeTime(%d ms) -> %s", (ms, expected) => {
    expect(formatRelativeTime(offset(ms))).toBe(expected);
  });

  it("falls back to a date after a week", () => {
    const ts = offset(-7 * DAY);
    expect(formatRelativeTime(ts)).toBe(new Date(ts).toLocaleDateString());
  });

  it.each([
    [0, "any moment"],
    [-5 * MIN, "any moment"],
    [10_000, "in 1m"],
    [12 * MIN, "in 12m"],
    [3 * HOUR, "in 3h"],
    [3 * DAY, "in 3d"],
  ])("formatTimeUntil(%d ms) -> %s", (ms, expected) => {
    expect(formatTimeUntil(offset(ms))).toBe(expected);
  });

  it("falls back to a date a week or more out", () => {
    const ts = offset(10 * DAY);
    expect(formatTimeUntil(ts)).toBe(`on ${new Date(ts).toLocaleDateString()}`);
  });
});
