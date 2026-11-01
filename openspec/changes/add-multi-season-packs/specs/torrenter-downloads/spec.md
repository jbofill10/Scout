## ADDED Requirements

### Requirement: Multi-season packs
When `season_packs` is on and a show request has episodes to download from more than one season, the torrenter SHALL first search for one pack covering every such season, accepting a title that names a season range enclosing them or a complete series, SHALL fall back to one pack per season with at least `season_pack_min_episodes` episodes, and SHALL search per episode for whatever no pack covered. A pack spanning more seasons than wanted qualifies but ranks below one that spans exactly the wanted seasons.

#### Scenario: Whole-series pack for a span of seasons
- **WHEN** seasons 5 to 10 are requested and "Show S01-13 1080p COMPLETE" is the only pack listed at or above the quality floor
- **THEN** that one torrent is added, every wanted episode reads downloading, and no per-episode search runs for them

#### Scenario: Exact span preferred
- **WHEN** both "Show S05-S10 1080p" and "Show S01-13 1080p COMPLETE" are listed with live swarms
- **THEN** the S05-S10 pack is chosen

#### Scenario: No multi-season pack
- **WHEN** no pack covers every wanted season
- **THEN** seasons with enough episodes are searched for single-season packs and the rest per episode, as before

### Requirement: Pack-first dispatch
When season packs are on and a show request has at least `season_pack_min_episodes` episodes to download, the webserver SHALL first send the whole set to the torrenter as a pack-only request. The torrenter SHALL answer a pack-only request with the episodes a pack covered as downloading and every other episode with code `no_pack`, within the per-episode search deadline, and SHALL NOT search those episodes one by one itself. The webserver SHALL then send the `no_pack` episodes one at a time as before.

#### Scenario: Pack covers part of the request
- **WHEN** a request of seasons 5 to 7 is sent pack-only and a pack covers seasons 5 and 6
- **THEN** the season 5 and 6 episodes read downloading and the season 7 episodes are sent to the torrenter one at a time

#### Scenario: Packs off
- **WHEN** `season_packs` is off
- **THEN** the webserver sends episodes one at a time and never a pack-only request

### Requirement: Multi-episode pack files
A pack file named for a run of episodes of one season SHALL be linked once under the Plex multi-episode name "Show - S##E##-E##.ext", and every wanted episode in the run SHALL be marked linked. A pack none of whose files match a wanted episode SHALL be removed and its episodes marked lost.

#### Scenario: Two segments in one file
- **WHEN** a pack holds "Show - S07E01-E02.mkv" and both episodes were wanted
- **THEN** the file is linked as "Show - S07E01-E02.mkv" and both episodes complete

### Requirement: Pack file selection
The torrenter SHALL add a pack torrent paused, starting it only to fetch its metadata when it has no file list (a magnet). Once its file list is available in qBittorrent, it SHALL deselect the files that hold no wanted episode, matching files by their own season and episode numbers (or absolute number), with a bare episode number read against the season folder the file sits in; when no file can be matched the whole pack SHALL stay selected. It SHALL then apply the free-space guard to the size of the selected files and start the torrent when it fits, or remove it and report every episode of the pack short of disk space when it does not. When a pack torrent is given up on for any reason, every other episode it stood for SHALL be marked lost so the retry engine re-arms them.

#### Scenario: Six of thirteen seasons
- **WHEN** a thirteen-season pack is added for a request of seasons 5 to 10
- **THEN** only the files of seasons 5 to 10 are left selected, the torrent is started, and the download completes when they are done

#### Scenario: Trimmed pack still too big
- **WHEN** the selected files of a pack exceed the free space less the reserve
- **THEN** the torrent is removed before it starts, the release is not blacklisted, and each of the pack's episodes reads insufficient disk space

#### Scenario: Landing reads season folders
- **WHEN** the pack's files are named "Season 07/Show - 03.mkv"
- **THEN** the file is linked as season 7 episode 3

### Requirement: Dead-swarm verdicts need a downloading torrent
The torrenter SHALL NOT treat a torrent as a dead swarm or as stalled while qBittorrent reports it queued, checking, allocating, moving or paused; the stall clocks run only while it is downloading. A torrent qBittorrent reports in an error state SHALL be given up on after the dead-swarm grace period and replaced, without the release being added to the item's failed list.

#### Scenario: Queued behind the active-download limit
- **WHEN** a torrent sits in `queuedDL` with no connected seeds for longer than the grace period
- **THEN** it is not removed, and it completes once qBittorrent starts it

#### Scenario: Client-side error
- **WHEN** qBittorrent reports `error` for a torrent for the grace period
- **THEN** the torrent is removed and a replacement searched for, and the release is not excluded from that search

### Requirement: Failed-torrent exclusions expire
A release recorded as failed for an item SHALL be excluded from that item's searches for `FAILED_TORRENT_TTL` (default 14 days) and SHALL be eligible again afterwards.

#### Scenario: Old verdict
- **WHEN** a release was given up on 30 days ago
- **THEN** a search for the item may pick it again

### Requirement: Bounded search ladder with a shared deadline
The torrenter SHALL search at most four names per episode (the title first, then aliases deduplicated by normalised form, Latin-script ones first), SHALL repeat only the "S##E##" form (and the padded absolute number for anime) under every name, SHALL drop repeated queries, and SHALL share the search deadline between the indexer tiers so that a fallback indexer is always queried when the preferred one yields nothing.

#### Scenario: Many aliases
- **WHEN** a show has six alias variants and the preferred indexer answers slowly
- **THEN** the fallback indexer is still queried with the strict "S##E##" form before the deadline

### Requirement: Word-level show name matching
A release SHALL match the expected title only when its parsed title has the same words, the same words once articles, years and country tags are set aside, or one extra leading possessive. A parsed title with an extra trailing word is another show and SHALL be rejected.

#### Scenario: Spin-off rejected
- **WHEN** "Dragon Ball DAIMA S01E04" is listed for a Dragon Ball episode
- **THEN** it is rejected as a name mismatch

#### Scenario: Year and article tolerated
- **WHEN** "SpongeBob SquarePants (1999) S16E11" is listed for SpongeBob SquarePants
- **THEN** it passes name validation
