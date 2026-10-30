# Change: Opt-in season packs for back-catalogue requests

## Why
Every episode is searched for and downloaded on its own, even when a whole season is requested. For a back catalogue that means ten searches against a rate-limited indexer, ten torrents and ten swarms, where one well-seeded season pack would do. Multi-episode torrents are rejected by design today.

## What Changes
- Settings `season_packs` (off by default) and `season_pack_min_episodes` (default 4). When on, a show request whose episodes-to-download include at least that many from one season first searches for a pack of that season ("Show S01", "Show Season 1", "complete"; "batch" for anime). A pack is accepted when its title names the show and the season (or an episode range covering the wanted episodes) at or above the quality floor; otherwise the season falls back to per-episode searches as today.
- A pack downloads as one torrent whose strategy carries the wanted episodes. Every one of them reads as downloading and counts as in flight. When the torrent lands, each video file inside it is matched to a wanted episode by its own season and episode numbers (or absolute number for anime) and linked under the Plex name; episodes the pack did not contain are marked failed with that reason.

## Impact
- Affected specs: `torrenter-downloads`, `settings`
- Affected code: torrenter `models` (PackInfo, pack results), `service/torrent_pack.go` (new), `service/torrent.go` (HandleDownload), `service/torrent_filter.go` (in-flight), `service/mediaprocessor.go` (pack linking), `interactors/download_interactor.go` (per-episode completion), settings; UI Settings toggle
