## Context
The torrenter's notion of "already have" is the Plex mirror (`EpisodeExistsByTvdbId`) plus torrents still being
monitored (`ActiveTorrents`). A torrent that finished is forgotten the moment its completion event is handled, even
when its files cover episodes Scout still wants (a pack whose files were not matched, a copy that was never linked).
The webserver's retry engine re-arms lost episodes one row each and the scheduler fires each row on its own timer, so
a show's retries are N one-episode requests and the torrenter's pack path (`tryPacks`, which needs
`season_pack_min_episodes` in one request) never runs for them.

## Goals / Non-Goals
- Goals: no indexer query for an episode a finished scout torrent already holds; one pack attempt per show per retry
  pass instead of per-episode searches; stop repeating identical no-result searches for old content.
- Non-Goals: movies (the Plex check covers them and they have no pack path); scanning the downloads folder for files
  qBittorrent does not know about; changing the pack matcher; a UI for finished torrents.

## Decisions
- **Local coverage reads qBittorrent, not a new table.** Finished torrents are listed from the `scout` category
  (`Progress == 1`, hash not in `ActiveTorrents`) and judged with the same acceptance rules as indexer results:
  `validatePackSpan` for packs (show name, season coverage, quality floor), `isCorrectTorrent` for single-episode
  releases. A pack's files come from qBittorrent's file list (`GetFilesInformationCtx`, downloaded files only) and are
  matched to the wanted episodes with `packFileEpisodes`, the matcher that links them when a pack lands. This works for
  content downloaded before the change (the Dexter pack) and cannot go stale: a torrent the user removed is simply not
  listed. Alternative considered: a `CompletedTorrents` table written on completion; rejected because it would know
  nothing about existing content and would drift from qBittorrent.
- **Linking reuses the completion path.** A covered episode set is turned into a `TorrentCompleteEvent` carrying a pack
  strategy (`packStrategy`) for packs, or the episode's strict strategy for a single, with the torrent's save path and
  hash, and sent on the request's `done` channel from a goroutine counted on the request's wait group. The completion
  handler then links files, settles notifications, refreshes the mirror and fetches subtitles exactly as for a download.
  An `ActiveTorrents` row is inserted first so a restart between the event and its processing resumes a monitor that
  re-emits it (the torrent reads complete on the first poll). No recheck is forced: the files were verified when the
  torrent finished.
- **`FailedTorrents` exclusions do not apply to finished torrents.** They record releases given up on while
  downloading; a finished torrent's earlier processing failure says nothing about its files.
- **Siblings are claimed in SQL, not drained from the channel.** When the dispatcher receives a due series row it runs
  one `UPDATE ... WHERE schedule_status = 'pending' AND media->>'id' = $show AND due <= now + 15m RETURNING ...`, which
  atomically moves the siblings to `queued`, then cancels their timers (`Scheduler.CancelTimer`). A row whose timer
  fired first is already `queued` and is not claimed twice; a crash mid-group leaves rows `queued`, which
  `ResetStaleQueued` returns to `pending` on the next poll, as today. The window covers rows re-armed in the same
  reconciler pass (seconds apart) and same-day air dates. Alternative considered: draining the queue channel for a
  moment; rejected as timing-dependent.
- **The group runs the pack-first path the request path uses.** `dispatchPacksFirst` is split so both callers share
  the pack-only request and the partition of its answer (`packFirstRequest`); the immediate path keeps recording
  dispatched rows and scheduling retries, the group path settles each existing row with the per-row logic extracted
  from `processDueItem` (`settleDueRow`). Per-episode requests stay sequential, one episode per call, for the reasons
  recorded on `dispatchShowDownload`.
- **The retry cap is a constant, not a setting.** `retry.MaxAttemptsFor(code, aired, now)` returns 2 for
  `no_torrent_found` when the row's newest episode (or the movie) aired more than 30 days before, else 5. It is applied
  where a transient failure is rescheduled and in the "attempt N of M" wording.

## Risks / Trade-offs
- A finished torrent of a similarly named show could be taken for the wanted one → the same word-level name matcher
  that guards search results is used, and a pack is only used for the episodes whose files match by number or title.
- A big group request holds the dispatcher for longer → the pack phase is bounded by the torrenter's per-episode search
  deadline (5 min, inside the 6-min client timeout) and the per-episode phase is the same sequence of one-episode
  calls the rows would have made alone.
- Old-content cap hides a release that appears later → fill-missing and a manual request start a fresh ladder.

## Migration Plan
Deploy torrenter and webserver together (the webserver's group request is an ordinary request to the torrenter, so
either order works). Re-request Dexter S01-S06 afterwards: the finished pack covers them with no indexer query.
Rollback: redeploy the previous images; no schema change.

## Open Questions
- Movies: extend the local check to finished movie torrents later if duplicates show up there.
