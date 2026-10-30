# Change: Retry downloads the torrenter loses after the hand-off

## Why
A 61 GB 4K remux of "X" (2022) was still downloading when the torrenter's fixed 6-hour watch expired. The monitor marked the item "Lost track of the download — will retry", stopped watching, and left the torrent running in qBittorrent, where it kept downloading with nobody to link it into Plex. Nothing retried it: the webserver only schedules retries from the dispatch response, and this failure arrived hours later through the notification. The item had no schedule row either (direct movie requests never created one), so Retry and Pick release were unavailable too. The same hole swallows dead swarms with no replacement and torrents removed from qBittorrent.

## What Changes
- The monitor gives up when a download makes **no progress for MONITOR_TIMEOUT** (default 6h), not after that much wall-clock time. A slow download that keeps moving is watched to completion; a resumed monitor measures the window from its own start. A stalled download is removed and replaced through the same path as a dead swarm.
- Every failure the torrenter reports on its own carries a machine-readable `failure_code` on the notification (new column, migration 013): `monitor_lost`, `no_torrent_found`, `processing_error`, and so on. Other transitions clear it.
- The webserver keeps a `completed` schedule row for every item the torrenter accepts into a download, including direct requests, so the request payload exists to retry from.
- A reconciler in the webserver (every 2 minutes) finds failed notifications with a transient code and no live row and re-arms the item's newest completed row on the usual backoff ladder (1h, 4h, 12h, 24h, then permanently failed). A lost movie with no row at all gets a retry row rebuilt from the notification.
- Retry now and Pick release work for an item whose only row is completed; Cancel ignores completed rows.

## Impact
- Affected specs: `torrenter-downloads`, `webserver-downloads`
- Affected code: `pkg/notifications` (FailureCode, SQL), migration 013, torrenter `service/torrent_monitor.go`, `torrent_notify.go`, `torrent_failures.go`, `interactors/download_interactor.go`; webserver `repository/scheduler_controls.go` (lookups, RearmCompletedRow, FailCompletedRow, RecordDispatched), `interactors/async_failures.go` (new), `download_interactor.go`, `download_control_interactor.go`, `activity_interactor.go`
