import { describe, expect, it } from "vitest";
import { formatRating, orderedRatings, pickCardRating } from "./ratings";

const sinners = [
  { source: "imdb", value: 7.7, votes: 151234 },
  { source: "rotten_tomatoes", value: 97 },
  { source: "metacritic", value: 84 },
];

describe("formatRating", () => {
  it("formats each source in its own style", () => {
    expect(formatRating({ source: "rotten_tomatoes", value: 97 })).toEqual({
      source: "rotten_tomatoes",
      label: "97%",
      title: "Tomatometer 97%",
      fresh: true,
    });
    expect(formatRating({ source: "imdb", value: 8 })).toEqual({ source: "imdb", label: "8.0", title: "IMDb 8.0" });
    expect(formatRating({ source: "metacritic", value: 84 })).toEqual({
      source: "metacritic",
      label: "84",
      title: "Metascore 84",
    });
  });

  it("marks a Tomatometer under 60 as rotten", () => {
    expect(formatRating({ source: "rotten_tomatoes", value: 59 })?.fresh).toBe(false);
    expect(formatRating({ source: "rotten_tomatoes", value: 60 })?.fresh).toBe(true);
  });

  it("ignores sources it does not know", () => {
    expect(formatRating({ source: "letterboxd", value: 4 })).toBeNull();
  });
});

describe("pickCardRating", () => {
  it("leads with the Tomatometer for movies", () => {
    expect(pickCardRating(sinners, "movie")?.title).toBe("Tomatometer 97%");
  });

  it("leads with IMDb for series", () => {
    expect(pickCardRating(sinners, "series")?.title).toBe("IMDb 7.7");
  });

  it("falls back to IMDb when a movie has no Tomatometer", () => {
    expect(pickCardRating([{ source: "imdb", value: 6.4 }], "movie")?.title).toBe("IMDb 6.4");
  });

  it("treats an unknown media type as a series", () => {
    expect(pickCardRating(sinners, undefined)?.source).toBe("imdb");
  });

  it("is null without ratings", () => {
    expect(pickCardRating(undefined, "movie")).toBeNull();
    expect(pickCardRating([], "movie")).toBeNull();
    expect(pickCardRating([{ source: "letterboxd", value: 4 }], "movie")).toBeNull();
  });
});

describe("orderedRatings", () => {
  it("lists every known score in the type's order, one per source", () => {
    const labels = orderedRatings([...sinners, { source: "imdb", value: 9 }], "movie").map((r) => r.label);
    expect(labels).toEqual(["97%", "7.7", "84"]);
  });
});
