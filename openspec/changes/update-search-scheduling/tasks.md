## 1. Torrenter: finished downloads before indexers
- [x] 1.1 `coverFromFinished` in `service/torrent_local.go`: list finished `scout` torrents, skip in-flight hashes, gate on the show name, accept packs via `validatePackSpan` + file match (downloaded files whatever their priority), singles via `isCorrectTorrent` on the episode the title numbers; emit completion events, insert `ActiveTorrents` rows, report covered episodes downloading
- [x] 1.2 Called in `HandleDownload` after the in-flight filter and before `tryPacks`; pack-only requests included; completion handler skips the UUID tag removal for untagged torrents
- [x] 1.3 Tests with the fake qBittorrent server: pack covers wanted episodes (files present only), pack of another show ignored, below the quality floor ignored, single release covers one episode, single of another show ignored, deselected-but-downloaded file counts, in-flight torrent skipped, qBittorrent unreachable falls through

## 2. Webserver: due rows of one show dispatched together
- [x] 2.1 `SchedulerRepository.ClaimDueSiblings(ctx, showID, exceptID, until)` (atomic pending → queued, RETURNING) + fake
- [x] 2.2 `packFirstRequest` shared by `dispatchPacksFirst` and the new `processDueGroup`; `settleDueRow` extracted from `processDueItem`; `processDueBatch` claims siblings, cancels timers, dispatches the group
- [x] 2.3 Tests: siblings claimed and sent as one pack-only request, each row settled on its own result, `no_pack` episodes sent one at a time, packs off → per episode, movie and lone rows unchanged

## 3. Webserver: shorter ladder for old content
- [x] 3.1 `retry.MaxAttemptsFor` / `NextAttemptWithin`; applied in `settleDueRow` and the attempt wording
- [x] 3.2 Tests: old `no_torrent_found` exhausts after 2, recent keeps 5, other codes keep 5

## 4. Verify
- [x] 4.1 `go vet ./...`, `gofmt -l`, `go test ./...` (all packages green 2026-11-04)
- [x] 4.2 Deployed torrenter + webserver 2026-11-04 18:06 UTC (PR #90). Re-requested Dexter's 72 missing episodes: 67 queued, one "Episodes covered by a finished download" event for pack FEC8D3A3, "Season pack processed wanted=67 linked=67" 112 s later, 0 Prowlarr queries mentioning Dexter; notifications 91 completed, S01-S05 12/12 each in the library
