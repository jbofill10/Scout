# Change: Fetch Subtitles After a Download Lands

## Why

Plex's own subtitle agent is unreliable for the library Scout builds: on
2026-10-25 "The Taking of Deborah Logan" arrived with no subtitles and Plex
found none, so they had to be located and placed by hand. Every step of that
manual job was deterministic (identify the file, hash it, query a provider,
prefer the release the file came from, reject a mistimed track, write
`<video>.en.srt` beside it, refresh Plex), which makes it a post-download step
in the torrenter rather than something to repeat by hand or hand to an agent.

## What Changes

- **IMDb id plumbing.** The TVDB extended record's `remoteIds` are decoded
  (`RemoteID.Id` becomes a string, matching TVDB v4) and the IMDb title id is
  carried on `media.Media.ImdbID` through every payload the webserver sends or
  schedules, and on `SearchStrategy.ImdbID` / `RuntimeMinutes` in the torrenter.
- **OpenSubtitles client** (`torrenter/clients/opensubtitles.go`): search by
  IMDb id or parent id + season/episode, with the OpenSubtitles file hash so
  exact-rip matches are flagged; title + year fallback; login token cached and
  renewed on 401; typed `APIError` with quota detection.
- **Subtitle service** (`torrenter/service/subtitles.go`): per configured
  language, skip if a sidecar exists, search, score candidates (hash match ≫
  same release group ≫ resolution/source/codec agreement ≫ trust and
  popularity; SDH penalised unless preferred; machine/AI translations and
  foreign-parts-only rejected; wrong episode rejected), download up to
  `SUBTITLE_MAX_DOWNLOADS_PER_ITEM` candidates until one passes a runtime
  sanity check, write it atomically as `<video>.<lang>.srt` (`.sdh.srt` for
  hearing-impaired), then ask Plex to rescan that directory. A 406 quota
  answer pauses fetching until the next UTC day.
- **Hook** in `MediaProcessSvc.ProcessDownloadedTorrent` after the link into
  the library succeeds. Outcomes are logged; a subtitle failure never fails the
  download.
- **Configuration**: `OPENSUBTITLES_API_KEY` (empty disables the feature),
  `OPENSUBTITLES_USERNAME`/`PASSWORD` (needed for downloads),
  `SUBTITLE_LANGUAGES` (default `en`), `SUBTITLE_PREFER_SDH`,
  `SUBTITLE_MAX_DOWNLOADS_PER_ITEM` (3), `SUBTITLE_TIMEOUT` (2m). Wired in
  both compose files and `.env.template`.

## Impact

- Affected specs: `torrenter-downloads` (new requirement), no API changes.
- Affected code: `pkg/media`, webserver download interactor (payload field),
  torrenter config/models/clients/service, `cmd/torrenter/main.go`.
- Scheduled rows written before this change have no `imdbId`; they fall back
  to the hash + title search.
- `FileSystem` gains `WriteFile` and `Exists`; mocks regenerated.

## Approval

Approved in conversation on 2026-10-25 ("Ok implement it") after the
programmed-vs-agentic design discussion; implementation proceeded on that
basis.
