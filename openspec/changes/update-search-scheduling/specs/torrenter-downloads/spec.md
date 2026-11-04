## ADDED Requirements

### Requirement: Finished downloads searched before indexers
Before searching indexers for a show's episodes, the torrenter SHALL look through the finished torrents in its qBittorrent category for one that already holds them: a pack whose title covers their seasons under the same acceptance rules as a search result (show name, season coverage, quality floor) and whose downloaded files match the wanted episodes by number or title, or a single-episode release that passes the same validation as a search result. Episodes found this way SHALL be linked from disk through the normal completion path, reported as downloading, and SHALL NOT be searched for. Torrents still in flight are not considered here. A torrent list that cannot be fetched SHALL leave the search to proceed as before.

#### Scenario: Finished pack covers lost episodes
- **WHEN** a request for Dexter S01E02 to S01E12 arrives and qBittorrent holds the finished "Dexter (2006) S01-S08 1080p" pack whose files are named "Dexter (2006) - S01E02 - Crocodile (...).mkv" and so on
- **THEN** the eleven episodes are linked from the pack's files, read downloading and then completed, and no indexer query runs for them

#### Scenario: Finished torrent is another show or below the floor
- **WHEN** the only finished torrents are "Dexter Resurrection S01 2160p" and a 720p Dexter pack while `min_quality` is 1080p
- **THEN** neither is used and the indexer search proceeds

#### Scenario: Finished single release
- **WHEN** qBittorrent holds a finished "Dexter S08E11 1080p" release and S08E11 is wanted
- **THEN** that episode is linked from it and only the other wanted episodes are searched

#### Scenario: qBittorrent unavailable
- **WHEN** the torrent list cannot be fetched
- **THEN** the indexer search proceeds as before
