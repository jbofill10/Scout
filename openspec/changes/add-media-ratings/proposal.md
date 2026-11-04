# Show a rating on every media card

## Why

The Home, Shows and Movies rows and the search results show posters with a library badge, but nothing about
whether a title is any good. TVDB carries no critic or audience score (its `score` is a lifetime popularity
figure), so picking what to download means leaving Scout to look a title up. The user asked for the rating on
each card: the Tomatometer for movies and "something significant" for shows.

## What Changes

- tvdb-proxy gains a ratings package (`internal/tvdbproxy/ratings`) with three sources. IMDb's public daily
  ratings table (`title.ratings.tsv.gz`, no key) is held in memory and gives every title an IMDb rating.
  Wikidata's recorded review scores (CC0, no key) give the Rotten Tomatoes Tomatometer and Metascore for
  established titles. OMDb, the only API that licenses a current Tomatometer, adds the Tomatometer and
  Metascore for everything when `OMDB_API_KEY` is set, winning over Wikidata's figure. All are keyed by IMDb
  id, which TVDB lists on search hits (`remote_ids`, now decoded) and extended records; anything that arrives
  with only a TVDB id (library items, person-credit search hits) is resolved through TVDB's short extended
  record.
- Search hits, popular rows and extended records carry `ratings` (`[{source, value, votes}]`). A new
  `POST /ratings/batch` rates titles by TVDB id; the webserver relays it at `/api/ratings/batch`. OMDb
  answers are cached for a week, misses for a day, a refused key pauses lookups for an hour, and a caller
  waits at most 4 seconds for an answer not cached yet (the lookup finishes detached).
- The webserver's extended-record conversion carries `ratings` and `imdbId` into `Media`, so the detail
  endpoint and the enriched popular rows have them too.
- Every poster card (Home, Shows, Movies, search results, Library) shows one score in its top-left corner:
  the Tomatometer for movies when a source has it, otherwise the IMDb rating; the IMDb rating for shows. The
  detail dialog lists every score.
- New env `RATINGS_IMDB` and `RATINGS_WIKIDATA` (default true; `IMDB_RATINGS_URL` and `WIKIDATA_SPARQL_URL`
  for tests) and `OMDB_API_KEY` (optional; `OMDB_HOST` for tests).

## Impact

- Backend: `pkg/media` (`Rating`, `Media.Ratings`, `TVDBSearchItem.RemoteIds`,
  `TVDBSeriesExtendedData.Ratings`), `internal/tvdbproxy/ratings` (new: dataset, OMDb source, resolver,
  cache, service), `internal/tvdbproxy/handlers` (search, popular and extended handlers take a `Rater`;
  new ratings batch handler), `internal/tvdbproxy/config`, `cmd/tvdb-proxy`, `internal/webserver/clients`
  (`ExtendedToMedia`, `GetRatingsBatch`), `internal/webserver/handlers` (ratings relay), `cmd/webserver`.
- UI: `utils/ratings.ts` (new), `components/ui/RatingPill.tsx` (new), `hooks/useRatings.ts` (new),
  `MediaCard`, `GenreRow`, `SearchResultsGrid`, `MediaStatusDialog`, `pages/Library.tsx`,
  `types/MediaStatus.ts`.
- Config: `.env.template`, `docker-compose.yml.template`, README, DEPLOYMENT.
- External traffic: a 9 MB download from IMDb at startup and once a day; about 20 MB of memory for the
  table. One Wikidata SPARQL query per 50 titles the proxy has not seen in a week (two in flight at most).
  With an OMDb key, one lookup per title the proxy has not seen in a week (a free key allows 1,000 a day;
  the warm popular rows need roughly 250 on a cold start). One short TVDB extended call per title that
  arrives without an IMDb id, cached for a week.
- Specs: `tvdb-proxy-api` (ratings enrichment, ratings batch, search hit mapping), `ui-netflix-layout`
  (media card), `ui-pages` (library page).

## Approval

Requested as a `/goal` on 2026-11-03 ("for all media (tv and movies) i want it to show the rating on each
card. for example movies and the tomatometer and shows something significant"); implemented on that basis.
