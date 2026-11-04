# Tasks

## 1. Shared types

- [x] 1.1 `pkg/media`: `Rating` and the source constants, `Media.Ratings`, `TVDBSearchItem.RemoteIds` +
  `ImdbID()`, `TVDBSeriesExtendedData.Ratings`; shared `imdbIDFrom`

## 2. tvdb-proxy

- [x] 2.1 `internal/tvdbproxy/ratings`: OMDb source (`Lookup` parsing the Ratings list with top-level
  fallbacks, `ErrRateLimited`/`ErrUnauthorized`), IMDb dataset table (`Dataset`: download, parse, sorted
  rows, daily refresh), TVDB id resolver (`Resolver`, short extended record), `asyncCache`, and the composed
  `Service` (`Attach` resolving ids first, `Lookup` merging table and source, pause, wait budget), with tests
- [x] 2.2 `search/convert.go` maps the hit's IMDb id; test
- [x] 2.3 Handlers take a `Rater`: search and popular attach to their results, extended (single and batch)
  rates the record; tests
- [x] 2.4 Config `RATINGS_IMDB` / `IMDB_RATINGS_URL` / `OMDB_API_KEY` / `OMDB_HOST`; `main.go` loads the table
  before serving and wires the service
- [x] 2.5 `POST /ratings/batch` handler rating by TVDB id; tests

## 3. Webserver

- [x] 3.1 `ExtendedToMedia` carries `ImdbID` and `Ratings`; test
- [x] 3.2 `GetRatingsBatch` client and `POST /api/ratings/batch` relay; tests

## 4. UI

- [x] 4.1 `types/MediaStatus.ts`: `MediaRating`, `MediaRecord.ratings`/`imdbId`; `SearchResult.ratings`
- [x] 4.2 `utils/ratings.ts`: `formatRating`, `orderedRatings`, `pickCardRating` (Tomatometer first for
  movies, IMDb first for series), with tests
- [x] 4.3 `RatingPill` component; `MediaCard` `rating` prop rendered top-left, phone insets; test
- [x] 4.4 `GenreRow` and `SearchResultsGrid` pass the card rating; `MediaStatusDialog` lists every score
- [x] 4.5 `useRatingsBatch` hook (chunked, cached) with tests; Library page cards show their score

## 5. Config and docs

- [x] 5.1 `.env.template`, `docker-compose.yml.template`, README, DEPLOYMENT, CLAUDE.md

## 6. Verification

- [x] 6.1 `go build ./...`, `go vet`, `gofmt`, `go test` for `internal/tvdbproxy/...`, `internal/webserver/...`,
  `pkg/...` (race detector on the new package and handlers)
- [x] 6.2 `npm run lint`, `tsc -b --noEmit`, `npm test`, `npm run build`
- [x] 6.3 Deployed; ratings seen on the Home rows, a title search, an actor search and the Library page
