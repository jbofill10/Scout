# Change: Requests survive a restart

## Why
On 2026-11-04 the Activity page showed 45 episodes failed with "Search was lost in a restart; request it again"
(Cyberpunk: Edgerunners, Dan Da Dan, JoJo's Bizarre Adventure, South Park), all requested on 2026-10-30 and all
sitting on "searching" since. A request sent straight to the torrenter is only written to `ScheduledDownloads`
after the torrenter answers for the episode (a completed row when it started, a retry row when it failed), and the
webserver works through a request's episodes one at a time. When the webserver restarts mid-batch, every episode not
yet answered has a notification and nothing else: no row, no payload, nothing a retry could be built from. The
reconciler's stale-search sweep can only retire the notification and ask the user to request it again. The user
should not have to; the system knows what it was asked for.

## What Changes
- **The row comes first (webserver).** `POST /shows` and `POST /movies` write one queued `ScheduledDownloads` row
  per aired episode (or the movie) before the torrenter is asked, keyed like the row a future air date would create.
  The dispatch then runs the path a due scheduled row takes (pack-first when packs are on, then one episode per
  call) and settles each row on its own answer: completed, on the retry ladder, or failed. An existing row for the
  item in any status but queued is taken over with a fresh ladder; a row already queued means a dispatch is under
  way and the summary counts the item as in progress. The separate immediate-dispatch code path
  (`dispatchShowDownload`, `dispatchPacksFirst`, `handleImmediateShowFailures`, `dispatchMovieDownload`,
  `handleImmediateMovieFailure`, `RecordDispatched`) is removed.
- **A restart re-arms what it interrupted (webserver).** Before its first poll a new process returns every queued
  row to pending, however recently it was queued, so the first poll sends it again. A group of rows working
  through its episodes refreshes the rows still waiting, so the 30-minute stale-queued sweep does not hand a long
  group's tail to a second dispatch.
- **An unreachable torrenter costs no attempt (webserver).** When the torrenter cannot be reached at all (connection
  refused or dropped), the rows of the dispatch are held for two minutes without counting an attempt, with one
  refused call per group rather than one per episode, and the scheduler is woken so they fire on time. An error
  status, a timeout and a cancelled dispatch mean the torrenter was there and keep the usual ladder; the client now
  types error statuses (`clients.StatusError`) so the two can be told apart.

## Impact
- Affected specs: `webserver-downloads`
- Affected code: webserver `interactors/download_interactor.go`, `interactors/due_groups.go`,
  `interactors/async_failures.go`, `repository/scheduler_requests.go` (new: `EnqueueNow`, `TouchQueued`,
  `DeferRow`), `repository/scheduler_controls.go` (`RecordDispatched` removed), `scheduler/scheduler.go`,
  `clients/torrenter_client.go`
- Schema: none. Settings: none.
- Behaviour visible to users: a failed episode's notification reads "(attempt 1 of 5, next try HH:MM)" after a
  direct request, as it already did after a scheduled one, instead of "— retry scheduled"; the Activity page's
  schedule metadata now covers direct requests while they run (status `queued`).
