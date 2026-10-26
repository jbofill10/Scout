## ADDED Requirements

### Requirement: Subtitle Fetching After Import

When subtitle fetching is configured, the torrenter SHALL fetch subtitles for
each video it links into the Plex library, without ever turning a successful
download into a failure.

#### Scenario: Subtitle written beside the linked video

- **WHEN** a completed torrent has been linked into the library
- **AND** `OPENSUBTITLES_API_KEY` is set
- **THEN** the service SHALL search OpenSubtitles using the video's file hash and the media's IMDb id (parent id plus season and episode for an episode)
- **AND** write the best candidate as `<video basename>.<lang>.srt` (`.<lang>.sdh.srt` when hearing-impaired) in the same directory
- **AND** ask Plex to rescan that directory

#### Scenario: Candidate selection is deterministic

- **WHEN** several candidates are returned for a language
- **THEN** candidates in another language, machine or AI translated, foreign-parts-only, or for a different episode SHALL be discarded
- **AND** the rest SHALL be ranked by: provider hash match, same release group as the downloaded file, matching resolution/source/codec tokens, trusted uploader, download count and rating, with hearing-impaired tracks penalised unless `SUBTITLE_PREFER_SDH` is true

#### Scenario: Mistimed subtitle rejected

- **WHEN** a downloaded subtitle has fewer than 50 cues (20 for an episode), or its last cue ends more than two minutes past the known runtime or before half of it
- **THEN** it SHALL be discarded and the next ranked candidate downloaded, up to `SUBTITLE_MAX_DOWNLOADS_PER_ITEM` downloads

#### Scenario: Existing subtitle respected

- **WHEN** a `.<lang>.srt` or `.<lang>.sdh.srt` already exists beside the video
- **THEN** no search or download SHALL be made for that language and the file SHALL NOT be overwritten

#### Scenario: Failures are non-fatal

- **WHEN** the provider is unreachable, returns nothing acceptable, or the daily download quota (HTTP 406) is hit
- **THEN** the download SHALL still be recorded as successful and its notification completed
- **AND** after a quota error no further provider calls SHALL be made until the next UTC day

#### Scenario: Feature disabled

- **WHEN** `OPENSUBTITLES_API_KEY` is empty
- **THEN** the torrenter SHALL log once at startup that subtitle fetching is disabled and make no provider calls
