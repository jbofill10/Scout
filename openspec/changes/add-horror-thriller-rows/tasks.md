# Tasks

## 1. UI

- [x] 1.1 `utils/curatedGenres.ts`: `CURATED_GENRES` with Horror and Thriller, `mapGenreName`, `curatedGenres()` (filter + row order), with tests
- [x] 1.2 `GenreContext` uses `curatedGenres()`; `Shows.tsx` and `Movies.tsx` drop their copies of the list and render the context's genres

## 2. Backend

- [x] 2.1 `popular.DefaultRows` warms Horror and Thriller rows for series and movies; test counts updated
- [x] 2.2 Webserver `PopularRefresher` categories include Horror and Thriller; test counts updated
- [x] 2.3 `TraktGenreSlug` test covers the two new names

## 3. Verification

- [x] 3.1 `go test` for `internal/tvdbproxy/popular` and `internal/webserver/cache`, `go vet`, `gofmt`
- [x] 3.2 `npm test`, `npm run lint`, `npm run build`
