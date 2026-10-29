import {
  buildStatusBadgeFromEnriched,
  calculateEpisodeCount,
  getStatusBadgeForMedia,
  getStatusBadgeForMediaWithEnriched,
} from "./statusHelpers";
import type {
  EnrichedMedia,
  MediaStatusInfo,
  SeasonStatus,
  StatusBatchResponse,
} from "../types/MediaStatus";

const ep = (n: number, downloaded: boolean) => ({ episodeNum: n, downloaded });
const season = (num: number, eps: ReturnType<typeof ep>[]): SeasonStatus => ({
  seasonNum: num,
  episodes: eps,
});

const batch: StatusBatchResponse = {
  shows: [
    { tvdbId: "s1", seasons: [season(1, [ep(1, true), ep(2, false)]), season(2, [ep(1, true)])] },
    { tvdbId: "empty-seasons", seasons: [] },
    { tvdbId: "empty-eps", seasons: [season(1, [])] },
  ],
  movies: [
    { tvdbId: "m1", inLibrary: true },
    { tvdbId: "m2", inLibrary: false },
  ],
};

describe("calculateEpisodeCount", () => {
  it("sums across seasons", () => {
    expect(calculateEpisodeCount(batch.shows[0].seasons)).toEqual({ downloaded: 2, total: 3 });
  });
  it.each([
    [[], { downloaded: 0, total: 0 }],
    [[season(1, [])], { downloaded: 0, total: 0 }],
    [[season(1, [ep(1, false)])], { downloaded: 0, total: 1 }],
    [[season(1, [ep(1, true), ep(2, true)])], { downloaded: 2, total: 2 }],
  ])("%j", (seasons, expected) => {
    expect(calculateEpisodeCount(seasons)).toEqual(expected);
  });
});

describe("getStatusBadgeForMedia", () => {
  it.each([
    ["movie in library", "m1", "movie", { type: "movie", inLibrary: true }],
    ["movie not in library", "m2", "movie", undefined],
    ["unknown movie", "nope", "movie", undefined],
    ["show with episodes", "s1", "show", { type: "show", episodeCount: { downloaded: 2, total: 3 } }],
    ["show without seasons", "empty-seasons", "show", undefined],
    ["show with empty seasons", "empty-eps", "show", undefined],
    ["unknown show", "nope", "show", undefined],
    ["show id looked up as movie", "s1", "movie", undefined],
  ] as const)("%s", (_n, id, type, expected) => {
    expect(getStatusBadgeForMedia(id, batch, type)).toEqual(expected);
  });

  it("returns undefined without status data", () => {
    expect(getStatusBadgeForMedia("m1", undefined, "movie")).toBeUndefined();
  });

  it("returns undefined for an unrecognised media type", () => {
    expect(getStatusBadgeForMedia("m1", batch, "other" as "movie")).toBeUndefined();
  });
});

describe("buildStatusBadgeFromEnriched", () => {
  it.each<[string, MediaStatusInfo, unknown]>([
    ["movie in library", { type: "movie", inLibrary: true }, { type: "movie", inLibrary: true }],
    ["movie not in library", { type: "movie", inLibrary: false }, undefined],
    ["movie with no flag", { type: "movie" }, undefined],
    ["series with counts", { type: "series", downloaded: 3, total: 10 }, { type: "show", episodeCount: { downloaded: 3, total: 10 } }],
    ["series total only", { type: "series", total: 5 }, { type: "show", episodeCount: { downloaded: 0, total: 5 } }],
    ["series with zero total", { type: "series", downloaded: 0, total: 0 }, undefined],
    ["series with nothing", { type: "series" }, undefined],
  ])("%s", (_n, info, expected) => {
    expect(buildStatusBadgeFromEnriched(info)).toEqual(expected);
  });

  it("returns undefined for an unknown type", () => {
    expect(buildStatusBadgeFromEnriched({ type: "x" } as unknown as MediaStatusInfo)).toBeUndefined();
  });
});

describe("getStatusBadgeForMediaWithEnriched", () => {
  const media = {} as EnrichedMedia["media"];

  it("prefers enriched data over batch data", () => {
    const enriched: EnrichedMedia = { media, status: { type: "series", downloaded: 1, total: 9 } };
    expect(getStatusBadgeForMediaWithEnriched("s1", batch, "show", enriched)).toEqual({
      type: "show",
      episodeCount: { downloaded: 1, total: 9 },
    });
  });

  it("falls back to batch data when enriched yields no badge", () => {
    const enriched: EnrichedMedia = { media, status: { type: "series", total: 0 } };
    expect(getStatusBadgeForMediaWithEnriched("s1", batch, "show", enriched)).toEqual({
      type: "show",
      episodeCount: { downloaded: 2, total: 3 },
    });
  });

  it.each([undefined, null])("falls back to batch data when enriched is %s", (enriched) => {
    expect(getStatusBadgeForMediaWithEnriched("m1", batch, "movie", enriched)).toEqual({
      type: "movie",
      inLibrary: true,
    });
  });

  it("returns undefined when neither source has a badge", () => {
    expect(getStatusBadgeForMediaWithEnriched("nope", undefined, "movie")).toBeUndefined();
  });
});
