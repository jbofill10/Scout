## Context
Two dispatch paths grew side by side. A due scheduled row is claimed (`queued`), sent, and settled by
`settleDueRow`; a crash between claim and settle is healed by `ResetStaleQueued` on the next poll. A direct request
had no row until the answer came back: `RecordDispatched` wrote a completed row for a download that started,
`ScheduleRetry` a pending row for one that failed. Between the request and the answer, which for a whole season is
hours of one-episode calls, the only trace of the request was the "searching" notification. The webserver restarted
twice on 2026-10-30 and 2026-11-03/04 for deployments, and each time the episodes not yet answered were stranded.

## Goals / Non-Goals
- Goals: no request accepted by the webserver is lost to a restart of the webserver or the torrenter; a torrenter
  that is down is waited for, not retried into the ladder; one dispatch path.
- Non-Goals: rebuilding episodes from notifications alone (the notification has no show id, air date or aliases);
  changing the stale-search sweep's 24-hour window; making the torrenter persist in-flight searches (its search is
  bounded in minutes and the webserver re-sends).

## Decisions
- **Direct requests reuse the scheduled-row machinery rather than persisting a separate "in-flight request".**
  `EnqueueNow` inserts a `queued` row with `next_attempt_at = now()` and `queued_at = now()`, keyed by the same
  content hash a scheduled row for the episode uses, so there is one row per episode across every way of asking.
  `ON CONFLICT ... WHERE schedule_status <> 'queued'` takes over completed, failed, cancelled and pending rows
  (fresh ladder, this request's payload and trace) and refuses a queued one, which is a dispatch under way. The
  request then calls `processDueGroup`/`processDueItem` with the rows, exactly what the scheduler calls for due
  rows, so pack-first, settlement, the old-content cap and the notification wording are shared. Alternative
  considered: route direct requests through the scheduler's queue channel; rejected because the channel has one
  consumer and a 23-episode request would hold every air-date download behind it, and because the response's
  counts are computed before dispatch either way.
- **Recovery is the scheduler's existing reset, run once at start with no age.** `ResetStaleQueued(ctx, 0)` before
  the first poll: in a new process nothing can be in flight, so every queued row was interrupted. The first poll
  then dispatches them, grouped per show by `ClaimDueSiblings` as any due rows are.
- **Long groups heartbeat instead of raising the stale threshold.** `processDueGroup` calls `TouchQueued` on the rows
  still waiting after each per-episode answer (`queued_at = now()`), so a 70-row group that runs for two hours is
  never mistaken for a crashed dispatch, while a real crash is still caught 30 minutes after the last answer.
- **"Torrenter down" is a transport error that is not a timeout.** `torrenterDown(err)`: a `*url.Error` from the
  HTTP client that is neither a `net.Error` timeout nor a cancelled or expired context, and not a
  `*clients.StatusError`. A 5xx is deliberately on the ladder: a payload the torrenter fails deterministically must
  not loop every two minutes forever. `deferRows` writes `pending`, `next_attempt_at = now + 2m`, the unreachable
  code and a reason, leaves `attempts` alone, updates the notifications and kicks the scheduler so the two-minute
  timers are armed at once. In a group the first refused call defers the remaining rows without asking for each.
- **Search priority.** The row keeps the request's `Source` (`manual`, `follow`, `missing`), so a direct dispatch
  searches at the priority it was made with; `promoteDueSource` (the former inline override) still lifts a
  scheduler-fired first attempt of a non-manual row to `scheduled`. A row recovered after a restart is dispatched
  by the scheduler and so reads `scheduled` on its first attempt, which is acceptable for a request that was
  interrupted anyway.
- **Rows the request cannot write are not started.** If `EnqueueNow` fails, the episode is counted as skipped with
  a history row, like a scheduling error for a future episode, rather than searched for with nothing to recover
  from.

## Risks / Trade-offs
- A request for an episode whose retry row is pending with a timer armed takes the row over → the timer is
  cancelled (`CancelTimer`) as `processDueBatch` does for siblings; a duplicate dispatch, should one slip through,
  is answered by the torrenter's in-flight check and settles as a no-op on a row no longer queued.
- `GetUpcoming` and the Activity metadata now see direct requests as queued rows → the UI treats `queued` as "not
  retrying" already; a deferred row shows its two-minute retry and the unreachable reason, which is the truth.
- The torrenter down for a long time means a dispatch attempt every two minutes per group → each is one refused
  connection, logged once per group at warn level.

## Migration Plan
Deploy the webserver. No schema change; the torrenter is untouched. On the first start the queued rows of the old
build (if any) are re-armed like any interrupted dispatch. The 45 episodes already retired by the sweep have no
row and are re-requested by hand once (payloads prepared from `/media/:id/details`). Rollback: redeploy the previous
image; queued rows it finds are handled by its stale sweep after 30 minutes.

## Open Questions
- Whether the stale-search sweep is still needed once every request has a row; it stays as a backstop for rows the
  database refused.
