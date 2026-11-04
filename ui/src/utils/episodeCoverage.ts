import type { EpisodeCoverage } from "../types/MediaStatus";
import { hasAired, parseAirDate } from "./airDates";

export type { EpisodeCoverage };

/** The two things coverage needs to know about an episode. */
export interface CoverableEpisode {
  downloaded: boolean;
  /** TVDB air date, "" when TVDB has none, undefined when unknown to the caller */
  aired?: string;
}

/**
 * What one episode row should say:
 * - library: in Plex
 * - missing: has aired and is not in Plex
 * - upcoming: has an air date in the future
 * - tba: TVDB lists it without a usable air date
 */
export type EpisodeState = "library" | "missing" | "upcoming" | "tba";

export const episodeState = (episode: CoverableEpisode, today?: Date): EpisodeState => {
  if (episode.downloaded) return "library";
  if (episode.aired === undefined) return "missing";
  if (!parseAirDate(episode.aired)) return "tba";
  return hasAired(episode.aired, today) ? "missing" : "upcoming";
};

export const episodeStateLabel: Record<EpisodeState, string> = {
  library: "In library",
  missing: "Missing",
  upcoming: "Upcoming",
  tba: "TBA",
};

/**
 * Library coverage of a list of episodes. Only aired episodes count against
 * the library; the rest are reported as upcoming. An episode in the library
 * has aired whatever its date says, so the count never reads 12 of 11.
 */
export const summarizeCoverage = (episodes: CoverableEpisode[], today?: Date): EpisodeCoverage => {
  let downloaded = 0;
  let aired = 0;
  let upcoming = 0;
  for (const episode of episodes) {
    const state = episodeState(episode, today);
    if (state === "library") {
      downloaded++;
      aired++;
    } else if (state === "missing") {
      aired++;
    } else {
      upcoming++;
    }
  }
  return { downloaded, aired, upcoming };
};

/**
 * The poster pill. "Caught up" when everything aired is in and more is still
 * to air; otherwise what you have over what has aired ("9/11", or "11/11" for
 * a show with nothing more announced).
 */
export const coverageLabel = (coverage: EpisodeCoverage): string => {
  if (coverage.aired > 0 && coverage.downloaded >= coverage.aired && coverage.upcoming > 0) {
    return "Caught up";
  }
  return `${coverage.downloaded}/${coverage.aired}`;
};

const plural = (count: number, noun: string): string => `${count} ${noun}${count === 1 ? "" : "s"}`;

/**
 * One line for the dialog header and the pill's tooltip, e.g.
 * "11 of 11 aired episodes in your library · 9 upcoming".
 */
export const coverageSentence = (coverage: EpisodeCoverage): string => {
  if (coverage.aired === 0) {
    return coverage.upcoming > 0 ? `Nothing has aired yet · ${coverage.upcoming} upcoming` : "No episodes";
  }
  const noun = coverage.upcoming > 0 ? "aired episode" : "episode";
  const base = `${coverage.downloaded} of ${plural(coverage.aired, noun)} in your library`;
  return coverage.upcoming > 0 ? `${base} · ${coverage.upcoming} upcoming` : base;
};

export const seasonName = (seasonNum: number): string =>
  seasonNum === 0 ? "Specials" : `Season ${seasonNum}`;

/** A season tab: "Season 2 · 1/1 · 9 upcoming", or "Season 3 · 10 upcoming" before any of it airs. */
export const seasonTabLabel = (seasonNum: number, coverage: EpisodeCoverage): string => {
  const name = seasonName(seasonNum);
  if (coverage.aired === 0) {
    return coverage.upcoming > 0 ? `${name} · ${coverage.upcoming} upcoming` : name;
  }
  const label = `${name} · ${coverage.downloaded}/${coverage.aired}`;
  return coverage.upcoming > 0 ? `${label} · ${coverage.upcoming} upcoming` : label;
};
