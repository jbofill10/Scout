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
- [ ] 4.2 Deploy the webserver, confirm "Re-armed downloads interrupted by the restart" on start when rows were queued, re-request the 45 swept episodes
