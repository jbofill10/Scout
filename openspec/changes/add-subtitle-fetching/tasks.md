# Tasks

## 1. IMDb id plumbing

- [x] 1.1 Decode `remoteIds` on the TVDB extended record; fix `RemoteID.Id` to string
- [x] 1.2 Add `Media.ImdbID` and `TVDBSeriesExtendedData.ImdbID()`
- [x] 1.3 Set `ImdbID` on every webserver download/schedule payload
- [x] 1.4 Carry `ImdbID` and `RuntimeMinutes` on `SearchStrategy`

## 2. Torrenter subtitle fetching

- [x] 2.1 `SubtitlesCfg` + env parsing in `config.Load`
- [x] 2.2 OpenSubtitles REST client (search, login, download, quota error)
- [x] 2.3 `SubtitleSvc`: hash, search ladder, scoring, sanity check, sidecar write, Plex refresh, quota pause
- [x] 2.4 `FileSystem.WriteFile` / `Exists`; `PlexLibraryRefresher`
- [x] 2.5 Hook into `ProcessDownloadedTorrent` (non-fatal) and wire in `main.go`

## 3. Config, docs, tests

- [x] 3.1 Compose files, `.env`, `.env.template`
- [x] 3.2 CLAUDE.md and openspec/project.md
- [x] 3.3 Unit tests: hash, sanity check, scoring, fetch flow, client, config, IMDb decoding, processor hook
- [ ] 3.4 Obtain an OpenSubtitles API key + account, set them in `.env`, rebuild the torrenter
- [ ] 3.5 Verify on the next real download that a `.en.srt` appears and Plex lists it
