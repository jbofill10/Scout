/**
 * Genre record as the webserver returns it from `/api/genres`.
 */
export interface Genre {
  name: string;
  slug: string;
}

/**
 * The curated genres shown as rows on the Shows and Movies pages, in row order.
 */
export const CURATED_GENRES = [
  'Action',
  'Comedy',
  'Drama',
  'Sci-Fi',
  'Horror',
  'Thriller',
  'Anime',
  'Documentary',
] as const;

/**
 * Maps a TVDB genre name to the name the UI shows ("Science Fiction" → "Sci-Fi").
 */
export const mapGenreName = (apiGenreName: string): string =>
  apiGenreName === 'Science Fiction' ? 'Sci-Fi' : apiGenreName;

/**
 * Picks the curated genres out of the full TVDB list, renamed for display and
 * in CURATED_GENRES order. Genres TVDB does not return are left out.
 */
export const curatedGenres = (all: Genre[]): Genre[] => {
  const byName = new Map(all.map((genre) => [mapGenreName(genre.name), genre]));
  return CURATED_GENRES.flatMap((name) => {
    const genre = byName.get(name);
    return genre ? [{ ...genre, name }] : [];
  });
};
