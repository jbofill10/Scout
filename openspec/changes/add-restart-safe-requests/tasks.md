## 1. Repository
- [x] 1.1 `EnqueueNow` (queued row before dispatch, take-over of non-queued rows, `started=false` for a queued one), `TouchQueued`, `DeferRow` in `scheduler_requests.go`; `RecordDispatched` removed; interface updated
- [x] 1.2 sqlmock tests: row written with the scheduled row's hash and a fresh ladder, queued row left alone, touch, defer keeps attempts

## 2. Interactor
- [x] 2.1 `DownloadShow`/`DownloadMovie` enqueue rows first, count a queued row as in progress and an unwritable one as skipped, then dispatch through `processDueGroup`/`processDueItem` on the request's detached context
- [x] 2.2 Immediate-dispatch path removed (`dispatchShowDownload`, `dispatchPacksFirst`, `handleImmediateShowFailures`, `dispatchMovieDownload`, `handleImmediateMovieFailure`, `recordDispatched`, `downloading`)
- [x] 2.3 `promoteDueSource` on the scheduler path only; `touchWaiting` after each per-episode answer; `torrenterDown` + `deferRows` in `processDueItem` and `processDueGroup` (rest of the group deferred on the first refused call)
- [x] 2.4 Tests: rows exist before the torrenter is asked, queued row → in progress, unwritable → skipped, each row settled on its own answer, pack-first unchanged, whole group / rest of group deferred, status error / timeout / deadline keep the ladder, waiting rows touched, source promotion

## 3. Scheduler and client
- [x] 3.1 `Scheduler.Start` re-arms every queued row (`ResetStaleQueued(ctx, 0)`) before the first poll; test
- [x] 3.2 `clients.StatusError` for non-200 torrenter answers; test

## 4. Verify
- [x] 4.1 `gofmt -l`, `go vet ./...`, `go test ./internal/webserver/...` green (2026-11-04)
- [x] 4.2 Deployed the webserver 2026-11-04 20:57 UTC (PR #92) and again 21:06 UTC (PR #93, notification take-over). Re-requested the 45 swept episodes (Cyberpunk: Edgerunners 2, Dan Da Dan 7, JoJo's 26, South Park 10): all 45 answered 202 with a queued row written before dispatch; 43 rows settled within 4 minutes. The second recreate interrupted one row mid-dispatch (Cyberpunk S02E05): the new process logged "Reset stale queued downloads" + "Re-armed downloads interrupted by the restart" at 21:06:16 UTC and re-queued it on its first poll
- [x] 4.3 Follow-up seen on the way: a re-request did not refresh the old failed notification (plain INSERT on the unique key) → PR #93 makes `CreateNotification` an upsert that clears `notified_status`
- [x] 4.4 Live outage test 2026-11-04 22:16:37 UTC: `docker compose restart torrenter` while a 25-row JoJo's group was mid-dispatch. The webserver logged "Torrenter unreachable, rows deferred without counting an attempt" (EOF, then connection refused), rows went pending for 22:18:37 with `torrenter_unreachable`, notifications read "Download service unreachable — will retry (next try 22:18)", and the torrenter resumed its 4 in-flight monitors on start. The retried group found the [Judas] Season 5 Part 2 pack at 22:17:17. Found on the way: stale same-instant timer items re-dispatching settled rows → PR #96 (`RowStatus` check in `processDueBatch`); torrenter slow-add orphan (JoJo's S05E13) → PR #95
- [x] 4.5 Third orphan mechanism found 22:25 UTC: the webserver recreate closed its connection to the torrenter, Gin cancelled the request context, the in-flight add for JoJo's S05E20 failed "context canceled" but qBittorrent carried it out → PR #98 (torrenter handlers run on `context.WithoutCancel`; a torrent arriving after a user cancel is deleted). Torrenter redeployed 22:56 UTC, resumed its monitors. Tally at 22:57 UTC for the 45 re-requested episodes: 34 in the library, 1 downloading, 10 on the retry ladder (no release listed by the indexers yet; next tries 02:22-02:40 UTC), 0 failed, 0 "request it again"
