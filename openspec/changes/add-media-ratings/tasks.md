# Tasks

## 1. Shared types

- [x] 1.1 `pkg/media`: `Rating` and the source constants, `Media.Ratings`, `TVDBSearchItem.RemoteIds` +
  `ImdbID()`, `TVDBSeriesExtendedData.Ratings`; shared `imdbIDFrom`

## 2. tvdb-proxy

- [x] 2.1 `internal/tvdbproxy/ratings`: OMDb source (`Lookup` parsing the Ratings list with top-level
  fallbacks, `ErrRateLimited`/`ErrUnauthorized`), caching `Service` (`Attach`, `Lookup`, in-flight sharing,
  pause, wait budget), with tests
- [x] 2.2 `search/convert.go` maps the hit's IMDb id; test
- [x] 2.3 Handlers take a `Rater`: search and popular attach to their results, extended (single and batch)
  rates the record; tests
- [x] 2.4 Config `OMDB_API_KEY` / `OMDB_HOST`; `main.go` wires the service when a key is set

## 3. Webserver

- [x] 3.1 `ExtendedToMedia` carries `ImdbID` and `Ratings`; test

## 4. UI

- [x] 4.1 `types/MediaStatus.ts`: `MediaRating`, `MediaRecord.ratings`/`imdbId`; `SearchResult.ratings`
- [x] 4.2 `utils/ratings.ts`: `formatRating`, `orderedRatings`, `pickCardRating` (Tomatometer first for
  movies, IMDb first for series), with tests
- [x] 4.3 `RatingPill` component; `MediaCard` `rating` prop rendered top-left, phone insets; test
- [x] 4.4 `GenreRow` and `SearchResultsGrid` pass the card rating; `MediaStatusDialog` lists every score

## 5. Config and docs

- [x] 5.1 `.env.template`, `docker-compose.yml.template`, README, DEPLOYMENT, CLAUDE.md

## 6. Verification

- [x] 6.1 `go build ./...`, `go vet`, `gofmt`, `go test` for `internal/tvdbproxy/...`, `internal/webserver/...`,
  `pkg/...` (race detector on the new package and handlers)
- [x] 6.2 `npm run lint`, `tsc -b --noEmit`, `npm test`, `npm run build`
