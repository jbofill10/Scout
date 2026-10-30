# Change: Refuse to start a download that would fill the disk

## Why
Nothing checks free space before a torrent is added. A full download volume means every in-flight torrent stalls at once, Plex scans fail, and the failure surfaces as a dozen unrelated errors.

## What Changes
- Before adding a torrent (automatic pick or manual grab) the torrenter compares the free space of the downloads volume with the release size plus a configurable reserve (`MIN_FREE_SPACE_GB`, default 10; overridable from Settings as `min_free_space_gb`). When it does not fit, the item fails with the new transient code `insufficient_disk_space`, so the retry engine tries again later instead of giving up.
- `GET /settings` reports the volume's current free space, and the Downloads section of the Settings page shows it next to the reserve field.

## Impact
- Affected specs: `torrenter-downloads`, `settings`
- Affected code: `pkg/dlstatus`, `pkg/settings`, torrenter `service/diskspace.go`, `service/torrent.go` (`initiateDownloadAndMonitor`), `internal/torrenter/settings`, config; UI Settings page
