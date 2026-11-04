# Show a rating on every media card

## Why

The Home, Shows and Movies rows and the search results show posters with a library badge, but nothing about
whether a title is any good. TVDB carries no critic or audience score (its `score` is a lifetime popularity
figure), so picking what to download means leaving Scout to look a title up. The user asked for the rating on
each card: the Tomatometer for movies and "something significant" for shows.

## What Changes

- tvdb-proxy gains a rating source (`internal/tvdbproxy/ratings`): OMDb, the only API that licenses the
  Rotten Tomatoes Tomatometer, which also returns IMDb's rating and the Metascore. Lookups are by IMDb id,
  which TVDB already lists on search hits (`remote_ids`, now decoded) and extended records.
- Search hits, popular rows and extended records carry `ratings` (`[{source, value, votes}]`) when
  `OMDB_API_KEY` is set. Answers are cached for a week, misses for a day, a refused key pauses lookups for an
  hour, and a caller waits at most 4 seconds for an answer not cached yet (the lookup finishes detached).
- The webserver's extended-record conversion carries `ratings` and `imdbId` into `Media`, so the detail
  endpoint and the enriched popular rows have them too.
- Every poster card shows one score in its top-left corner: the Tomatometer for movies (IMDb when Rotten
  Tomatoes has not scored it), the IMDb rating for shows. The detail dialog lists every score.
- New env `OMDB_API_KEY` (optional; `OMDB_HOST` for tests). Without it nothing changes.

## Impact

- Backend: `pkg/media` (`Rating`, `Media.Ratings`, `TVDBSearchItem.RemoteIds`,
  `TVDBSeriesExtendedData.Ratings`), `internal/tvdbproxy/ratings` (new), `internal/tvdbproxy/handlers`
  (search, popular and extended handlers take a `Rater`), `internal/tvdbproxy/config`, `cmd/tvdb-proxy`,
  `internal/webserver/clients` (`ExtendedToMedia`).
- UI: `utils/ratings.ts` (new), `components/ui/RatingPill.tsx` (new), `MediaCard`, `GenreRow`,
  `SearchResultsGrid`, `MediaStatusDialog`, `types/MediaStatus.ts`.
- Config: `.env.template`, `docker-compose.yml.template`, README, DEPLOYMENT.
- External traffic: one OMDb lookup per title the proxy has not seen in a week. A free key allows 1,000 a day;
  the warm popular rows need roughly 250 on a cold start.
- Specs: `tvdb-proxy-api` (ratings enrichment, search hit mapping), `ui-netflix-layout` (media card).

## Approval

Requested as a `/goal` on 2026-11-03 ("for all media (tv and movies) i want it to show the rating on each
card. for example movies and the tomatometer and shows something significant"); implemented on that basis.
