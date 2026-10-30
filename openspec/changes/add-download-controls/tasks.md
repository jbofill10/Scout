## 1. Shared
- [x] 1.1 `dlstatus.CodeCancelled` (permanent, "Cancelled") and `notifications.StatusCancelled`

## 2. Torrenter
- [x] 2.1 Cancel registry; register searches in `processEpisodeDownload` and monitors in `monitorTorrentCompletion`; monitor sleep becomes cancellable
- [x] 2.2 `CancelItem`: abort search / stop monitor, delete torrent + files, clear `ActiveTorrents`, notification `cancelled`
- [x] 2.3 Extract `scoreMatch` from `pickBestTorrent` (no scoring change); `ListReleases` across all indexer tiers with accept/reject reasons
- [x] 2.4 `DownloadRelease` (manual grab) through `initiateDownloadAndMonitor`
- [x] 2.5 Interactor + handler + routes: `POST /downloads/cancel`, `POST /releases`, `POST /downloads/manual`
- [x] 2.6 Tests: registry/cancel, release listing, manual grab, cancelled search result

## 3. Webserver
- [x] 3.1 Repository: `StatusCancelled`, `FindRowsByItem`, `CancelRow`, `RearmRow`, `GetUpcoming`, `GetScheduleMeta(statuses)`, `Schedule` revives cancelled rows
- [x] 3.2 Scheduler: per-row timer cancellation, `Kick()`
- [x] 3.3 Torrenter client: `Cancel`, `ListReleases`, `ManualDownload`
- [x] 3.4 `DownloadControlInteractor`: cancel, retry now, releases, grab, upcoming; dispatch paths ignore `cancelled` results
- [x] 3.5 Activity items carry `can_cancel`, `can_retry`, `can_pick_release`
- [x] 3.6 Handler + routes: `POST /downloads/:id/cancel`, `POST /downloads/:id/retry`, `GET|POST /downloads/:id/releases`, `GET /schedule/upcoming`
- [x] 3.7 Tests: repository (sqlmock), scheduler cancel/kick, interactor paths, handler status codes

## 4. UI
- [x] 4.1 `cancelled` stage (colour, label, icon, description)
- [x] 4.2 `useDownloadControls` (cancel / retry / releases / grab) and `useUpcoming`
- [x] 4.3 Activity rows: Cancel, Retry now, Pick release actions; `ReleasePickerDialog`
- [x] 4.4 Upcoming section with Cancel, collapsed past 10 rows
- [x] 4.5 Tests: stage vocabulary, controls hook, dialog rendering

## 5. Verify
- [x] 5.1 `go vet`, `go test ./...`; `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [ ] 5.2 Deploy webserver, torrenter, ui; cancel a scheduled item, retry a failed one, grab a release by hand
