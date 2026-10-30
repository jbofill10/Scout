## 1. Backend
- [x] 1.1 Migration `012_add_upgrade_attempts.sql`; settings keys `upgrades_enabled`, `upgrade_cutoff`, `upgrade_batch` through the resolver
- [x] 1.2 `SearchStrategy.MinTier/MaxTier/Upgrade`; `scoreMatch` honours the window; `FileSystem.Remove`
- [x] 1.3 Repository: upgrade candidates (episodes + movies with resolution and paths), attempts CRUD
- [x] 1.4 `QbittHandler.DownloadUpgrade` (bypasses the exists filter, checks in-flight) and `MediaProcessSvc` replaces the old files on landing
- [x] 1.5 `UpgradeInteractor`: candidates with Plex resolution mapping, run (batch, attempts, notifications upsert), completion bookkeeping, timer; handler + routes; webserver relay; config `UPGRADE_CHECK_INTERVAL`
- [x] 1.6 Tests: resolution mapping and candidate selection, quality window in scoring, processor removes old files, handler codes

## 2. UI
- [x] 2.1 `useUpgrades` hooks; Upgrades section on Settings (toggle, cutoff, batch, candidates, attempts, Run now)
- [x] 2.2 Tests: hooks

## 3. Verify
- [x] 3.1 `go vet`, `go test ./...`; `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [ ] 3.2 Deploy torrenter, webserver, ui; candidates list matches the mirror's sub-cutoff items; upgrades stay off until enabled
