import React, { createContext, useContext } from 'react';
import type { ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';

/**
 * Genre data structure from the API
 */
export interface Genre {
  name: string;
  slug: string;
}

/**
 * Genre context value type
 */
interface GenreContextValue {
  genres: Genre[];
  isLoading: boolean;
  error: Error | null;
}

const GenreContext = createContext<GenreContextValue | undefined>(undefined);

/**
 * The 6 curated genres for Scout's Netflix-style UI
 */
const CURATED_GENRES = [
  'Action',
  'Comedy',
  'Drama',
  'Sci-Fi',
  'Anime',
  'Documentary',
];

/**
 * Maps genre names from the API to our curated genre names
 * Handles the "Science Fiction" → "Sci-Fi" mapping
 */
const mapGenreName = (apiGenreName: string): string => {
  if (apiGenreName === 'Science Fiction') {
    return 'Sci-Fi';
  }
  return apiGenreName;
};

/**
 * Fetches genres from the webserver API
 */
const fetchGenres = async (): Promise<Genre[]> => {
  const response = await fetch('/api/genres');

  if (!response.ok) {
    throw new Error(`Failed to fetch genres: ${response.statusText}`);
  }

  const allGenres: Genre[] = await response.json();

  // Filter to only our 6 curated genres and map names
  const filtered = allGenres
    .map((genre) => ({
      ...genre,
      name: mapGenreName(genre.name),
    }))
    .filter((genre) => CURATED_GENRES.includes(genre.name));

  return filtered;
};

/**
 * Genre Provider Component
 *
 * Fetches and provides genre data to the application.
 * Filters to 6 curated genres: Action, Comedy, Drama, Sci-Fi, Anime, Documentary
 */
export const GenreProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const { data, isLoading, error } = useQuery<Genre[], Error>({
    queryKey: ['genres'],
    queryFn: fetchGenres,
    staleTime: 60 * 60 * 1000, // Genres rarely change, keep fresh for 1 hour
    gcTime: 24 * 60 * 60 * 1000, // Cache for 24 hours
    retry: 2, // Retry failed requests twice
  });

  const value: GenreContextValue = {
    genres: data || [],
    isLoading,
    error: error || null,
  };

  return (
    <GenreContext.Provider value={value}>{children}</GenreContext.Provider>
  );
};

/**
 * Hook to access genre data
 *
 * @returns Genre context value with genres, loading state, and error
 * @throws Error if used outside GenreProvider
 */
// eslint-disable-next-line react-refresh/only-export-components
export const useGenres = (): GenreContextValue => {
  const context = useContext(GenreContext);

  if (context === undefined) {
    throw new Error('useGenres must be used within a GenreProvider');
  }

  return context;
};
