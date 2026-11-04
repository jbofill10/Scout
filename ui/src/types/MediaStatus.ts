/**
 * Type definitions for media download status API
 * Matches backend contract in backend/pkg/media/status.go
 */

export interface StatusRequest {
  tvdbId: string;
  mediaType: 'show' | 'movie';
}

export interface EpisodeStatus {
  episodeNum: number;
  downloaded: boolean;
  tvdbId?: string;
}

export interface SeasonStatus {
  seasonNum: number;
  episodes: EpisodeStatus[];
}

export interface ShowStatus {
  tvdbId: string;
  seasons: SeasonStatus[];
}

export interface MovieStatus {
  tvdbId: string;
  inLibrary: boolean;
}

export interface StatusBatchResponse {
  shows: ShowStatus[];
  movies: MovieStatus[];
}

/**
 * Library coverage of a show. Only episodes that have aired count against the
 * library; upcoming is how many more TVDB lists that have not aired yet, so a
 * show mid-run reads 11/11 with 9 upcoming rather than 11/20.
 */
export interface EpisodeCoverage {
  downloaded: number;
  aired: number;
  upcoming: number;
}

/**
 * Helper type for status badge display on MediaCard
 */
export interface MediaStatusBadge {
  type: 'movie' | 'show';
  inLibrary?: boolean;
  episodeCount?: EpisodeCoverage;
}

/**
 * Request type for batch extended media info
 */
export interface ExtendedBatchRequest {
  id: string;
  mediaType: 'series' | 'movie';
}

/**
 * Extended media data from tvdb-proxy (subset needed for status checks)
 */
export interface ExtendedMediaData {
  id: number;
  name: string;
  type?: string;
}

/**
 * Response type for batch extended media info
 */
export interface ExtendedBatchResponse {
  data: ExtendedMediaData[];
}

/**
 * Media status information from enriched search endpoint
 * Matches backend contract in backend/pkg/media/status.go MediaStatusInfo
 */
export interface MediaStatusInfo {
  type: 'series' | 'movie';
  downloaded?: number;    // For shows: number of episodes downloaded
  total?: number;         // For shows: total number of episodes TVDB lists
  aired?: number;         // For shows: how many of total have aired (absent from older backends)
  inLibrary?: boolean;    // For movies: whether in library
  seasons?: SeasonStatus[];  // Detailed season/episode breakdown (if needed)
}

/**
 * One critic or audience score (tvdb.Rating). value is in the source's own
 * scale: 0-10 for imdb, 0-100 for rotten_tomatoes and metacritic.
 */
export interface MediaRating {
  source: string;
  value: number;
  votes?: number;
}

/**
 * A media record as returned by the backend (tvdb.Media).
 * Detail fields are optional because the popular/search list endpoints only
 * populate a subset; the details endpoint fills in the rest.
 */
export interface MediaRecord {
  id: string;
  image_url: string;
  mediaName: string;
  anime?: boolean;
  type?: string;
  overview?: string;
  tagline?: string;
  year?: string;
  status?: { name?: string };
  genres?: string[];
  runtime?: number;
  contentRating?: string;
  originalCountry?: string;
  originalLanguage?: string;
  score?: number;
  /** Director as TVDB's search index lists it (movies) */
  director?: string;
  /** Production studios (movies) */
  studios?: string[];
  /** Original network (series) */
  network?: string;
  /** Why a search returned this when it was not a title match, e.g. "Mia Goth · Actor" */
  matchedVia?: string;
  /** IMDb id, "tt1234567", when TVDB knows it */
  imdbId?: string;
  /** Critic and audience scores, when a rating source is configured and knows the title */
  ratings?: MediaRating[];
  metadata?: {
    firstAired?: string;
    lastAired?: string;
    episodes?: Array<{
      aired: string;
      id: number;
      image: string;
      name: string;
      number: number;
      seasonNumber: number;
      absoluteNumber?: number;
      overview?: string;
      runtime?: number;
    }>;
  };
}

/**
 * Full detail record from GET /api/media/:id/details
 */
export type MediaDetails = MediaRecord;

/**
 * Enriched media object with status already merged
 * From backend GET /api/search/enriched endpoint
 */
export interface EnrichedMedia {
  media: MediaRecord;
  status: MediaStatusInfo;
}
