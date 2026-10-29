# Tasks

## 1. Webserver: immediate response

- [x] 1.1 Add `DownloadSummary` (queued now / scheduled / skipped / next release / trace id)
- [x] 1.2 Return the summary from `DownloadShow` and `DownloadMovie`
- [x] 1.3 Move the torrenter hand-off onto a detached context via an injectable dispatcher
- [x] 1.4 Return HTTP 202 with the summary from both download handlers
- [x] 1.5 Treat an already-scheduled movie as skipped rather than an error

## 2. Webserver: activity endpoint

- [x] 2.1 Add `GetPendingScheduleMeta` keyed by notification tvdb id
- [x] 2.2 Add `ActivityInteractor` combining notifications with scheduling state
- [x] 2.3 Add `GET /activity` handler and route

## 3. UI: request feedback

- [x] 3.1 Add `ToastContext` / `ToastProvider` and mount it in `App`
- [x] 3.2 Add `useDownloadRequest` with summary-to-sentence formatting
- [x] 3.3 Route GenreRow, SearchDropdown, ScheduleWidget and SearchResultsList through it
- [x] 3.4 Pass the pending state into `MediaStatusDialog` at every call site

## 4. UI: activity view

- [x] 4.1 Add shared stage vocabulary (`downloadStage`, `StageChip`, `stageIcon`)
- [x] 4.2 Add `useActivity` and `useActiveDownloadCount`
- [x] 4.3 Add the `/activity` page with stage tiles, in-flight and finished lists
- [x] 4.4 Add the navbar link with the in-flight badge
- [x] 4.5 Move NotificationDropdown onto the shared stage vocabulary

## 5. Verification

- [x] 5.1 Interactor tests: summary counts, duplicate handling, request returns before dispatch
- [x] 5.2 Activity tests: in-flight/finished split, retry enrichment, degraded metadata
- [x] 5.3 Repository test: schedule metadata keying for shows and movies
- [x] 5.4 `go build ./...`, `go test ./...`, `tsc -b`, `npm run lint`, `npm run build`
