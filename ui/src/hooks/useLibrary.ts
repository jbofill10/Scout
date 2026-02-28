import { useQuery } from "@tanstack/react-query";

interface LibraryShow {
  tvdbId: string;
  title: string;
  thumb: string;
}

interface LibraryMovie {
  tvdbId: string;
  title: string;
  thumb: string;
  year: number;
}

interface EpisodeWithStatus {
  tvdbId: string;
  seasonNumber: number;
  episodeNumber: number;
  absoluteNumber?: number;
  name: string;
  aired: string;
  downloaded: boolean;
}

interface MetadataStatus {
  hasTvdbData: boolean;
  tvdbEpisodeCount: number;
  plexEpisodeCount: number;
  missingCount: number;
}

export const useLibraryShows = () => {
  return useQuery<LibraryShow[]>({
    queryKey: ["library", "shows"],
    queryFn: async () => {
      const response = await fetch("/api/library/shows");
      if (!response.ok) {
        throw new Error("Failed to fetch library shows");
      }
      return response.json();
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
};

export const useLibraryMovies = () => {
  return useQuery<LibraryMovie[]>({
    queryKey: ["library", "movies"],
    queryFn: async () => {
      const response = await fetch("/api/library/movies");
      if (!response.ok) {
        throw new Error("Failed to fetch library movies");
      }
      return response.json();
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
};

export const useShowEpisodes = (tvdbId: string | null) => {
  return useQuery<EpisodeWithStatus[]>({
    queryKey: ["library", "show", tvdbId, "episodes"],
    queryFn: async () => {
      if (!tvdbId) return [];
      const response = await fetch(`/api/library/shows/${tvdbId}`);
      if (!response.ok) {
        throw new Error("Failed to fetch show episodes");
      }
      return response.json();
    },
    enabled: !!tvdbId,
    staleTime: 2 * 60 * 1000, // 2 minutes
  });
};

export const useShowMetadataStatus = (tvdbId: string | null) => {
  return useQuery<MetadataStatus>({
    queryKey: ["library", "show", tvdbId, "metadata-status"],
    queryFn: async () => {
      if (!tvdbId) return { hasTvdbData: false, tvdbEpisodeCount: 0, plexEpisodeCount: 0, missingCount: 0 };
      const response = await fetch(`/api/library/shows/${tvdbId}/metadata-status`);
      if (!response.ok) {
        throw new Error("Failed to fetch metadata status");
      }
      return response.json();
    },
    enabled: !!tvdbId,
    staleTime: 30 * 1000, // 30 seconds
  });
};

export type { LibraryShow, LibraryMovie, EpisodeWithStatus, MetadataStatus };
