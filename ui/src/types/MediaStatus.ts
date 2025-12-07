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
 * Helper type for status badge display on MediaCard
 */
export interface MediaStatusBadge {
  type: 'movie' | 'show';
  inLibrary?: boolean;
  episodeCount?: { downloaded: number; total: number };
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
  total?: number;         // For shows: total number of episodes
  inLibrary?: boolean;    // For movies: whether in library
  seasons?: SeasonStatus[];  // Detailed season/episode breakdown (if needed)
}

/**
 * Enriched media object with status already merged
 * From backend GET /api/search/enriched endpoint
 */
export interface EnrichedMedia {
  media: {
    id: string;
    image_url: string;
    mediaName: string;
    metadata?: {
      episodes?: Array<{
        aired: string;
        id: number;
        image: string;
        name: string;
        number: number;
        seasonNumber: number;
      }>;
    };
  };
  status: MediaStatusInfo;
}
