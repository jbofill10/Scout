# Change: Search what Scout already holds first, and retry a show as one request

## Why
On 2026-11-02 Scout found and downloaded the Dexter S01-S08 pack in one search, then spent the next two days running
about 2,700 indexer queries for the same 85 episodes: the pack's files were not recognised when it landed (a matcher
bug since fixed), the 95 lost episodes were re-armed as 95 single-episode retry rows, and every retry was a one-episode
request, which is below the pack threshold and knows nothing about the finished pack sitting in qBittorrent with every
file on disk. Nothing in the retry path looks at what is already downloaded or at the other rows of the same show.

## What Changes
- **Finished downloads before indexers (torrenter).** Before searching for a show's episodes, the torrenter looks
  through the finished torrents in its qBittorrent category for one that already holds them: a pack whose title covers
  their seasons and whose downloaded files match them, or a single-episode release that passes the same validation as
  a search result. Those episodes are linked from disk through the normal completion path and reported as
  downloading; no indexer query runs for them. Torrents still in flight are left to the existing in-flight check.
- **Due rows of one show dispatched together (webserver).** When a scheduled row for a series comes due, the webserver
  claims the show's other pending rows due within the next 15 minutes, cancels their timers, and sends them all as one
  request: pack-first when packs are on and there are enough episodes, then one episode at a time for whatever no pack
  covered. Each row is still settled on its own episode's outcome.
- **Shorter retry ladder for old content with no release (webserver).** A `no_torrent_found` failure on content that
  aired more than 30 days ago exhausts after 2 attempts instead of 5; repeating the identical ladder four more times
  over 41 hours finds nothing new for a 2007 episode. Other transient failures keep the full ladder.

## Impact
- Affected specs: `torrenter-downloads`, `webserver-downloads`
- Affected code: torrenter `service/torrent_local.go` (new), `service/torrent.go`; webserver
  `interactors/download_interactor.go`, `interactors/due_groups.go` (new), `repository/scheduler_repository.go`
  (`ClaimDueSiblings`), `retry/policy.go`
- Settings: none added. Packs keep following `season_packs` / `season_pack_min_episodes`.
