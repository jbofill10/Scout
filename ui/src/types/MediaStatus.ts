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
