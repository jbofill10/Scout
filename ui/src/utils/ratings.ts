import type { MediaRating } from "../types/MediaStatus";

export type RatingSource = "imdb" | "rotten_tomatoes" | "metacritic";

/** One score ready to show: which source it is, its text and how it reads. */
export interface RatingBadgeModel {
  source: RatingSource;
  /** "97%", "8.7" or "84" */
  label: string;
  /** Accessible name, e.g. "Tomatometer 97%" */
  title: string;
  /** Rotten Tomatoes only: fresh (60% and up) or rotten */
  fresh?: boolean;
}

/** A Tomatometer score of 60% or more is "fresh". */
export const FRESH_THRESHOLD = 60;

/**
 * Which score leads on a card. Movies are judged by the Tomatometer where
 * there is one; TV shows by IMDb, whose user rating is the figure people quote
 * for series and which Rotten Tomatoes rarely scores.
 */
const SOURCE_ORDER: Record<"movie" | "series", RatingSource[]> = {
  movie: ["rotten_tomatoes", "imdb", "metacritic"],
  series: ["imdb", "rotten_tomatoes", "metacritic"],
};

const SOURCE_NAMES: Record<RatingSource, string> = {
  imdb: "IMDb",
  rotten_tomatoes: "Tomatometer",
  metacritic: "Metascore",
};

const isKnownSource = (source: string): source is RatingSource => source in SOURCE_NAMES;

/** Formats one rating in its source's own style; null for unknown sources. */
export function formatRating(rating: MediaRating): RatingBadgeModel | null {
  if (!isKnownSource(rating.source) || !Number.isFinite(rating.value)) return null;
  const name = SOURCE_NAMES[rating.source];
  switch (rating.source) {
    case "rotten_tomatoes": {
      const label = `${Math.round(rating.value)}%`;
      return { source: rating.source, label, title: `${name} ${label}`, fresh: rating.value >= FRESH_THRESHOLD };
    }
    case "imdb": {
      const label = rating.value.toFixed(1);
      return { source: rating.source, label, title: `${name} ${label}` };
    }
    case "metacritic": {
      const label = `${Math.round(rating.value)}`;
      return { source: rating.source, label, title: `${name} ${label}` };
    }
  }
}

/**
 * Every known score, in the order the media type prefers (see SOURCE_ORDER).
 * Anything that is not a movie counts as a series.
 */
export function orderedRatings(
  ratings: MediaRating[] | undefined,
  mediaType: string | undefined,
): RatingBadgeModel[] {
  if (!ratings?.length) return [];
  const order = SOURCE_ORDER[mediaType === "movie" ? "movie" : "series"];
  const bySource = new Map<RatingSource, RatingBadgeModel>();
  for (const rating of ratings) {
    const model = formatRating(rating);
    if (model && !bySource.has(model.source)) bySource.set(model.source, model);
  }
  return order.flatMap((source) => bySource.get(source) ?? []);
}

/** The one score a poster card shows, or null when the title has none. */
export function pickCardRating(
  ratings: MediaRating[] | undefined,
  mediaType: string | undefined,
): RatingBadgeModel | null {
  return orderedRatings(ratings, mediaType)[0] ?? null;
}
