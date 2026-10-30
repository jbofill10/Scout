# Design: download controls

## Item identity
Every layer already agrees on one key for "this episode" or "this movie": the notification `tvdb_id`, which is the TVDB episode id for series and the media id for movies. The webserver derives it from a scheduled row with `NotificationKey(media)`, the torrenter stores it on every strategy as `EpisodeTvdbID`, and the UI receives it as `ActivityItem.tvdb_id`. All control endpoints take this key, so the UI never needs row ids or info hashes.

## Cancel
Three places can hold work for an item and all three are cleared:
1. **Scheduler timers** (webserver memory). The scheduler keeps a per-row cancel channel; `CancelTimer(rowID)` closes it and releases the content-hash dedupe slot so a later poll can re-arm the row if it is ever revived.
2. **Scheduled rows** (webserver DB). New `schedule_status = 'cancelled'`. Excluded from due queries, pending metadata and the schedule views. `Schedule`'s `ON CONFLICT (content_hash)` now revives a cancelled row (status back to pending, attempts reset) instead of doing nothing, so re-requesting works; any other status still reports a duplicate.
3. **Torrenter work**. A registry keyed by item id holds the cancel function of the in-flight search context, or the cancel function of the monitor plus its info hash. Cancel aborts the search (the result comes back with code `cancelled`, which the webserver ignores rather than scheduling a retry) or stops the monitor, deletes the torrent with its files from qBittorrent and removes the `ActiveTorrents` row. Monitors register themselves on start, so resumed monitors are covered too. Rows in `ActiveTorrents` that have no live monitor are also removed.
The notification ends as `cancelled`. Cancelled items appear under Finished on the Activity page.

## Retry now
Only rows can be retried (an item with no row has nothing to re-dispatch). For each pending or failed row for the item: cancel its timer, set `schedule_status = 'pending'`, `next_attempt_at = now()`, and for a permanently failed row reset `attempts` to 0 so it gets a fresh retry budget. Then `Scheduler.Kick()` wakes the poll loop, which finds the row already due and dispatches it immediately. A row that is `queued` (dispatch in progress) is reported as a conflict.

## Pick a release
`POST torrenter /releases` runs the item's search strategies (strict and first relaxed tier) against every indexer tier without stopping at the first hit, validates each result exactly as the automatic path does, and scores accepted ones with the same scoring `pickBestTorrent` uses (extracted into `scoreMatch`). The response carries, per candidate: title, indexer, size, seeders, leechers, age, parsed quality, score, whether Scout would accept it and if not why, whether it previously failed for this item, and the identifiers needed to grab it (guid, download url, info hash, indexer id). Duplicates across queries collapse on guid or info hash.
`POST torrenter /downloads/manual` takes one of those candidates, builds the item's strict strategy, cancels anything already in flight for the item, and feeds the chosen torrent through the normal `initiateDownloadAndMonitor` path, so completion, Plex linking and subtitles work as for an automatic pick.
The webserver resolves the item's `tvdb.Media` from its scheduled row when one exists and otherwise lets the torrenter resolve it from the active torrent's stored request. After a successful manual grab, the item's rows are marked completed and their timers cancelled, exactly as after an automatic dispatch.

## Upcoming
`GET /schedule/upcoming` returns every `pending` row ordered by `COALESCE(next_attempt_at, release_time)`, with the item key, title, episode, poster, release time, next attempt, attempts and last failure. The Activity page renders it as an Upcoming section between In flight and Finished, with Cancel per row.

## Why not a new page
Upcoming lives on the Activity page rather than a new route because it answers the same question ("what is Scout going to do?") and the phone bottom bar has no room for another tab.
