# Change: Add download controls (cancel, retry now, pick a release) and an upcoming list

## Why
Once something is requested there is no way to act on it. A download that picked a dead torrent, a retry that is waiting hours for its backoff, or an episode requested by mistake can only be fixed by editing the database and restarting services. The schedule widget shows seven days, so anything scheduled further out is invisible.

## What Changes
- **Cancel** an item from the Activity page: stops an in-flight search, removes a downloading torrent (and its files) from qBittorrent, cancels any scheduled or retry row, and marks the notification `cancelled` (new stage).
- **Retry now** for an item waiting on a retry backoff or permanently failed: re-arms its scheduled row and makes the scheduler dispatch it immediately instead of at the next poll.
- **Pick a release**: lists every candidate the torrenter can see for an item across all indexers (accepted and rejected, with the reason and score) and lets the user grab one by hand, replacing whatever is in flight.
- **Upcoming** section on the Activity page listing every pending scheduled row (future releases and pending retries) with a cancel action, not just the next seven days.
- Re-requesting a cancelled item revives its row instead of being reported as a duplicate.

## Impact
- Affected specs: `webserver-downloads`, `torrenter-downloads`, `ui-notifications`
- Affected code:
  - torrenter: `service/torrent_cancel.go` (new), `service/torrent_releases.go` (new), monitor loop made cancellable, `handlers/control_handler.go` (new), `interactors/download_interactor.go`
  - webserver: `scheduler` (per-row timer cancellation + kick), `repository/scheduler_repository.go` (cancelled status, lookup by item, re-arm, upcoming), `interactors/download_control_interactor.go` (new), `handlers/download_control_handler.go` (new), `clients/torrenter_client.go`
  - shared: `pkg/dlstatus` (`cancelled` code), `pkg/notifications` (`cancelled` status)
  - ui: `downloadStage.ts` (`cancelled`), `hooks/useDownloadControls.ts` (new), `hooks/useUpcoming.ts` (new), `components/ui/ReleasePickerDialog.tsx` (new), `pages/Activity.tsx`
