# Change: Download Request Feedback and Activity View

## Why

Clicking "Download" gave the user nothing to go on. The request blocked while the
webserver called the torrenter, which searches Prowlarr and adds torrents to
qBittorrent — up to the client's 5 minute timeout — and then returned an empty
`{}`. Three of the four UI download entry points showed no confirmation at all;
only the search results list raised a toast, and none of them said what was
actually going to happen.

Once a request was accepted there was also no way to see it through. Per-episode
stages (scheduled → searching → downloading → completed/failed) were already
recorded in the `Notifications` table, along with retry counts and next-attempt
times in `ScheduledDownloads`, but nothing surfaced them beyond the bell
dropdown's grouped summary. A download that silently failed or sat retrying was
invisible.

## What Changes

- **Download endpoints answer immediately.** `POST /shows` and `POST /movies`
  classify every episode (queued now / scheduled for release / skipped), create
  its notification, then return HTTP 202 with a `DownloadSummary`. The torrenter
  hand-off runs on a detached context; failures continue to surface through
  notifications and the retry engine exactly as before.
- **New `GET /activity` endpoint** joining notifications with the scheduling and
  retry state of their `ScheduledDownloads` row, split into in-flight and
  finished work with a count per stage.
- **App-wide toast** reporting the outcome of every download request, with a
  shortcut to the activity view.
- **Single shared download hook** (`useDownloadRequest`) replacing four
  hand-rolled `fetch` calls, so every entry point reports progress and errors
  identically and shows a pending state on its button.
- **New `/activity` page** listing each tracked episode or movie with its stage,
  the reason it is waiting, when it will be retried, and its trace id. The navbar
  Activity link badges the in-flight count.

### Breaking Changes

- **BREAKING**: `POST /shows` and `POST /movies` now return HTTP 202 with a
  summary body instead of HTTP 200 with `{}`. Clients checking for exactly 200
  must accept 2xx.
- A movie that is already scheduled no longer returns HTTP 500; it reports
  `skipped: 1` so the UI can say "already tracked".

## Impact

### Affected Specs
- `webserver-downloads` (MODIFIED) — asynchronous dispatch and summary response
- `webserver-downloads` (ADDED) — activity endpoint
- `ui-notifications` (ADDED) — download confirmation toasts and the activity page

### Affected Code
- `backend/internal/webserver/interactors/download_interactor.go`
- `backend/internal/webserver/interactors/activity_interactor.go` (new)
- `backend/internal/webserver/handlers/download_handler.go`
- `backend/internal/webserver/handlers/activity_handler.go` (new)
- `backend/internal/webserver/repository/scheduler_repository.go`
- `backend/cmd/webserver/main.go`
- `ui/src/hooks/useDownloadRequest.ts`, `ui/src/hooks/useActivity.ts` (new)
- `ui/src/pages/Activity.tsx` (new)
- `ui/src/components/ui/ToastProvider.tsx`, `StageChip.tsx`, `stageIcon.tsx` (new)
- `ui/src/contexts/ToastContext.ts`, `ui/src/utils/downloadStage.ts` (new)
- `ui/src/components/ui/{GenreRow,SearchDropdown,ScheduleWidget,Navbar,NotificationDropdown}.tsx`
- `ui/src/components/SearchResultsList.tsx`, `ui/src/App.tsx`
