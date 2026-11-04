import React, { createContext, useContext } from 'react';
import type { ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { curatedGenres } from '../utils/curatedGenres';
import type { Genre } from '../utils/curatedGenres';

export type { Genre };

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
 * Fetches genres from the webserver API and keeps the curated ones, in row order
 */
const fetchGenres = async (): Promise<Genre[]> => {
  const response = await fetch('/api/genres');

  if (!response.ok) {
    throw new Error(`Failed to fetch genres: ${response.statusText}`);
  }

  const allGenres: Genre[] = await response.json();
  return curatedGenres(allGenres);
};

/**
 * Genre Provider Component
 *
 * Fetches and provides genre data to the application.
 * Filters to the curated genres in `utils/curatedGenres.ts`
 * (Action, Comedy, Drama, Sci-Fi, Horror, Thriller, Anime, Documentary)
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
