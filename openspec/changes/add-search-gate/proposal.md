# Change: Pace and prioritise indexer searches

## Why
Scout can now generate a lot of searches on its own: a follow with "also grab missing episodes" on a 100-episode show, a Missing-tab fill, the upgrade pass. Each episode costs several queries (strict and relaxed strategies, then the fallback indexer) and requests run in parallel with each other, with nothing counting searches across them. The original 1337x ban came from about 77 queries in four minutes. There is also no notion of priority, so a back-catalogue fill competes equally with the episode a person just asked for, and a banned indexer keeps receiving queries until Prowlarr re-enables it.

## What Changes
- A search gate in the torrenter (`internal/torrenter/searchgate`) that every Prowlarr search passes through: per indexer, a token bucket (`search_rate_per_minute`, default 6, with `search_burst`, default 3; per-indexer overrides in `indexer_rate_limits`) and one search in flight at a time; waiting searches are served by priority (manual, scheduled, follow, missing, upgrade), then arrival.
- Requests carry a `source` (`tvdb.Media.Source`): the webserver sets `manual` for UI requests, `scheduled` for an air-date dispatch, `follow` and `missing` for those checks; retries keep the source of the request they came from; the upgrade pass sets `upgrade`. Release listings and manual grabs are manual.
- An indexer Prowlarr has disabled is paused on the gate by the health monitor: queued and new searches against it fail fast and the request falls through to its fallback tier instead of waiting out the ban. It is resumed when healthy again.
- A scheduled follow check hands over at most `bulk_request_batch` episodes per run (default 10) across all shows, least recently checked show first, oldest wanted episodes first; the rest wait for the next check and are reported as deferred. Check now and Follow are never capped.
- `GET /indexers/health` gains a `queue` array per indexer: what is running, what waits (by priority, with labels), searches in the last minute, the limits, and any pause. The Settings page gets a Search pacing section.

## Impact
- Affected specs: `torrenter-downloads`, `webserver-downloads`
- Affected code: torrenter `searchgate` (new), `service/torrent_search.go`, `torrent.go`, `torrent_upgrade.go`, `torrent_releases.go`, `torrent_monitor.go`, `indexer_health.go`, `handlers/indexer_handler.go`, settings resolver, config; `pkg/media` (Source), `pkg/settings`; webserver `download_handler.go`, `follow_interactor.go`, `missing_interactor.go`, `download_interactor.go`, `download_control_interactor.go`, `clients/torrenter_client.go`; UI settings and indexer health types
