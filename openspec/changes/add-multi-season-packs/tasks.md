## 1. Search reliability
- [x] 1.1 Monitor: waiting states reset the stall clocks; error states give up without blacklisting; `giveUpOnTorrent` takes a blame flag
- [x] 1.2 `FailedTorrent.FailedAt` read from the table; exclusions older than `FAILED_TORRENT_TTL` ignored
- [x] 1.3 `searchNames`: normalised dedupe, parenthesised split, Latin-first, cap of four; ladder repeats only "S##E##" per name and drops duplicate queries
- [x] 1.4 `searchForMatch` shares the deadline between indexer tiers; strategies cut off by a tier's share are skipped, not counted as indexer errors
- [x] 1.5 `scoreNameMatch` compares words, not substrings; upgrade searches carry the year

## 2. Multi-season packs
- [x] 2.1 `parsePack` reads season ranges and complete-series wording; `PackInfo.Seasons`
- [x] 2.2 `validatePack`/`packQueries`/`searchForPack` take the wanted seasons; oversize packs scored below exact ones
- [x] 2.3 `tryPacks` tries one pack across all seasons, then per season, then per episode
- [x] 2.4 Shared file matcher reads season folders; packs are added paused, `trimPackFiles` deselects unwanted files once the list is known, the space guard runs on the trimmed size, then the torrent is started; pack siblings are marked lost when a pack torrent is given up on
- [x] 2.5 Webserver pack-first dispatch (`dispatchPacksFirst`, `PackOnly`, `no_pack`); torrenter answers pack-only requests within the search deadline
- [x] 2.6 Multi-episode pack files linked as "S##E##-E##"; unusable packs given up on
- [x] 2.7 Tests: name matching, names and ladder size, waiting/error states, exclusion expiry, tier budget, range parsing, multi-season validation and selection, file matching, trimming, landing

## 3. Verify
- [x] 3.1 `go vet ./...`, `go test ./...`
- [x] 3.2 Deployed torrenter, webserver, ui (PRs #69-#73); `season_packs` on; SpongeBob S5-S10 requested 2026-11-01: one pack-only request, "SpongeBob SquarePants S01-13 1080p COMPLETE" added from TPB, trimmed from 222 GB to the 132 title-matched files (~43 GB), downloading
