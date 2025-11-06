# Implementation Tasks

## 1. TVDB-Proxy Service (tvdb-proxy-maintainer agent)

- [x] 1.1 Update `getExtendedInformation()` to accept `mediaType` query parameter (main.go:330)
- [x] 1.2 Add conditional endpoint logic: `/movies/{id}/extended` for movies, `/series/{id}/extended` for series (main.go:333)
- [x] 1.3 Update `fetchTranslations()` to accept `mediaType` parameter (main.go:276)
- [x] 1.4 Add conditional translations endpoint: `/movies/{id}/translations/` vs `/series/{id}/translations/` (main.go:279)
- [x] 1.5 Update `queryShow()` to skip `querySeriesMetadata()` call for movies (main.go:151-166)
- [x] 1.6 Set empty metadata for movies: `Metadata.Episodes = []` (main.go:~160)
- [x] 1.7 Update anime detection to check "Animation" genre for movies (main.go:383-390)
- [x] 1.8 Update `getExtendedInformation()` to pass `mediaType` to `fetchTranslations()` calls (main.go:393-414)
- [x] 1.9 Run test-guardian to verify no regressions in series functionality

## 2. Webserver Service (webserver-maintainer agent)

- [x] 2.1 Update `TVDBProxyClient.GetExtendedInfo()` to accept and pass `mediaType` parameter (internal/clients/tvdb_client.go:51)
- [x] 2.2 Add `mediaType` query parameter to extended info URL (internal/clients/tvdb_client.go:52)
- [x] 2.3 Implement `DownloadMovie()` method in `DownloadInteractor` (internal/interactors/download_interactor.go)
  - [x] 2.3.1 Accept `tvdb.Media` with Category="movie"
  - [x] 2.3.2 Call `TVDBProxyClient.GetExtendedInfo()` with mediaType="movie" to get anime status
  - [x] 2.3.3 Parse release date from `media.Metadata.FirstAired`
  - [x] 2.3.4 If release date is future: call `repo.Schedule()` with movie data
  - [x] 2.3.5 If release date is past or parse fails: call `torrenterClient.Download()` immediately
  - [x] 2.3.6 Return appropriate success/error response
- [x] 2.4 Implement `DownloadMovie()` handler in `DownloadHandler` (internal/handlers/download_handler.go:48-55)
  - [x] 2.4.1 Extract context from request
  - [x] 2.4.2 Parse `tvdb.Media` from request body
  - [x] 2.4.3 Validate media.Category == "movie"
  - [x] 2.4.4 Call `interactor.DownloadMovie()`
  - [x] 2.4.5 Return JSON response with success/error
- [x] 2.5 Register `POST /movies` route to `DownloadMovie()` handler (cmd/webserver/main.go)
- [x] 2.6 Run test-guardian to verify download flow and scheduling

## 3. Torrenter Service (torrenter-maintainer agent)

- [x] 3.1 Remove movie-blocking guard in `HandleDownload()` (internal/service/torrent.go:93)
- [x] 3.2 Add `MovieExistsByTvdbId()` method to Repository interface (internal/repository/repository.go)
- [x] 3.3 Implement `MovieExistsByTvdbId()` in Postgres repository (internal/repository/postgres.go)
  - [x] 3.3.1 Query: `SELECT EXISTS(SELECT 1 FROM Movies WHERE tvdb_id = $1)`
  - [x] 3.3.2 Return boolean and error
- [x] 3.4 Update `HandleDownload()` to check movie existence before downloading (internal/service/torrent.go:~95)
  - [x] 3.4.1 For movies: call `repo.MovieExistsByTvdbId(req.Id)`
  - [x] 3.4.2 If exists: return nil (skip download)
  - [x] 3.4.3 If not exists: proceed to search
- [x] 3.5 Implement `createMovieSearchStrategy()` function (internal/service/torrent.go:~520)
  - [x] 3.5.1 Accept `*tvdb.Media` as parameter
  - [x] 3.5.2 Create query: "{MovieName} {Year}" or "{MovieName} ({Year})"
  - [x] 3.5.3 Set category to 2000 (movie category for Prowlarr)
  - [x] 3.5.4 Set indexers based on anime status:
    - Anime movies: `[]int{NYAA, 1337}` (both)
    - Regular movies: `[]int{1337}` (1337x only)
  - [x] 3.5.5 Return array of SearchStrategy with single movie query
- [x] 3.6 Update `HandleDownload()` to call movie search strategy (internal/service/torrent.go:~112)
  - [x] 3.6.1 Check if `req.Category == "movie"`
  - [x] 3.6.2 Call `createMovieSearchStrategy(req)` to get strategies
  - [x] 3.6.3 Use existing search/filter/download logic from shows
- [x] 3.7 Update `ProcessDownloadedTorrent()` if needed to verify movie path logic (internal/service/mediaprocessor.go:109-124)
- [x] 3.8 Run test-guardian to verify movie search and download flow

## 4. Documentation

- [x] 5.1 Update CLAUDE.md with movie download flow examples
- [x] 5.2 Document anime movie indexer logic in comments
- [x] 5.3 Add movie API endpoint to webserver documentation
- [x] 5.4 Note that MovieDownloadHistory is unused (future feature)

## 5. Validation

- [x] 6.1 Run `openspec validate add-movie-downloading --strict`
- [x] 6.2 Verify all tests pass with test-guardian
- [x] 6.3 Run golangci-lint on modified Go files
- [x] 6.4 Verify OpenTelemetry tracing works for movie downloads (requires runtime testing)
- [x] 6.5 Test with Docker Compose deployment (requires deployment)

## Notes

- Use tvdb-proxy-maintainer agent for section 1
- Use webserver-maintainer agent for section 2
- Use torrenter-maintainer agent for section 3
- Use test-guardian agent after each section completes
- Follow layered architecture: handlers → interactors → services → repositories
- Ensure proper context.Context propagation for observability
- Adhere to golangci-lint rules (120 char max line length)
