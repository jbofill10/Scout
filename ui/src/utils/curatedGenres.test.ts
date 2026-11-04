import { CURATED_GENRES, curatedGenres, mapGenreName } from './curatedGenres';

const tvdbGenres = [
  { name: 'Soap', slug: 'soap' },
  { name: 'Science Fiction', slug: 'science-fiction' },
  { name: 'Horror', slug: 'horror' },
  { name: 'Drama', slug: 'drama' },
  { name: 'Documentary', slug: 'documentary' },
  { name: 'Comedy', slug: 'comedy' },
  { name: 'Action', slug: 'action' },
  { name: 'Thriller', slug: 'thriller' },
  { name: 'Anime', slug: 'anime' },
  { name: 'Western', slug: 'western' },
];

describe('curatedGenres', () => {
  it('includes Horror and Thriller in the curated list', () => {
    expect(CURATED_GENRES).toContain('Horror');
    expect(CURATED_GENRES).toContain('Thriller');
    expect(CURATED_GENRES).toHaveLength(8);
  });

  it('keeps the curated genres in row order, renamed for display', () => {
    expect(curatedGenres(tvdbGenres)).toEqual([
      { name: 'Action', slug: 'action' },
      { name: 'Comedy', slug: 'comedy' },
      { name: 'Drama', slug: 'drama' },
      { name: 'Sci-Fi', slug: 'science-fiction' },
      { name: 'Horror', slug: 'horror' },
      { name: 'Thriller', slug: 'thriller' },
      { name: 'Anime', slug: 'anime' },
      { name: 'Documentary', slug: 'documentary' },
    ]);
  });

  it('leaves out genres TVDB does not return', () => {
    const names = curatedGenres(tvdbGenres.filter((g) => g.name !== 'Thriller')).map((g) => g.name);
    expect(names).not.toContain('Thriller');
    expect(names).toHaveLength(7);
  });

  it('returns nothing for an empty genre list', () => {
    expect(curatedGenres([])).toEqual([]);
  });

  it('maps Science Fiction to Sci-Fi and leaves other names alone', () => {
    expect(mapGenreName('Science Fiction')).toBe('Sci-Fi');
    expect(mapGenreName('Horror')).toBe('Horror');
  });
});
