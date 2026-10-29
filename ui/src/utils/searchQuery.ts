import type { SearchMediaType } from "../hooks/useEnrichedSearch";

/**
 * One constraint the backend read out of the search text, e.g. a year typed
 * after the title or an `actor:` filter. Mirrors pkg/search.Filter.
 */
export interface SearchFilter {
  key: string;
  value: string;
  /** True when the filter was inferred from the title text ("X 2022"). */
  inferred?: boolean;
}

/**
 * How the backend interpreted the search text. Mirrors pkg/search.Query.
 */
export interface SearchQuery {
  raw: string;
  title: string;
  year?: string;
  director?: string;
  person?: string;
  personRole?: string;
  company?: string;
  country?: string;
  language?: string;
  imdbId?: string;
  filters: SearchFilter[];
}

/** Human labels for the canonical filter keys. */
export const FILTER_LABELS: Record<string, string> = {
  year: "Year",
  director: "Director",
  actor: "Actor",
  person: "Person",
  studio: "Studio",
  network: "Network",
  country: "Country",
  language: "Language",
  imdb: "IMDb",
};

export const filterLabel = (key: string): string => FILTER_LABELS[key] ?? key;

/** Filters that can be searched on their own, without a title. */
const STANDALONE_KEYS = new Set(["actor", "person", "director", "imdb"]);

/**
 * True when the text is only filters that need a title to go with them
 * (a bare year or studio, say), so the backend had nothing to search.
 */
export const needsTitle = (query: SearchQuery): boolean =>
  query.title === "" &&
  query.filters.length > 0 &&
  !query.filters.some((filter) => STANDALONE_KEYS.has(filter.key));

const quoteIfNeeded = (value: string): string =>
  /\s/.test(value) ? `"${value}"` : value;

/**
 * Rebuilds search text from the interpretation with one filter removed, so a
 * chip's delete button edits the query the way the user would have.
 */
export const withoutFilter = (query: SearchQuery, removed: SearchFilter): string => {
  const parts: string[] = [];
  if (query.title) parts.push(query.title);
  for (const filter of query.filters) {
    if (filter === removed) continue;
    parts.push(`${filter.key}:${quoteIfNeeded(filter.value)}`);
  }
  return parts.join(" ");
};

export interface SearchExample {
  text: string;
  hint: string;
  mediaType: SearchMediaType;
}

/** Shown on the empty search screen; clicking one fills the box. */
export const SEARCH_EXAMPLES: SearchExample[] = [
  { text: "X (2022)", hint: "a title and its year", mediaType: "movie" },
  { text: "actor:Mia Goth", hint: "everything someone has starred in", mediaType: "movie" },
  { text: "director:Ti West", hint: "a director's films", mediaType: "movie" },
  { text: "Succession network:HBO", hint: "a show on a network", mediaType: "series" },
  { text: "X studio:A24", hint: "a title from a studio", mediaType: "movie" },
  { text: "tt13560574", hint: "an IMDb id or link", mediaType: "movie" },
];
