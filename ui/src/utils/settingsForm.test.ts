import { durationError, durationToSeconds, invalidLanguage, parseLanguages, tidyDuration } from "./settingsForm";

describe("parseLanguages", () => {
  it.each([
    ["en", ["en"]],
    ["en, ES,,fr", ["en", "es", "fr"]],
    ["en en", ["en"]],
    ["", []],
    ["  ,  ", []],
  ])("%j -> %j", (input, expected) => {
    expect(parseLanguages(input)).toEqual(expected);
  });

  it("flags codes that are not two or three letters", () => {
    expect(invalidLanguage(["en", "spa"])).toBeNull();
    expect(invalidLanguage(["en", "english"])).toBe("english");
    expect(invalidLanguage(["e1"])).toBe("e1");
  });
});

describe("durations", () => {
  it.each([
    ["2m", 120],
    ["1h30m", 5400],
    ["0s", 0],
    ["500ms", 0.5],
    ["2m0s", 120],
  ])("%s is %d seconds", (value, seconds) => {
    expect(durationToSeconds(value)).toBeCloseTo(seconds);
  });

  it("rejects garbage", () => {
    expect(durationToSeconds("soon")).toBeNaN();
  });

  it("explains what is wrong", () => {
    expect(durationError("")).toMatch(/Enter a duration/);
    expect(durationError("soon")).toMatch(/Use a duration/);
    expect(durationError("0s")).toBe("Must be more than 0");
    expect(durationError("0s", { allowZero: true })).toBeNull();
    expect(durationError("30s", { min: "1m" })).toBe("Must be at least 1m");
    expect(durationError("2m")).toBeNull();
    expect(durationError("1h30m", { min: "1m" })).toBeNull();
  });

  it("tidies what the server echoes back", () => {
    expect(tidyDuration("2m0s")).toBe("2m");
    expect(tidyDuration("30m0s")).toBe("30m");
    expect(tidyDuration("1h0m0s")).toBe("1h");
    expect(tidyDuration("0s", "off")).toBe("off");
    expect(tidyDuration("")).toBe("");
  });
});
