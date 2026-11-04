import {
  coverageLabel,
  coverageSentence,
  episodeState,
  seasonTabLabel,
  summarizeCoverage,
} from "./episodeCoverage";

const today = new Date(2026, 10, 4); // 2026-11-04

describe("episodeState", () => {
  it.each([
    ["in the library", { downloaded: true, aired: "2025-07-11" }, "library"],
    ["in the library ahead of its date", { downloaded: true, aired: "2099-01-01" }, "library"],
    ["in the library with no date", { downloaded: true, aired: "" }, "library"],
    ["aired and absent", { downloaded: false, aired: "2025-07-11" }, "missing"],
    ["aired today and absent", { downloaded: false, aired: "2026-11-04" }, "missing"],
    ["no air information", { downloaded: false }, "missing"],
    ["airs later", { downloaded: false, aired: "2027-01-10" }, "upcoming"],
    ["no date yet", { downloaded: false, aired: "" }, "tba"],
    ["unparseable date", { downloaded: false, aired: "TBA" }, "tba"],
  ] as const)("%s", (_name, episode, expected) => {
    expect(episodeState(episode, today)).toBe(expected);
  });
});

describe("summarizeCoverage", () => {
  it("counts only aired episodes against the library", () => {
    // Dexter: Resurrection mid-run: season 1 in, season 2 announced
    const episodes = [
      ...Array.from({ length: 11 }, () => ({ downloaded: true, aired: "2025-07-11" })),
      { downloaded: false, aired: "2027-01-10" },
      ...Array.from({ length: 8 }, () => ({ downloaded: false, aired: "" })),
    ];
    expect(summarizeCoverage(episodes, today)).toEqual({ downloaded: 11, aired: 11, upcoming: 9 });
  });

  it("reports a gap in aired episodes as missing", () => {
    const episodes = [
      { downloaded: true, aired: "2026-10-21" },
      { downloaded: false, aired: "2026-10-28" },
      { downloaded: false, aired: "2026-11-11" },
    ];
    expect(summarizeCoverage(episodes, today)).toEqual({ downloaded: 1, aired: 2, upcoming: 1 });
  });

  it("is empty for no episodes", () => {
    expect(summarizeCoverage([], today)).toEqual({ downloaded: 0, aired: 0, upcoming: 0 });
  });
});

describe("labels", () => {
  it("pill shows what you have over what has aired", () => {
    expect(coverageLabel({ downloaded: 11, aired: 11, upcoming: 9 })).toBe("11/11");
  });

  it.each([
    [{ downloaded: 11, aired: 11, upcoming: 9 }, "11 of 11 aired episodes in your library · 9 upcoming"],
    [{ downloaded: 3, aired: 10, upcoming: 0 }, "3 of 10 episodes in your library"],
    [{ downloaded: 1, aired: 1, upcoming: 1 }, "1 of 1 aired episode in your library · 1 upcoming"],
    [{ downloaded: 0, aired: 0, upcoming: 9 }, "Nothing has aired yet · 9 upcoming"],
    [{ downloaded: 0, aired: 0, upcoming: 0 }, "No episodes"],
  ])("sentence for %j", (coverage, expected) => {
    expect(coverageSentence(coverage)).toBe(expected);
  });

  it.each([
    [1, { downloaded: 11, aired: 11, upcoming: 0 }, "Season 1 · 11/11"],
    [2, { downloaded: 1, aired: 1, upcoming: 9 }, "Season 2 · 1/1 · 9 upcoming"],
    [3, { downloaded: 0, aired: 0, upcoming: 10 }, "Season 3 · 10 upcoming"],
    [0, { downloaded: 2, aired: 5, upcoming: 0 }, "Specials · 2/5"],
    [4, { downloaded: 0, aired: 0, upcoming: 0 }, "Season 4"],
  ])("tab label for season %i %j", (seasonNum, coverage, expected) => {
    expect(seasonTabLabel(seasonNum, coverage)).toBe(expected);
  });
});
