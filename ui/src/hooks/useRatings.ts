import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import type { MediaRating } from "../types/MediaStatus";

/**
 * One title to rate, by TVDB id. Title and year are optional and let the
 * backend find a film's Wikipedia article when Wikidata has no item for it.
 */
export interface RatingsItem {
  id: string;
  mediaType: "series" | "movie";
  title?: string;
  year?: string;
}

/** One row of POST /api/ratings/batch. */
export interface RatingsBatchRow {
  id: string;
  mediaType: string;
  imdbId?: string;
  ratings?: MediaRating[];
}

/**
 * Titles per request. The proxy caps a batch at 200 and resolves each title's
 * IMDb id on first sight, so smaller chunks get the first posters their score
 * while the rest are still being looked up.
 */
export const RATINGS_CHUNK_SIZE = 40;

export const chunkRatingsItems = (items: RatingsItem[], size = RATINGS_CHUNK_SIZE): RatingsItem[][] => {
  const chunks: RatingsItem[][] = [];
  for (let start = 0; start < items.length; start += size) {
    chunks.push(items.slice(start, start + size));
  }
  return chunks;
};

async function fetchRatingsBatch(items: RatingsItem[]): Promise<RatingsBatchRow[]> {
  const response = await fetch("/api/ratings/batch", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(items),
  });
  if (!response.ok) {
    throw new Error(`Failed to fetch ratings: ${response.statusText}`);
  }
  return response.json();
}

/** Folds every chunk that has answered into one id → ratings map. */
const combineRatings = (results: UseQueryResult<RatingsBatchRow[]>[]): Map<string, MediaRating[]> => {
  const byId = new Map<string, MediaRating[]>();
  for (const result of results) {
    for (const row of result.data ?? []) {
      if (row.ratings?.length) byId.set(row.id, row.ratings);
    }
  }
  return byId;
};

/**
 * Ratings for titles that only have a TVDB id, such as the Plex library.
 * Returns a map from TVDB id to the title's scores; a title the backend has
 * no score for is simply absent. Each chunk is cached for an hour.
 */
export function useRatingsBatch(items: RatingsItem[], enabled = true): Map<string, MediaRating[]> {
  const chunks = useMemo(() => chunkRatingsItems(items), [items]);
  return useQueries({
    queries: chunks.map((chunk) => ({
      queryKey: ["ratings", chunk.map((item) => `${item.mediaType}:${item.id}`)],
      queryFn: () => fetchRatingsBatch(chunk),
      staleTime: 60 * 60 * 1000,
      enabled,
    })),
    combine: combineRatings,
  });
}
