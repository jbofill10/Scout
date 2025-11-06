# Change: Add Movie Downloading Support

## Why

Scout currently has partial movie support with significant gaps in implementation. While users can search for movies via the TVDB API, the download flow is incomplete and non-functional. Movies were initially implemented alongside shows but were paused to focus on the TV show download workflow. Now that show downloads are mature, it's time to bring movies to feature parity.

The current codebase has a show-centric architecture, but most of the infrastructure (Plex sync, database tables, search endpoints) already exists for movies. This change will complete the movie support by implementing the missing download orchestration logic across all three backend services.

## What Changes

- **tvdb-proxy**: Update TVDB API integration to handle movie-specific endpoints (`/movies/{id}/extended` vs `/series/{id}/extended`), skip episode metadata fetching for movies, and enhance anime detection for animated movies
- **webserver**: Implement movie download handler and interactor with support for future release scheduling, similar to how episodes are scheduled
- **torrenter**: Implement movie search strategies, add movie existence checking against Plex library, and configure indexer selection based on anime classification (anime movies use NYAA+1337x, regular movies use 1337x only)

All changes reuse existing infrastructure (ScheduledDownloads table, Plex sync, repository patterns) to minimize new code and maintain architectural consistency.

## Impact

**Affected specs:**
- `tvdb-proxy-api` - MODIFIED to support conditional movie/series endpoints
- `webserver-downloads` - ADDED movie download orchestration
- `torrenter-downloads` - ADDED movie search and processing

**Affected code:**
- `tvdb_proxy/main.go` - Conditional endpoint logic, skip episode fetching
- `webserver/internal/handlers/download_handler.go` - Implement DownloadMovie()
- `webserver/internal/interactors/download_interactor.go` - Movie download orchestration
- `webserver/internal/clients/tvdb_client.go` - Pass media type to extended info
- `torrenter/internal/service/torrent.go` - Remove movie guard, add search strategy
- `torrenter/internal/repository/postgres.go` - Add MovieExistsByTvdbId()

**No breaking changes** - This is additive functionality that doesn't modify existing show download behavior.

**Out of scope:**
- UI changes (backend only)
- Download history tracking (MovieDownloadHistory table exists but unused)
- Shared data structure changes (current structs work with empty episodes array)
