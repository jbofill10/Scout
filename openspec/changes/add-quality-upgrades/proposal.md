# Change: Upgrade library copies below a quality cutoff

## Why
Once a 720p (or worse) copy lands it is never replaced, even when a 1080p release appears later. The Plex mirror already records each file's resolution, so the gaps are known; nothing acts on them.

## What Changes
- Settings `upgrades_enabled` (off by default), `upgrade_cutoff` (default 1080p) and `upgrade_batch` (default 3 per run).
- A torrenter pass (`UPGRADE_CHECK_INTERVAL`, default 6h, and on demand) that lists library episodes and movies whose best copy is below the cutoff, skips items attempted recently or in flight, and for a batch of them runs the normal search with a per-search quality window (above the current copy, at most the cutoff). A found release downloads through the normal pipeline; when it lands the old files are removed from the library folder before the new one is linked, Plex is re-synced, and the attempt is recorded. No release found counts as an attempt and is retried after a week, giving up after three tries.
- `GET /upgrades/candidates`, `GET /upgrades/attempts`, `POST /upgrades/run` on the torrenter, relayed by the webserver; an Upgrades section on Settings with the toggle, cutoff, batch size, candidate count, recent attempts and Run now. Upgrades appear on the Activity page like any download, with the reason "Upgrading from 720p".

## Impact
- Affected specs: new `upgrades` capability; `settings`; `torrenter-downloads`
- Affected code: migration `012_add_upgrade_attempts.sql`; `pkg/settings`; torrenter `models` (strategy quality window + upgrade info), `service/torrent_score.go`, `service/torrent_upgrade.go`, `service/mediaprocessor.go` (replace old files), `service/filesystem.go` (Remove), repository (candidates, attempts), `interactors/upgrade_interactor.go`, `handlers/upgrade_handler.go`, config, main; webserver relay routes; UI Settings + `hooks/useUpgrades.ts`
