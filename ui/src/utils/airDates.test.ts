import { formatAirDate, hasAired, parseAirDate } from "./airDates";

const today = new Date(2026, 10, 4, 15, 30); // 2026-11-04, mid-afternoon

describe("parseAirDate", () => {
  it("parses a TVDB date as a local calendar day", () => {
    const date = parseAirDate("2025-07-11");
    expect(date?.getFullYear()).toBe(2025);
    expect(date?.getMonth()).toBe(6);
    expect(date?.getDate()).toBe(11);
  });

  it.each(["", undefined, "TBA", "2025-07", "2025-00-01"])("returns null for %j", (value) => {
    expect(parseAirDate(value)).toBeNull();
  });
});

describe("formatAirDate", () => {
  it("formats a date and returns null without one", () => {
    expect(formatAirDate("2025-07-11")).toMatch(/2025/);
    expect(formatAirDate("")).toBeNull();
  });
});

describe("hasAired", () => {
  it.each([
    ["last year", "2025-06-13", true],
    ["yesterday", "2026-11-03", true],
    ["today, even before the episode airs tonight", "2026-11-04", true],
    ["tomorrow", "2026-11-05", false],
    ["next season", "2027-02-01", false],
    ["blank date", "", false],
    ["unparseable date", "TBA", false],
  ])("%s", (_name, aired, expected) => {
    expect(hasAired(aired, today)).toBe(expected);
  });

  it("treats an unknown date as aired so library-only status still adds up", () => {
    expect(hasAired(undefined, today)).toBe(true);
  });
});
